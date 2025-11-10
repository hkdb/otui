package model

import (
	"context"
	"otui/config"
	"otui/ollama"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// Provider abstracts LLM provider implementations (Ollama, OpenAI, Anthropic)
// using provider-agnostic types from OTUI's model layer.
//
// This interface is defined in the model package (not provider package) to avoid
// import cycles: provider implementations can import model, and model can use the
// Provider interface without importing the provider package.
type Provider interface {
	// Chat sends messages and streams responses back via callback.
	Chat(ctx context.Context, messages []Message, callback StreamCallback) error

	// ChatWithTools sends messages with available tools and streams responses.
	ChatWithTools(ctx context.Context, messages []Message, tools []mcptypes.Tool, callback StreamCallback) error

	// ListModels returns available models for this provider.
	ListModels(ctx context.Context) ([]ollama.ModelInfo, error)

	// GetModel returns the currently selected model name (InternalName for API calls).
	// For OpenRouter, this returns the full name with vendor prefix (e.g., "qwen/qwen3-coder:free").
	GetModel() string

	// GetDisplayName returns the model name formatted for UI display.
	// For OpenRouter, this strips the vendor prefix (e.g., "qwen/qwen3-coder:free" → "qwen3-coder:free").
	// For Ollama, this returns the same value as GetModel().
	GetDisplayName() string

	// SetModel changes the active model.
	SetModel(model string)

	// Ping checks if the provider is reachable.
	Ping(ctx context.Context) error
}

// StreamCallback is called for each chunk of streamed response.
type StreamCallback func(chunk string, toolCalls []ToolCall) error

// ShouldBlockOnOllamaValidation returns true if Ollama validation errors should prevent saving settings.
// This is business logic that determines when Ollama must be reachable vs when it's optional.
//
// Ollama validation should only block if:
// 1. Ollama is the default provider (user intends to use it), AND
// 2. Ollama is enabled in the provider list
//
// OpenRouter-only users should be able to save settings even when Ollama is unreachable.
func ShouldBlockOnOllamaValidation(cfg *config.Config) bool {
	// If Ollama is not the default provider, validation failure should not block
	if cfg.DefaultProvider != "ollama" && cfg.DefaultProvider != "" {
		return false
	}

	// Check if Ollama is explicitly disabled in providers list
	for _, p := range cfg.Providers {
		if p.ID == "ollama" && !p.Enabled {
			return false
		}
	}

	// Ollama is default or no default set - validation should block
	return true
}
