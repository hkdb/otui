package model

import (
	"context"
	"sync"
	"testing"
	"time"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/config"
	"otui/ollama"
	"otui/storage"
)

// These tests run a Cmd concurrently with the kind of change Update makes
// while it runs. Under -race they fail if the Cmd shares state with Update.

// blockingProvider holds ChatWithTools until released, so a Cmd is still
// running while the test changes the model
type blockingProvider struct {
	metaProvider
	started chan struct{}
	release chan struct{}
}

func newBlockingProvider() *blockingProvider {
	return &blockingProvider{started: make(chan struct{}, 8), release: make(chan struct{})}
}

func (p *blockingProvider) ChatWithTools(ctx context.Context, msgs []Message, _ []mcptypes.Tool, callback StreamCallback) error {
	p.started <- struct{}{}
	<-p.release
	return callback("done", nil)
}

func (p *blockingProvider) ListModels(context.Context) ([]ollama.ModelInfo, error) {
	return []ollama.ModelInfo{{Name: "m", Provider: "cloud"}}, nil
}

func runCmd(cmd func() any) chan any {
	done := make(chan any, 1)
	go func() { done <- cmd() }()
	return done
}

func TestSendToOllamaDoesNotShareMessages(t *testing.T) {
	provider := newBlockingProvider()
	m := toolCallModel(provider)
	m.Messages = []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}

	cmd := m.SendToOllama()
	done := runCmd(func() any { return cmd() })

	// Update: a markdown render finished (no synchronization with the Cmd,
	// like the real event loop)
	m.Messages[1].Rendered = "rendered"
	m.CurrentIteration = 1

	<-provider.started
	close(provider.release)
	<-done
}

func TestToolStepDoesNotTouchIterationState(t *testing.T) {
	provider := newBlockingProvider()
	m := toolCallModel(provider)
	m.Config.RequireApproval = false
	m.Config.EnableMultiStep = true
	m.MaxIterations = 10

	cmd := m.ExecuteToolsAndContinue(ToolCallsDetectedMsg{})
	done := runCmd(func() any { return cmd() })

	// Update: Esc resets the iteration state (no synchronization with the Cmd)
	m.ResetIterations()

	<-provider.started
	close(provider.release)
	<-done
}

func TestConcurrentModelFetchesDontWriteCache(t *testing.T) {
	m := &Model{
		Config:      &config.Config{},
		Providers:   map[string]Provider{"cloud": &blockingProvider{}},
		ModelCache:  map[string][]ollama.ModelInfo{},
		CacheExpiry: map[string]time.Time{},
	}

	// Startup fetch and a provider refresh overlap
	first, second := m.FetchAllModels(false), m.FetchAllModels(false)
	var wg sync.WaitGroup
	var results [2]ModelsListMsg
	for i, cmd := range []func() any{func() any { return first() }, func() any { return second() }} {
		wg.Add(1)
		go func(i int, cmd func() any) {
			defer wg.Done()
			results[i] = cmd().(ModelsListMsg)
		}(i, cmd)
	}
	wg.Wait()

	if len(m.ModelCache) != 0 {
		t.Fatal("FetchAllModels wrote the cache from its Cmd")
	}
	m.StoreModelCache(results[0].Fetched)
	if len(m.ModelCache["cloud"]) != 1 {
		t.Fatalf("cache = %v, want the fetched cloud models", m.ModelCache)
	}

	// Cached now: the next fetch doesn't call the provider again
	if next := m.FetchAllModels(false)().(ModelsListMsg); len(next.Fetched) != 0 || len(next.Models) != 1 {
		t.Errorf("second fetch: fetched=%v models=%d, want cached list", next.Fetched, len(next.Models))
	}
}

func TestSaveCurrentSessionSavesSnapshot(t *testing.T) {
	ss, err := storage.NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := toolCallModel(&metaProvider{})
	m.SessionStorage = ss
	m.CurrentSession.ID = ""

	cmd := m.SaveCurrentSession()
	id := m.CurrentSession.ID
	if id == "" {
		t.Fatal("session ID not assigned before saving")
	}
	done := runCmd(func() any { return cmd() })

	// Update: a tool is approved while the save runs
	m.CurrentSession.AllowedTools = append(m.CurrentSession.AllowedTools, "fs.read_file")
	m.CurrentSession.Model = "other"

	if msg := (<-done).(SessionSavedMsg); msg.Err != nil {
		t.Fatal(msg.Err)
	}
	if saved, err := ss.Load(id); err != nil || saved.ID != id {
		t.Fatalf("saved session %v, err %v", saved, err)
	}
}

func TestApplyToolStep(t *testing.T) {
	m := &Model{}
	m.CurrentIteration = 2

	step1 := &ToolExecutionCompleteMsg{Step: &IterationStep{StepNumber: 1}, HasMoreSteps: true}
	m.ApplyToolStep(step1)
	if len(m.IterationHistory) != 1 || step1.IterationSummary.TotalSteps != 0 {
		t.Fatalf("after step 1: history=%d summary=%+v", len(m.IterationHistory), step1.IterationSummary)
	}

	step2 := &ToolExecutionCompleteMsg{Step: &IterationStep{StepNumber: 2}}
	m.ApplyToolStep(step2)
	if step2.IterationSummary.TotalSteps != 2 || step2.IterationSummary.MaxReached {
		t.Errorf("summary = %+v, want 2 steps", step2.IterationSummary)
	}
	if m.CurrentIteration != 0 || m.IterationHistory != nil {
		t.Error("iteration state not reset after the last step")
	}

	maxed := &ToolExecutionCompleteMsg{MaxReached: true}
	m.ApplyToolStep(maxed)
	if !maxed.IterationSummary.MaxReached {
		t.Error("max iterations not reported in the summary")
	}
}
