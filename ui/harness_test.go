package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/config"
	appmodel "otui/model"
	"otui/ollama"
	"otui/storage"
)

// Test harness for the update loop: a real AppView (temp data dir, fake
// provider) driven like bubbletea drives it. Cmds run in goroutines and
// their messages are fed back into Update; spinner and cursor blink ticks
// are dropped (they'd tick forever).

// chatScript is a fake provider's reply to request n (0-based)
type chatScript func(ctx context.Context, n int, msgs []appmodel.Message, cb appmodel.StreamCallback) error

// fakeChatProvider is a scriptable model.Provider
type fakeChatProvider struct {
	script chatScript

	mu       sync.Mutex
	requests int
	lastCtx  context.Context
}

func (p *fakeChatProvider) Chat(ctx context.Context, msgs []appmodel.Message, cb appmodel.StreamCallback) error {
	return p.ChatWithTools(ctx, msgs, nil, cb)
}

func (p *fakeChatProvider) ChatWithTools(ctx context.Context, msgs []appmodel.Message, _ []mcptypes.Tool, cb appmodel.StreamCallback) error {
	p.mu.Lock()
	n := p.requests
	p.requests++
	p.lastCtx = ctx
	p.mu.Unlock()
	return p.script(ctx, n, msgs, cb)
}

func (p *fakeChatProvider) Requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

func (p *fakeChatProvider) ListModels(context.Context) ([]ollama.ModelInfo, error) {
	return []ollama.ModelInfo{{Name: "fake-model", InternalName: "fake-model", Provider: "fake"}}, nil
}
func (p *fakeChatProvider) GetModel() string           { return "fake-model" }
func (p *fakeChatProvider) GetDisplayName() string     { return "fake-model" }
func (p *fakeChatProvider) SetModel(string)            {}
func (p *fakeChatProvider) Ping(context.Context) error { return nil }
func (p *fakeChatProvider) FallbackModelMetadata(string) appmodel.ModelMetadata {
	return appmodel.ModelMetadata{ContextWindow: 8192, Source: appmodel.MetadataSourceFallback}
}
func (p *fakeChatProvider) GetModelMetadata(_ context.Context, m string) (appmodel.ModelMetadata, error) {
	return p.FallbackModelMetadata(m), nil
}

// stream sends chunks with a small delay each (like a real stream)
func stream(ctx context.Context, cb appmodel.StreamCallback, chunks ...string) error {
	for _, c := range chunks {
		select {
		case <-time.After(5 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := cb(c, nil); err != nil {
			return err
		}
	}
	return nil
}

type harness struct {
	t        *testing.T
	app      AppView
	provider *fakeChatProvider
	storage  *storage.SessionStorage
	msgs     chan tea.Msg
}

func newHarness(t *testing.T, script chatScript) *harness {
	t.Helper()
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDirectory:         dataDir,
		OllamaHost:            "http://127.0.0.1:1",
		DefaultProvider:       "fake",
		DefaultModel:          "fake-model",
		RequireApproval:       true,
		EnableMultiStep:       true,
		MaxIterations:         10,
		Keybindings:           config.DefaultKeybindings(),
		FirstTokenTimeoutSecs: 60,
		StreamIdleTimeoutSecs: 60,
	}
	ss, err := storage.NewSessionStorage(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	session := &storage.Session{Name: "Test", Model: "fake-model", Provider: "fake"}
	if err := ss.Save(session); err != nil {
		t.Fatal(err)
	}

	a := NewAppView(cfg, ss, session, "", "test", "test")
	fake := &fakeChatProvider{script: script}
	a.dataModel.Providers["fake"] = fake
	a.dataModel.Provider = fake

	h := &harness{t: t, app: a, provider: fake, storage: ss, msgs: make(chan tea.Msg, 256)}
	h.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return h
}

// update applies msg and runs the returned Cmd in the background
func (h *harness) update(msg tea.Msg) {
	m, cmd := h.app.Update(msg)
	h.app = m.(AppView)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		if msg := cmd(); msg != nil {
			h.msgs <- msg
		}
	}()
}

// deliver feeds a Cmd result back into Update, expanding batches and
// sequences and dropping endless ticks
func (h *harness) deliver(msg tea.Msg) {
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf((tea.Cmd)(nil)) {
		for i := 0; i < v.Len(); i++ {
			h.run(v.Index(i).Interface().(tea.Cmd))
		}
		return
	}
	if name := fmt.Sprintf("%T", msg); strings.HasPrefix(name, "spinner.") || strings.HasPrefix(name, "cursor.") {
		return
	}
	h.update(msg)
}

// runUntil processes messages until done is true
func (h *harness) runUntil(what string, done func(AppView) bool) {
	h.t.Helper()
	timeout := time.After(5 * time.Second)
	for !done(h.app) {
		select {
		case msg := <-h.msgs:
			h.deliver(msg)
		case <-timeout:
			h.t.Fatalf("timed out waiting for %s; messages: %s", what, h.transcript())
		}
	}
}

// settle processes messages for d (to check nothing else happens)
func (h *harness) settle(d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case msg := <-h.msgs:
			h.deliver(msg)
		case <-deadline:
			return
		}
	}
}

func (h *harness) send(text string) {
	h.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	h.update(tea.KeyMsg{Type: tea.KeyEnter})
}

func (h *harness) key(k string) {
	switch k {
	case "esc":
		h.update(tea.KeyMsg{Type: tea.KeyEsc})
	case "enter":
		h.update(tea.KeyMsg{Type: tea.KeyEnter})
	default:
		alt := strings.HasPrefix(k, "alt+")
		h.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.TrimPrefix(k, "alt+")), Alt: alt})
	}
}

func (h *harness) transcript() string {
	var parts []string
	for _, m := range h.app.dataModel.Messages {
		parts = append(parts, fmt.Sprintf("%s: %q", m.Role, m.Content))
	}
	return strings.Join(parts, " | ")
}

func (h *harness) last() appmodel.Message {
	msgs := h.app.dataModel.Messages
	if len(msgs) == 0 {
		return appmodel.Message{}
	}
	return msgs[len(msgs)-1]
}

func idle(a AppView) bool { return !a.dataModel.Streaming }

// --- Flows ---------------------------------------------------------------

func TestFlowReplyStreamsAndIsSaved(t *testing.T) {
	h := newHarness(t, func(ctx context.Context, _ int, _ []appmodel.Message, cb appmodel.StreamCallback) error {
		return stream(ctx, cb, "Hello ", "there, ", "friend.")
	})
	h.send("hi")
	h.runUntil("the reply", func(a AppView) bool { return idle(a) && len(a.dataModel.Messages) >= 2 })
	h.settle(100 * time.Millisecond) // Autosave

	if got := h.last(); got.Role != "assistant" || got.Content != "Hello there, friend." {
		t.Fatalf("last message %+v (%s)", got, h.transcript())
	}
	saved, err := h.storage.Load(h.app.dataModel.CurrentSession.ID)
	if err != nil || len(saved.Messages) != 2 || saved.Messages[1].Content != "Hello there, friend." {
		t.Errorf("saved session: %+v, err %v", saved, err)
	}
}

func TestFlowEscKeepsPartialReply(t *testing.T) {
	h := newHarness(t, func(ctx context.Context, _ int, _ []appmodel.Message, cb appmodel.StreamCallback) error {
		stream(ctx, cb, "Partial ", "answer")
		<-ctx.Done() // A slow model
		return ctx.Err()
	})
	h.send("hi")
	h.runUntil("text on screen", func(a AppView) bool { return strings.Contains(a.currentResp.String(), "answer") })
	h.key("esc")

	if h.app.dataModel.Streaming || h.last().Content != "Partial answer\n\n⚠️ Response cancelled" {
		t.Fatalf("after Esc: streaming=%v, %s", h.app.dataModel.Streaming, h.transcript())
	}
	if h.provider.lastCtx.Err() == nil {
		t.Error("request not cancelled")
	}
	h.settle(100 * time.Millisecond)
	if !strings.Contains(h.last().Content, "Response cancelled") {
		t.Errorf("late messages changed the chat: %s", h.transcript())
	}
}

func TestFlowToolPermissionDenied(t *testing.T) {
	h := newHarness(t, func(ctx context.Context, n int, _ []appmodel.Message, cb appmodel.StreamCallback) error {
		return cb("", []appmodel.ToolCall{{Name: "fs.write_file", Arguments: map[string]any{"path": "/tmp/x"}}})
	})
	h.send("write a file")
	h.runUntil("the permission prompt", func(a AppView) bool { return a.waitingForPermission })
	if !strings.Contains(h.last().Content, "fs.write_file") {
		t.Fatalf("prompt: %s", h.transcript())
	}

	h.key("n")
	h.settle(100 * time.Millisecond)
	if h.app.dataModel.Streaming || !strings.Contains(h.last().Content, "Permission denied for tool: fs.write_file") {
		t.Fatalf("after denying: %s", h.transcript())
	}
	if h.provider.Requests() != 1 {
		t.Errorf("model called %d times, want 1 (no follow-up after denial)", h.provider.Requests())
	}
}

func TestFlowSessionSwitchBlockedWhileStreaming(t *testing.T) {
	h := newHarness(t, func(ctx context.Context, _ int, _ []appmodel.Message, cb appmodel.StreamCallback) error {
		<-ctx.Done()
		return ctx.Err()
	})
	other := &storage.Session{Name: "Other", Model: "fake-model", Provider: "fake"}
	h.storage.Save(other)
	current := h.app.dataModel.CurrentSession.ID

	h.send("hi")
	h.key("alt+s") // Session manager
	h.runUntil("the session list", func(a AppView) bool { return a.showSessionManager && len(a.sessionList) == 2 })
	for i, s := range h.app.sessionList {
		if s.ID == other.ID {
			h.app.selectedSessionIdx = i
		}
	}
	h.key("enter")
	h.settle(100 * time.Millisecond)

	if !h.app.showAcknowledgeModal || h.app.dataModel.CurrentSession.ID != current || !h.app.dataModel.Streaming {
		t.Errorf("switch not blocked: warning=%v session=%s streaming=%v",
			h.app.showAcknowledgeModal, h.app.dataModel.CurrentSession.ID, h.app.dataModel.Streaming)
	}
}

func TestFlowStreamErrorKeepsPartialReply(t *testing.T) {
	h := newHarness(t, func(ctx context.Context, _ int, _ []appmodel.Message, cb appmodel.StreamCallback) error {
		stream(ctx, cb, "Half an ")
		return errors.New("connection reset")
	})
	h.send("hi")
	h.runUntil("the error", func(a AppView) bool { return idle(a) && strings.Contains(a.transcriptText(), "connection reset") })

	msgs := h.app.dataModel.Messages
	if len(msgs) < 3 || msgs[len(msgs)-2].Content != "Half an\n\n⚠️ Response incomplete" && msgs[len(msgs)-2].Content != "Half an \n\n⚠️ Response incomplete" {
		t.Errorf("partial reply not kept: %s", h.transcript())
	}
}

// transcriptText is all message contents (for conditions)
func (a AppView) transcriptText() string {
	var b strings.Builder
	for _, m := range a.dataModel.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}
