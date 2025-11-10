package provider

import (
	"otui/model"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		expectNil   bool
	}{
		{
			name: "ollama provider with defaults",
			config: Config{
				Type:    ProviderTypeOllama,
				BaseURL: "",
				Model:   "",
			},
			expectError: false,
			expectNil:   false,
		},
		{
			name: "ollama provider with custom config",
			config: Config{
				Type:    ProviderTypeOllama,
				BaseURL: "http://localhost:11434",
				Model:   "llama3.1",
			},
			expectError: false,
			expectNil:   false,
		},
		{
			name: "openai provider",
			config: Config{
				Type:    ProviderTypeOpenAI,
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
				APIKey:  "test-key",
			},
			expectError: false,
			expectNil:   false,
		},
		{
			name: "anthropic provider",
			config: Config{
				Type:    ProviderTypeAnthropic,
				BaseURL: "https://api.anthropic.com",
				Model:   "claude-sonnet-4-5-20250929",
				APIKey:  "test-key",
			},
			expectError: false,
			expectNil:   false,
		},
		{
			name: "unknown provider type",
			config: Config{
				Type:    ProviderType("unknown"),
				BaseURL: "http://localhost",
				Model:   "test",
			},
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.config)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check nil expectation
			if tt.expectNil && provider != nil {
				t.Error("expected nil provider, got non-nil")
			}
			if !tt.expectNil && provider == nil {
				t.Error("expected non-nil provider, got nil")
			}

			// For successful Ollama provider creation, verify it implements the interface
			if !tt.expectError && provider != nil {
				// Verify provider can be used as Provider interface
				var _ model.Provider = provider
			}
		})
	}
}

// TestFactoryReturnsOllamaProvider verifies that the factory returns an actual OllamaProvider
func TestFactoryReturnsOllamaProvider(t *testing.T) {
	cfg := Config{
		Type:    ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
		Model:   "llama3.1",
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Type assertion to verify it's actually an OllamaProvider
	_, ok := provider.(*OllamaProvider)
	if !ok {
		t.Errorf("expected *OllamaProvider, got %T", provider)
	}
}

// Note: Testing invalid URL scenarios is challenging because url.Parse is very permissive.
// Invalid URL handling is primarily tested at the ollama.Client level.
// The factory's responsibility is to correctly dispatch to the right provider constructor,
// which is tested in TestNewProvider.
