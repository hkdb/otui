package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"otui/model"
)

// anthropicAPI fakes the Models API and a streamed reply that stops with
// stopReason; it records the max_tokens of each request
type anthropicAPI struct {
	mu         sync.Mutex
	maxTokens  []int
	stopReason string
	modelsDown bool
}

func (a *anthropicAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && a.modelsDown:
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"type":"error","error":{"type":"api_error","message":"down"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			fmt.Fprint(w, `{"data":[{"type":"model","id":"claude-new-9","display_name":"Claude New 9","created_at":"2026-09-01T00:00:00Z"},
				{"type":"model","id":"claude-old-1","display_name":"Claude Old 1","created_at":"2025-01-01T00:00:00Z"}],
				"has_more":false,"first_id":"claude-new-9","last_id":"claude-old-1"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/models/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/models/")
			fmt.Fprintf(w, `{"type":"model","id":%q,"display_name":"x","created_at":"2026-09-01T00:00:00Z","max_input_tokens":500000,"max_tokens":64000}`, id)
		default:
			var req struct {
				MaxTokens int `json:"max_tokens"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			a.mu.Lock()
			a.maxTokens = append(a.maxTokens, req.MaxTokens)
			a.mu.Unlock()

			w.Header().Set("Content-Type", "text/event-stream")
			ev := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
			ev("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
			ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			ev("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Partial answer"}}`)
			ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
			ev("message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q,"stop_sequence":null},"usage":{"output_tokens":5}}`, a.stopReason))
			ev("message_stop", `{"type":"message_stop"}`)
		}
	}))
}

func chatOnce(t *testing.T, p *AnthropicProvider) (string, error) {
	t.Helper()
	var text strings.Builder
	err := p.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil,
		func(chunk string, _ []model.ToolCall) error { text.WriteString(chunk); return nil })
	return text.String(), err
}

func TestAnthropicUsesModelOutputLimit(t *testing.T) {
	api := &anthropicAPI{stopReason: "end_turn"}
	srv := api.server(t)
	defer srv.Close()

	p, _ := NewAnthropicProvider(srv.URL, "key", "claude-new-9")
	if _, err := chatOnce(t, p); err != nil {
		t.Fatal(err)
	}

	// Models API down: fallback table, or the default for unknown models
	api.modelsDown = true
	p2, _ := NewAnthropicProvider(srv.URL, "key", "claude-opus-5-5")
	chatOnce(t, p2)
	p3, _ := NewAnthropicProvider(srv.URL, "key", "claude-unknown")
	chatOnce(t, p3)

	if fmt.Sprint(api.maxTokens) != "[64000 128000 8192]" {
		t.Errorf("max_tokens sent = %v, want [64000 128000 8192] (Models API, fallback table, default)", api.maxTokens)
	}
}

func TestAnthropicReportsCutOffReply(t *testing.T) {
	api := &anthropicAPI{stopReason: "max_tokens"}
	srv := api.server(t)
	defer srv.Close()

	p, _ := NewAnthropicProvider(srv.URL, "key", "claude-new-9")
	text, err := chatOnce(t, p)
	if err == nil || !strings.Contains(err.Error(), "output limit (64000 tokens)") {
		t.Fatalf("err = %v, want cut-off error", err)
	}
	if text != "Partial answer" {
		t.Errorf("streamed %q before the error", text)
	}
}

func TestAnthropicListModelsFromAPI(t *testing.T) {
	api := &anthropicAPI{}
	srv := api.server(t)
	defer srv.Close()

	p, _ := NewAnthropicProvider(srv.URL, "key", "x")
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) != 2 || models[0].Name != "claude-new-9" || models[0].Provider != "anthropic" {
		t.Fatalf("models = %+v, err %v", models, err)
	}

	api.modelsDown = true
	models, _ = p.ListModels(context.Background())
	if len(models) != len(anthropicFallbackModels) {
		t.Errorf("fallback list = %+v", models)
	}
}
