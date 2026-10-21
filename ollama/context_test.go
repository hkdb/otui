package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseNumCtx(t *testing.T) {
	params := "stop                           \"<|im_end|>\"\nnum_ctx                        32768\ntemperature                    0.6"
	if got := parseNumCtx(params); got != 32768 {
		t.Errorf("parseNumCtx = %d, want 32768", got)
	}
	if got := parseNumCtx("temperature 0.6"); got != 0 {
		t.Errorf("parseNumCtx without num_ctx = %d, want 0", got)
	}
}

func TestTrainedContextLength(t *testing.T) {
	info := map[string]any{
		"general.architecture":    "qwen35",
		"qwen35.context_length":   float64(262144),
		"qwen35.embedding_length": float64(5120),
	}
	if got := trainedContextLength(info); got != 262144 {
		t.Errorf("trainedContextLength = %d, want 262144", got)
	}
	if got := trainedContextLength(map[string]any{}); got != 0 {
		t.Errorf("trainedContextLength(empty) = %d, want 0", got)
	}
}

func TestSameModelName(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"qwen3:8b", "qwen3:8b", true},
		{"qwen3", "qwen3:latest", true},
		{"hf.co/unsloth/Qwen3.6-27B-GGUF:Q4_K_M", "hf.co/unsloth/Qwen3.6-27B-GGUF:Q4_K_M", true},
		{"hf.co/unsloth/Qwen3.6-27B-GGUF", "hf.co/unsloth/Qwen3.6-27B-GGUF:latest", true},
		{"qwen3:8b", "qwen3:14b", false},
	}
	for _, tt := range tests {
		if got := sameModelName(tt.a, tt.b); got != tt.want {
			t.Errorf("sameModelName(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestModelContextEffective(t *testing.T) {
	tests := []struct {
		name string
		mc   ModelContext
		want int
	}{
		{"loaded wins", ModelContext{Loaded: 4096, Modelfile: 32768, Trained: 262144}, 4096},
		{"modelfile when not loaded", ModelContext{Modelfile: 32768, Trained: 262144}, 32768},
		{"trained when nothing else", ModelContext{Trained: 262144}, 262144},
		{"capped at trained", ModelContext{Modelfile: 65536, Trained: 40960}, 40960},
		{"unknown", ModelContext{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := tt.mc.Effective(); got != tt.want {
				t.Errorf("Effective() = %d, want %d", got, tt.want)
			}
		})
	}
}

// fakeOllama serves /api/ps and /api/show for one model
func fakeOllama(t *testing.T, loadedCtx int, parameters string) *httptest.Server {
	t.Helper()
	const name = "hf.co/unsloth/Qwen3.6-27B-GGUF:Q4_K_M"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			models := []map[string]any{}
			if loadedCtx > 0 {
				models = append(models, map[string]any{"name": name, "model": name, "context_length": loadedCtx})
			}
			json.NewEncoder(w).Encode(map[string]any{"models": models})
		case "/api/show":
			json.NewEncoder(w).Encode(map[string]any{
				"parameters":   parameters,
				"capabilities": []string{"completion", "tools"},
				"model_info": map[string]any{
					"general.architecture":  "qwen35",
					"qwen35.context_length": 262144,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestGetModelContext(t *testing.T) {
	const name = "hf.co/unsloth/Qwen3.6-27B-GGUF:Q4_K_M"

	t.Run("loaded model", func(t *testing.T) {
		srv := fakeOllama(t, 8192, "")
		defer srv.Close()
		c, _ := NewClient(srv.URL, name, "")

		mc, err := c.GetModelContext(context.Background(), name)
		if err != nil {
			t.Fatalf("GetModelContext: %v", err)
		}
		if mc.Loaded != 8192 || mc.Trained != 262144 || !mc.SupportsTools {
			t.Errorf("got %+v", mc)
		}
		if got, _ := mc.Effective(); got != 8192 {
			t.Errorf("Effective = %d, want loaded 8192", got)
		}
	})

	t.Run("not loaded, modelfile num_ctx", func(t *testing.T) {
		srv := fakeOllama(t, 0, "num_ctx                        32768")
		defer srv.Close()
		c, _ := NewClient(srv.URL, name, "")

		mc, err := c.GetModelContext(context.Background(), name)
		if err != nil {
			t.Fatalf("GetModelContext: %v", err)
		}
		if got, _ := mc.Effective(); got != 32768 {
			t.Errorf("Effective = %d, want modelfile 32768", got)
		}
	})

	t.Run("server down", func(t *testing.T) {
		srv := fakeOllama(t, 0, "")
		srv.Close()
		c, _ := NewClient(srv.URL, name, "")
		if _, err := c.GetModelContext(context.Background(), name); err == nil {
			t.Error("expected error when server is unreachable")
		}
	})
}
