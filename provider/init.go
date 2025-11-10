package provider

import (
	"otui/config"
	"otui/model"
)

// InitializeProviders creates ALL provider instances for the application.
//
// This function is the single entry point for provider initialization.
// It handles:
//   - Creating the Ollama provider (if configured)
//   - Creating all enabled cloud providers (OpenRouter, Anthropic, etc.)
//   - Loading API keys from credential store
//   - Mapping provider IDs to provider types
//   - Graceful degradation (logs warnings but doesn't fail)
//
// The provider package owns the complete provider lifecycle, so all
// initialization logic lives here, not in config or ui packages.
//
// Parameters:
//   - cfg: The application configuration containing provider settings
//
// Returns:
//   - map[string]model.Provider: Map of provider ID to provider instance
//
// The map will always include an "ollama" entry (may be nil for offline mode)
// plus any enabled cloud providers.
//
// Example:
//
//	providers := provider.InitializeProviders(cfg)
//	// providers = {"ollama": ..., "openrouter": ..., "anthropic": ...}
func InitializeProviders(cfg *config.Config) map[string]model.Provider {
	providers := make(map[string]model.Provider)

	// Initialize Ollama provider first (special case - always attempted)
	ollamaProvider := initializeOllama(cfg)
	if ollamaProvider != nil {
		providers["ollama"] = ollamaProvider
		if config.Debug {
			config.DebugLog.Printf("[Provider] Initialized Ollama provider")
		}
	} else {
		if config.Debug {
			config.DebugLog.Printf("[Provider] Ollama provider initialization failed (offline mode)")
		}
	}

	// Initialize cloud providers from config
	for _, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}

		// Get API key from credential store
		apiKey := ""
		if cfg.CredentialStore != nil {
			apiKey = cfg.CredentialStore.Get(providerCfg.ID)
		}

		// Map provider ID to Type (handles openrouter→openai mapping)
		providerType := MapProviderIDToType(providerCfg.ID)

		// Create provider via factory
		p, err := NewProvider(Config{
			Type:    providerType,
			BaseURL: providerCfg.BaseURL,
			APIKey:  apiKey,
			Model:   "", // Will be set when session loads
		})

		if err != nil {
			// Log warning but don't fail - allow app to start
			if config.Debug {
				config.DebugLog.Printf("[Provider] Warning: failed to initialize provider %s: %v", providerCfg.ID, err)
			}
			continue
		}

		providers[providerCfg.ID] = p
		if config.Debug {
			config.DebugLog.Printf("[Provider] Initialized provider: %s (type: %s)", providerCfg.ID, providerType)
		}
	}

	return providers
}

// initializeOllama creates the Ollama provider instance.
// Returns nil if initialization fails (allows offline mode).
func initializeOllama(cfg *config.Config) model.Provider {
	providerCfg := Config{
		Type:    ProviderTypeOllama,
		BaseURL: cfg.OllamaURL(),
		Model:   cfg.Model(),
	}

	p, err := NewProvider(providerCfg)
	if err != nil {
		if config.Debug {
			config.DebugLog.Printf("[Provider] Ollama provider creation failed: %v", err)
		}
		return nil
	}

	return p
}
