package model

import (
	"context"
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

	// GetModelMetadata returns metadata for a specific model (context window, etc.)
	// Queries the provider's API where possible (may do network I/O, so never
	// call it from View/Update - use Model.RefreshModelMetadataCmd) and falls
	// back to hardcoded defaults.
	GetModelMetadata(ctx context.Context, modelName string) (ModelMetadata, error)

	// FallbackModelMetadata returns metadata from hardcoded tables only.
	// Must not do any I/O - it's used for rendering until the real
	// metadata has been fetched.
	FallbackModelMetadata(modelName string) ModelMetadata
}

// ModelMetadata contains metadata about a model's capabilities
type ModelMetadata struct {
	ContextWindow int
	MaxOutput     int
	SupportsTools bool
	Source        string // Where ContextWindow came from (for debug logs), e.g. "ollama:/api/ps", "fallback"
}

// MetadataSourceFallback marks metadata that came from hardcoded tables
const MetadataSourceFallback = "fallback"

// StreamCallback is called for each chunk of streamed response.
type StreamCallback func(chunk string, toolCalls []ToolCall) error
