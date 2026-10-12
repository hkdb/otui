package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testModel = "Qwen3.6-27B-Q4_K_M"

// serveJSON returns a handler serving fixed JSON bodies by path ("path?query" also matches)
func serveJSON(t *testing.T, routes map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
}

func modelsList(entry map[string]any) map[string]any {
	entry["id"] = testModel
	entry["object"] = "model"
	return map[string]any{"object": "list", "data": []any{entry}}
}

func TestContextProber(t *testing.T) {
	tests := []struct {
		name       string
		routes     map[string]any
		wantCtx    int
		wantSource string
	}{
		{
			name: "llama.cpp b9076+ meta.n_ctx",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{"meta": map[string]any{"n_ctx": 65536, "n_ctx_train": 262144}}),
			},
			wantCtx: 65536, wantSource: "llama.cpp:/v1/models meta.n_ctx",
		},
		{
			name: "older llama.cpp uses /props at server root",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{"meta": map[string]any{"n_ctx_train": 262144}}),
				"/props":     map[string]any{"default_generation_settings": map[string]any{"n_ctx": 32768}, "total_slots": 1},
			},
			wantCtx: 32768, wantSource: "llama.cpp:/props",
		},
		{
			name: "llama.cpp router mode needs ?model= and must not autoload",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{}),
				"/props?autoload=false&model=" + testModel: map[string]any{"default_generation_settings": map[string]any{"n_ctx": 16384}},
			},
			wantCtx: 16384, wantSource: "llama.cpp:/props?model",
		},
		{
			name: "vLLM max_model_len",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{"max_model_len": 40960}),
			},
			wantCtx: 40960, wantSource: "vllm:/v1/models max_model_len",
		},
		{
			name: "LM Studio loaded instance",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{}),
				"/api/v1/models": map[string]any{"models": []any{map[string]any{
					"key":                testModel,
					"max_context_length": 262144,
					"loaded_instances":   []any{map[string]any{"id": testModel, "config": map[string]any{"context_length": 8192}}},
				}}},
			},
			wantCtx: 8192, wantSource: "lmstudio:/api/v1/models loaded context",
		},
		{
			name: "Ollama /v1 endpoint uses native /api/ps",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{}),
				"/api/ps":    map[string]any{"models": []any{map[string]any{"name": testModel, "model": testModel, "context_length": 4096}}},
				"/api/show":  map[string]any{"model_info": map[string]any{"general.architecture": "qwen35", "qwen35.context_length": 262144}},
			},
			wantCtx: 4096, wantSource: "ollama:/api/ps",
		},
		{
			name: "only trained context known",
			routes: map[string]any{
				"/v1/models": modelsList(map[string]any{"meta": map[string]any{"n_ctx_train": 262144}}),
			},
			wantCtx: 262144, wantSource: "llama.cpp:/v1/models meta.n_ctx_train",
		},
		{
			name:    "nothing known",
			routes:  map[string]any{"/v1/models": modelsList(map[string]any{})},
			wantCtx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serveJSON(t, tt.routes)
			defer srv.Close()

			cp := newContextProber(srv.URL+"/v1/", "")
			got, source := cp.contextWindow(context.Background(), testModel)
			if got != tt.wantCtx {
				t.Errorf("contextWindow = %d (%s), want %d", got, source, tt.wantCtx)
			}
			if tt.wantSource != "" && source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
		})
	}
}

func TestContextProberRemembersServerKind(t *testing.T) {
	var propsCalls, lmStudioCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			json.NewEncoder(w).Encode(modelsList(map[string]any{}))
		case "/props":
			propsCalls++
			json.NewEncoder(w).Encode(map[string]any{"default_generation_settings": map[string]any{"n_ctx": 32768}})
		case "/api/v1/models", "/api/v0/models/" + testModel:
			lmStudioCalls++
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cp := newContextProber(srv.URL+"/v1", "")
	for i := 0; i < 3; i++ {
		if got, _ := cp.contextWindow(context.Background(), testModel); got != 32768 {
			t.Fatalf("contextWindow = %d, want 32768", got)
		}
	}
	if propsCalls != 3 {
		t.Errorf("/props called %d times, want 3", propsCalls)
	}
	if lmStudioCalls != 0 {
		t.Errorf("later probes ran %d times after llama.cpp was detected", lmStudioCalls)
	}
}

func TestContextProberSendsAPIKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(modelsList(map[string]any{"max_model_len": 4096}))
	}))
	defer srv.Close()

	cp := newContextProber(srv.URL+"/v1", "secret")
	cp.contextWindow(context.Background(), testModel)
	if auth != "Bearer secret" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer secret")
	}
}

func TestOpenAICompatibleGetModelMetadata(t *testing.T) {
	srv := serveJSON(t, map[string]any{
		"/v1/models": modelsList(map[string]any{"max_model_len": 40960}),
	})
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(srv.URL+"/v1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.GetModelMetadata(context.Background(), testModel)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ContextWindow != 40960 || !meta.SupportsTools {
		t.Errorf("metadata = %+v, want 40960 with tools", meta)
	}

	// Unreachable server falls back instead of failing
	srv.Close()
	meta, err = p.GetModelMetadata(context.Background(), testModel)
	if err != nil || meta.ContextWindow != 8192 || meta.Source != "fallback" {
		t.Errorf("unreachable: metadata = %+v, err = %v; want fallback", meta, err)
	}
}

func TestOpenRouterGetModelMetadata(t *testing.T) {
	srv := serveJSON(t, map[string]any{
		"/api/v1/models": map[string]any{"data": []any{
			map[string]any{
				"id": "qwen/qwen3.8-27b", "object": "model", "context_length": 1000000,
				"top_provider": map[string]any{"context_length": 262144, "max_completion_tokens": 65536},
			},
			map[string]any{
				"id": "vendor/router-model", "object": "model", "context_length": 131072,
				"top_provider": map[string]any{"context_length": nil},
			},
		}},
	})
	defer srv.Close()

	p, err := NewOpenRouterProvider(srv.URL+"/api/v1", "key", "")
	if err != nil {
		t.Fatal(err)
	}

	meta, _ := p.GetModelMetadata(context.Background(), "qwen/qwen3.8-27b")
	if meta.ContextWindow != 262144 || meta.MaxOutput != 65536 {
		t.Errorf("top_provider: metadata = %+v, want 262144/65536", meta)
	}

	meta, _ = p.GetModelMetadata(context.Background(), "vendor/router-model")
	if meta.ContextWindow != 131072 {
		t.Errorf("null top_provider: ContextWindow = %d, want context_length 131072", meta.ContextWindow)
	}

	meta, _ = p.GetModelMetadata(context.Background(), "vendor/unknown")
	if meta.Source != "fallback" {
		t.Errorf("unknown model: source = %q, want fallback", meta.Source)
	}
}

func TestAnthropicGetModelMetadata(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/models/") {
			http.NotFound(w, r)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": strings.TrimPrefix(r.URL.Path, "/v1/models/"), "type": "model",
			"display_name": "Claude", "created_at": "2026-01-01T00:00:00Z",
			"max_input_tokens": 1000000, "max_tokens": 128000,
		})
	}))
	defer srv.Close()

	p, err := NewAnthropicProvider(srv.URL, "key", "")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		meta, _ := p.GetModelMetadata(context.Background(), "claude-opus-5-5")
		if meta.ContextWindow != 1000000 || meta.MaxOutput != 128000 {
			t.Errorf("metadata = %+v, want 1000000/128000", meta)
		}
	}
	if calls != 1 {
		t.Errorf("Models API called %d times, want 1 (cached)", calls)
	}
}

func TestRawJSONInt(t *testing.T) {
	tests := map[string]int{"": 0, "null": 0, "262144": 262144, "1.0e6": 1000000, `"x"`: 0}
	for raw, want := range tests {
		if got := rawJSONInt(raw); got != want {
			t.Errorf("rawJSONInt(%q) = %d, want %d", raw, got, want)
		}
	}
}
