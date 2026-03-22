package ui

import (
	"strings"
	"testing"

	"otui/ollama"
)

func TestIsCurrentModel(t *testing.T) {
	tests := []struct {
		name         string
		model        ollama.ModelInfo
		currentModel string
		want         bool
	}{
		{
			"match_by_InternalName",
			ollama.ModelInfo{Name: "llama-3.1-8b:free", InternalName: "meta-llama/llama-3.1-8b:free"},
			"meta-llama/llama-3.1-8b:free",
			true,
		},
		{
			"match_by_Name",
			ollama.ModelInfo{Name: "llama3.1:latest", InternalName: "llama3.1:latest"},
			"llama3.1:latest",
			true,
		},
		{
			"no_match",
			ollama.ModelInfo{Name: "modelA", InternalName: "provider/modelA"},
			"different-model",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCurrentModel(tt.model, tt.currentModel)
			if got != tt.want {
				t.Errorf("IsCurrentModel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindModelByName(t *testing.T) {
	models := []ollama.ModelInfo{
		{Name: "model-a", InternalName: "provider/model-a"},
		{Name: "model-b", InternalName: "provider/model-b"},
		{Name: "model-c", InternalName: "provider/model-c"},
	}

	t.Run("Found", func(t *testing.T) {
		idx, model := FindModelByName(models, "provider/model-b")
		if idx != 1 {
			t.Errorf("expected index 1, got %d", idx)
		}
		if model == nil {
			t.Fatal("expected model, got nil")
		}
		if model.Name != "model-b" {
			t.Errorf("expected 'model-b', got %q", model.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		idx, model := FindModelByName(models, "nonexistent")
		if idx != -1 {
			t.Errorf("expected -1, got %d", idx)
		}
		if model != nil {
			t.Error("expected nil model")
		}
	})

	t.Run("EmptyList", func(t *testing.T) {
		idx, model := FindModelByName([]ollama.ModelInfo{}, "anything")
		if idx != -1 {
			t.Errorf("expected -1, got %d", idx)
		}
		if model != nil {
			t.Error("expected nil model")
		}
	})
}

func TestModelSupportsTools(t *testing.T) {
	tests := []struct {
		name  string
		model ollama.ModelInfo
		want  bool
	}{
		{
			"ollama_supported",
			ollama.ModelInfo{Name: "llama3.1:latest", Provider: "ollama"},
			true,
		},
		{
			"ollama_unsupported",
			ollama.ModelInfo{Name: "llama3:latest", Provider: "ollama"},
			false,
		},
		{
			"anthropic_always_true",
			ollama.ModelInfo{Name: "claude-3-opus", Provider: "anthropic", InternalName: "claude-3-opus"},
			true,
		},
		{
			"openai_gpt4",
			ollama.ModelInfo{Name: "gpt-4", Provider: "openai", InternalName: "gpt-4"},
			true,
		},
		{
			"openai_gpt35",
			ollama.ModelInfo{Name: "gpt-3.5-turbo", Provider: "openai", InternalName: "gpt-3.5-turbo"},
			true,
		},
		{
			"openai_unknown",
			ollama.ModelInfo{Name: "davinci", Provider: "openai", InternalName: "davinci"},
			false,
		},
		{
			"openrouter_default_true",
			ollama.ModelInfo{Name: "some-model", Provider: "openrouter", InternalName: "vendor/some-model"},
			true,
		},
		{
			"openrouter_small_llama",
			ollama.ModelInfo{Name: "llama-3.2-1b", Provider: "openrouter", InternalName: "meta-llama/llama-3.2-1b"},
			false,
		},
		{
			"unknown_provider",
			ollama.ModelInfo{Name: "model", Provider: "unknown", InternalName: "model"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModelSupportsTools(tt.model)
			if got != tt.want {
				t.Errorf("ModelSupportsTools() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		check func(t *testing.T, result string)
	}{
		{
			"short_text_no_wrap",
			"hello world",
			50,
			func(t *testing.T, result string) {
				if result != "hello world" {
					t.Errorf("expected unchanged text, got %q", result)
				}
			},
		},
		{
			"long_text_wraps",
			"the quick brown fox jumps over the lazy dog",
			20,
			func(t *testing.T, result string) {
				lines := strings.Split(result, "\n")
				if len(lines) < 2 {
					t.Errorf("expected multiple lines, got %d", len(lines))
				}
				for _, line := range lines {
					if len(line) > 20 {
						t.Errorf("line exceeds width: %q (len %d)", line, len(line))
					}
				}
			},
		},
		{
			"zero_width_returns_text",
			"hello",
			0,
			func(t *testing.T, result string) {
				if result != "hello" {
					t.Errorf("expected original text for zero width, got %q", result)
				}
			},
		},
		{
			"preserves_newlines",
			"line one\n\nline three",
			50,
			func(t *testing.T, result string) {
				if !strings.Contains(result, "\n") {
					t.Error("expected newlines to be preserved")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wordWrap(tt.text, tt.width)
			tt.check(t, result)
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no_ansi", "plain text", "plain text"},
		{"with_color", "\x1b[31mred text\x1b[0m", "red text"},
		{"with_bold", "\x1b[1mbold\x1b[0m normal", "bold normal"},
		{"multiple_codes", "\x1b[31;1mhi\x1b[0m \x1b[32mworld\x1b[0m", "hi world"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
