package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"otui/model"
)

// SetModel is called from Update while requests run in Cmd goroutines;
// these fail under -race if the model field isn't guarded.

func TestSetModelConcurrentWithGetModel(t *testing.T) {
	openai, _ := NewOpenAIProvider("http://127.0.0.1:1/v1", "k", "a")
	compat, _ := NewOpenAICompatibleProvider("http://127.0.0.1:1/v1", "", "a")
	anthropicP, _ := NewAnthropicProvider("http://127.0.0.1:1", "k", "a")
	openrouter, _ := NewOpenRouterProvider("http://127.0.0.1:1/api/v1", "k", "a")
	ollamaP, _ := NewOllamaProvider("http://127.0.0.1:1", "a", "")

	for name, p := range map[string]model.Provider{
		"openai": openai, "openai-compatible": compat, "anthropic": anthropicP,
		"openrouter": openrouter, "ollama": ollamaP,
	} {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); p.SetModel("b") }()
		go func() { defer wg.Done(); _ = p.GetModel(); _ = p.GetDisplayName() }()
		wg.Wait()
		if got := p.GetModel(); got != "b" {
			t.Errorf("%s: model = %q, want b", name, got)
		}
	}
}

func TestSetModelDuringRequest(t *testing.T) {
	requested := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requested)
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(srv.URL+"/v1", "k", "a")
	done := make(chan error, 1)
	go func() {
		done <- p.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil,
			func(string, []model.ToolCall) error { return nil })
	}()

	<-requested
	p.SetModel("b") // User switches model while the request runs
	close(release)
	<-done
}
