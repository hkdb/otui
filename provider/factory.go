package provider

import (
	"fmt"
	"otui/model"
)

// NewProvider creates a provider based on configuration.
//
// This is the centralized factory function for creating any provider type.
// It handles dispatching to the appropriate provider constructor based on
// the Config.Type field.
//
// Supported provider types:
//   - ProviderTypeOllama: Local Ollama server (implemented)
//   - ProviderTypeOpenAI: OpenAI API (not yet implemented)
//   - ProviderTypeAnthropic: Anthropic API (not yet implemented)
//
// Returns an error if:
//   - The provider type is unknown
//   - The provider type is not yet implemented
//   - The provider-specific constructor fails (e.g., invalid URL)
//
// Example (Ollama):
//
//	cfg := provider.Config{
//	    Type:    provider.ProviderTypeOllama,
//	    BaseURL: "http://localhost:11434",
//	    Model:   "llama3.1",
//	}
//	p, err := provider.NewProvider(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Example (OpenAI - not yet implemented):
//
//	cfg := provider.Config{
//	    Type:    provider.ProviderTypeOpenAI,
//	    BaseURL: "https://api.openai.com/v1",
//	    Model:   "gpt-4",
//	    APIKey:  "sk-...",
//	}
//	p, err := provider.NewProvider(cfg)
//	// Returns error: "OpenAI provider not yet implemented"
func NewProvider(cfg Config) (model.Provider, error) {
	switch cfg.Type {
	case ProviderTypeOllama:
		return NewOllamaProvider(cfg.BaseURL, cfg.Model)
	case ProviderTypeOpenRouter:
		return NewOpenRouterProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
	case ProviderTypeOpenAI:
		return NewOpenAIProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
	case ProviderTypeAnthropic:
		return NewAnthropicProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// MapProviderIDToType converts config provider ID to factory ProviderType.
//
// This handles the mapping between user-facing provider IDs (from config)
// and internal ProviderType constants used by the factory.
//
// Mappings:
//   - "ollama" → ProviderTypeOllama
//   - "openrouter" → ProviderTypeOpenAI (OpenRouter is OpenAI-compatible)
//   - "openai" → ProviderTypeOpenAI
//   - "anthropic" → ProviderTypeAnthropic
//
// For unknown IDs, returns the ID cast as ProviderType (factory will error).
func MapProviderIDToType(id string) ProviderType {
	switch id {
	case "ollama":
		return ProviderTypeOllama
	case "openrouter":
		return ProviderTypeOpenRouter
	case "openai":
		return ProviderTypeOpenAI
	case "anthropic":
		return ProviderTypeAnthropic
	default:
		// Fallback: pass ID as-is (factory will return error)
		return ProviderType(id)
	}
}
