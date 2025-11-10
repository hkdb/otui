// Package provider defines the abstract interface for LLM providers.
//
// OTUI supports multiple LLM providers (Ollama, OpenAI, Anthropic) through
// a common Provider interface. This allows the UI and business logic to
// remain provider-agnostic, making it easy to add support for new LLM providers
// without changing the core application logic.
//
// # Why Provider Abstraction?
//
// The provider abstraction exists to:
//   - Enable multi-provider support (local Ollama, cloud APIs like OpenAI/Anthropic)
//   - Isolate provider-specific types from OTUI's core types
//   - Allow easy testing with mock providers
//   - Make adding new providers straightforward (just implement the interface)
//
// # Type Conversions
//
// The provider layer handles all type conversions between OTUI's provider-agnostic
// types and provider-specific types. See the conversion functions in conversions.go:
//   - ConvertToOllamaMessages / ConvertFromOllamaMessages
//   - ConvertToProviderToolCalls / ConvertFromProviderToolCalls
//
// # Architecture
//
//   - provider.Provider defines the contract (interface)
//   - provider.OllamaProvider implements for Ollama (implemented)
//   - provider.OpenAIProvider for OpenAI (future)
//   - provider.AnthropicProvider for Anthropic (future)
//   - provider.NewProvider() factory creates providers from config
//
// # Usage
//
//	cfg := provider.Config{
//	    Type: provider.ProviderTypeOllama,
//	    BaseURL: "http://localhost:11434",
//	    Model: "llama3.1",
//	}
//	p, err := provider.NewProvider(cfg)
//	if err != nil {
//	    // handle error
//	}
//	err = p.Chat(ctx, messages, callback)
package provider

// Note: The Provider interface and StreamCallback are defined in the model package
// (model/provider.go) to avoid import cycles. This package implements model.Provider.

// ProviderType identifies the provider implementation.
type ProviderType string

const (
	ProviderTypeOllama     ProviderType = "ollama"
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeAnthropic  ProviderType = "anthropic"
)

// Config holds provider-specific configuration.
type Config struct {
	Type    ProviderType
	BaseURL string
	Model   string
	APIKey  string // For OpenAI/Anthropic (unused for Ollama)
}
