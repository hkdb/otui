package config

import (
	"fmt"
)

// DefaultOpenAICompatibleBaseURL is the default endpoint for the OpenAI
// compatible provider (llama.cpp's llama-server default)
const DefaultOpenAICompatibleBaseURL = "http://localhost:8080/v1"

// GetProviderBaseURL returns the configured base URL for a provider,
// falling back to the provider's default
func GetProviderBaseURL(providers []ProviderConfig, providerID string) string {
	for _, p := range providers {
		if p.ID == providerID && p.BaseURL != "" {
			return p.BaseURL
		}
	}
	return getProviderDefaultBaseURL(providerID)
}

// UpdateProviderField updates a single provider configuration field.
// This is the business logic layer for provider settings.
//
// Fields:
//   - Ollama: "host", "enabled"
//   - OpenAI Compatible: "base_url", "enabled"
//   - Cloud providers: "enabled"
//
// API keys are credentials - use SaveProviderAPIKeys with the live store.
func UpdateProviderField(dataDir, providerID, fieldName, value string) error {
	// Load existing config
	cfg, err := LoadUserConfig(dataDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update field based on provider and field name
	switch providerID {
	case "ollama":
		switch fieldName {
		case "host":
			cfg.Ollama.Host = value

			// Sync to [[providers]] array for consistency
			for i := range cfg.Providers {
				if cfg.Providers[i].ID == "ollama" {
					cfg.Providers[i].BaseURL = value
					break
				}
			}
		case "enabled":
			if err := updateProviderEnabled(cfg, providerID, value == "true"); err != nil {
				return err
			}
			// Fall through to SaveUserConfig() below
		default:
			return fmt.Errorf("unknown field for ollama: %s", fieldName)
		}

	case "openrouter", "anthropic", "openai", "openai-compatible":
		switch fieldName {
		case "base_url":
			if providerID != "openai-compatible" {
				return fmt.Errorf("unknown field for %s: %s", providerID, fieldName)
			}
			if value == "" {
				value = getProviderDefaultBaseURL(providerID)
			}
			findOrAddProvider(cfg, providerID).BaseURL = value
			// Fall through to SaveUserConfig() below
		case "enabled":
			if err := updateProviderEnabled(cfg, providerID, value == "true"); err != nil {
				return err
			}
			// Fall through to SaveUserConfig() below
		default:
			return fmt.Errorf("unknown field for %s: %s", providerID, fieldName)
		}

	default:
		return fmt.Errorf("unknown provider: %s", providerID)
	}

	// Save updated config
	if err := SaveUserConfig(cfg, dataDir); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// SaveProviderAPIKeys stores changed API keys (provider ID -> key) in the
// running app's credential store and saves it to dataDir. Using the live
// store matters for passphrase protected credentials, which a freshly loaded
// store couldn't decrypt. Writes nothing if no key changed.
func SaveProviderAPIKeys(store *CredentialStore, dataDir string, keys map[string]string) error {
	if store == nil {
		return fmt.Errorf("no credential store")
	}

	changed := false
	for providerID, key := range keys {
		if store.Get(providerID) == key {
			continue
		}
		if err := store.Set(providerID, key); err != nil {
			return fmt.Errorf("failed to set API key for %s: %w", providerID, err)
		}
		changed = true
	}
	if !changed {
		return nil
	}

	if err := store.Save(dataDir); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}
	return nil
}

// updateProviderEnabled updates the enabled status of a provider
func updateProviderEnabled(cfg *UserConfig, providerID string, enabled bool) error {
	findOrAddProvider(cfg, providerID).Enabled = enabled
	return nil
}

// findOrAddProvider returns the provider entry with the given ID, adding a
// disabled entry with default values if it isn't in the list yet
func findOrAddProvider(cfg *UserConfig, providerID string) *ProviderConfig {
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == providerID {
			return &cfg.Providers[i]
		}
	}

	cfg.Providers = append(cfg.Providers, ProviderConfig{
		ID:      providerID,
		Name:    getProviderDisplayName(providerID),
		Enabled: false,
		BaseURL: getProviderDefaultBaseURL(providerID),
	})
	return &cfg.Providers[len(cfg.Providers)-1]
}

// getProviderDisplayName returns the display name for a provider
func getProviderDisplayName(providerID string) string {
	switch providerID {
	case "ollama":
		return "Ollama"
	case "openrouter":
		return "OpenRouter"
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "openai-compatible":
		return "OpenAI Compatible"
	default:
		return providerID
	}
}

// getProviderDefaultBaseURL returns the default base URL for a provider
func getProviderDefaultBaseURL(providerID string) string {
	switch providerID {
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai":
		return "https://api.openai.com/v1"
	case "openai-compatible":
		return DefaultOpenAICompatibleBaseURL
	default:
		return ""
	}
}
