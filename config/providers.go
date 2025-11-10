package config

import (
	"fmt"
)

// UpdateProviderField updates a single provider configuration field.
// This is the business logic layer for provider settings.
//
// Fields:
//   - Ollama: "host", "enabled"
//   - Cloud providers: "apikey", "enabled"
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

	case "openrouter", "anthropic", "openai":
		switch fieldName {
		case "apikey":
			// Update API key in credentials
			// Note: We need to load the full Config to access CredentialStore
			fullCfg, err := Load()
			if err != nil {
				return fmt.Errorf("failed to load full config for credential update: %w", err)
			}

			if fullCfg.CredentialStore != nil {
				if err := fullCfg.CredentialStore.Set(providerID, value); err != nil {
					return fmt.Errorf("failed to set API key: %w", err)
				}

				// Save credentials to disk
				if err := fullCfg.CredentialStore.Save(dataDir); err != nil {
					return fmt.Errorf("failed to persist credentials: %w", err)
				}
			}
			// Don't save UserConfig for API key changes (already saved credentials)
			return nil

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

// updateProviderEnabled updates the enabled status of a provider
func updateProviderEnabled(cfg *UserConfig, providerID string, enabled bool) error {
	// Find provider in list
	found := false
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == providerID {
			cfg.Providers[i].Enabled = enabled
			found = true
			break
		}
	}

	// If provider not in list, add it (for Ollama or new providers)
	if !found {
		cfg.Providers = append(cfg.Providers, ProviderConfig{
			ID:      providerID,
			Name:    getProviderDisplayName(providerID),
			Enabled: enabled,
			BaseURL: getProviderDefaultBaseURL(providerID),
		})
	}

	return nil
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
	default:
		return ""
	}
}
