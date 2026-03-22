package ollama

import (
	"testing"
)

func TestModelSupportsToolCalling(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		// Supported models
		{"llama3.1:latest", true},
		{"llama3.1:8b", true},
		{"llama3.2:3b", true},
		{"llama3.3:70b", true},
		{"qwen2.5-coder:latest", true},
		{"qwen:latest", true},
		{"mistral:latest", true},
		{"mistral-nemo:latest", true},
		{"command-r:latest", true},
		{"nemotron:latest", true},
		{"granite3:latest", true},

		// Unsupported models
		{"llama3:latest", false},
		{"llama3:8b", false},
		{"llama3-gradient:latest", false},
		{"phi:latest", false},
		{"gemma:latest", false},
		{"codellama:latest", false},
		{"deepseek:latest", false},

		// Unknown models
		{"unknown-model:latest", false},

		// Case insensitivity
		{"Llama3.1:Latest", true},
		{"QWEN:latest", true},
		{"Mistral:7b", true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ModelSupportsToolCalling(tt.model)
			if got != tt.want {
				t.Errorf("ModelSupportsToolCalling(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("ValidURL", func(t *testing.T) {
		client, err := NewClient("http://localhost:11434", "llama3.1:latest", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.model != "llama3.1:latest" {
			t.Errorf("expected model 'llama3.1:latest', got %q", client.model)
		}
		if client.baseURL != "http://localhost:11434" {
			t.Errorf("expected baseURL 'http://localhost:11434', got %q", client.baseURL)
		}
	})

	t.Run("Defaults", func(t *testing.T) {
		client, err := NewClient("", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.model != "llama3.1:latest" {
			t.Errorf("expected default model, got %q", client.model)
		}
		if client.baseURL != "http://localhost:11434" {
			t.Errorf("expected default baseURL, got %q", client.baseURL)
		}
	})

	t.Run("WithAPIKey", func(t *testing.T) {
		client, err := NewClient("http://localhost:11434", "test:latest", "my-api-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.model != "test:latest" {
			t.Errorf("expected model 'test:latest', got %q", client.model)
		}
	})

	t.Run("InvalidURL", func(t *testing.T) {
		_, err := NewClient("://invalid", "model", "")
		if err == nil {
			t.Error("expected error for invalid URL")
		}
	})
}

func TestSetGetModel(t *testing.T) {
	client, err := NewClient("http://localhost:11434", "initial:latest", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.GetModel() != "initial:latest" {
		t.Errorf("expected 'initial:latest', got %q", client.GetModel())
	}

	client.SetModel("new:latest")
	if client.GetModel() != "new:latest" {
		t.Errorf("expected 'new:latest', got %q", client.GetModel())
	}
}

func TestClientSupportsToolCalling(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"llama3.1:latest", true},
		{"llama3:latest", false},
		{"qwen:7b", true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			client, err := NewClient("http://localhost:11434", tt.model, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := client.SupportsToolCalling()
			if got != tt.want {
				t.Errorf("SupportsToolCalling() for %q = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
