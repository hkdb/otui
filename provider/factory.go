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
//   - ProviderTypeOllama: Local Ollama server
//   - ProviderTypeOpenRouter: OpenRouter API
//   - ProviderTypeOpenAI: OpenAI API
//   - ProviderTypeOpenAICompatible: Any OpenAI API compatible server (llama.cpp, vLLM, etc.)
//   - ProviderTypeAnthropic: Anthropic API
//
// Returns an error if:
//   - The provider type is unknown
//   - The provider-specific constructor fails (e.g., missing API key)
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
// Example (OpenAI):
//
//	cfg := provider.Config{
//	    Type:    provider.ProviderTypeOpenAI,
//	    BaseURL: "https://api.openai.com/v1",
//	    Model:   "gpt-4",
//	    APIKey:  "sk-...",
//	}
//	p, err := provider.NewProvider(cfg)
func NewProvider(cfg Config) (model.Provider, error) {
	switch cfg.Type {
	case ProviderTypeOllama:
		return NewOllamaProvider(cfg.BaseURL, cfg.Model, cfg.APIKey)
	case ProviderTypeOpenRouter:
		return NewOpenRouterProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
	case ProviderTypeOpenAI:
		return NewOpenAIProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
	case ProviderTypeOpenAICompatible:
		return NewOpenAICompatibleProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
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
//   - "openai-compatible" → ProviderTypeOpenAICompatible
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
	case "openai-compatible":
		return ProviderTypeOpenAICompatible
	case "anthropic":
		return ProviderTypeAnthropic
	default:
		// Fallback: pass ID as-is (factory will return error)
		return ProviderType(id)
	}
}
