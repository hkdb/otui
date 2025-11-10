package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"otui/config"
)

// ProviderSettingsState manages the provider configuration sub-screen state.
// This screen shows tabs for each provider (Ollama, OpenRouter, etc.) with
// provider-specific fields (host, API key, enabled status).
type ProviderSettingsState struct {
	// Visibility
	visible bool

	// Tab selection
	selectedProviderID string // "ollama", "openrouter", "anthropic", "openai"

	// Field selection
	selectedFieldIdx int // 0=host/apikey, 1=enabled
	editMode         bool
	editInput        textinput.Model

	// UI state cache (holds field values for ALL providers before save)
	// Key = providerID ("ollama", "openrouter", "openai", "anthropic")
	currentFieldsMap map[string][]ProviderField

	// Change tracking
	hasChanges bool
	saveError  string
}

// Provider tab order (for navigation)
var providerTabs = []string{"ollama", "openrouter", "openai", "anthropic"}

// Provider display names
var providerNames = map[string]string{
	"ollama":     "Ollama",
	"openrouter": "OpenRouter",
	"openai":     "OpenAI",
	"anthropic":  "Anthropic",
}

// ProviderFieldType identifies the type of provider field
type ProviderFieldType int

const (
	ProviderFieldTypeHost ProviderFieldType = iota
	ProviderFieldTypeAPIKey
	ProviderFieldTypeEnabled
)

// ProviderField represents a single field in the provider settings
type ProviderField struct {
	Label string
	Value string
	Type  ProviderFieldType
}

// getProviderFields returns field list for a provider
func (p *ProviderSettingsState) getProviderFields(providerID string, cfg *config.Config) []ProviderField {
	switch providerID {
	case "ollama":
		return []ProviderField{
			{Label: "Ollama Host", Value: cfg.OllamaHost, Type: ProviderFieldTypeHost},
			{Label: "Enabled", Value: p.getProviderEnabled(cfg, "ollama"), Type: ProviderFieldTypeEnabled},
		}
	case "openrouter", "anthropic", "openai":
		apiKey := ""
		if cfg.CredentialStore != nil {
			apiKey = cfg.CredentialStore.Get(providerID)
		}
		return []ProviderField{
			{Label: "API Key", Value: p.maskAPIKey(apiKey), Type: ProviderFieldTypeAPIKey},
			{Label: "Enabled", Value: p.getProviderEnabled(cfg, providerID), Type: ProviderFieldTypeEnabled},
		}
	default:
		return []ProviderField{}
	}
}

// getProviderEnabled checks if provider is enabled in config
func (p *ProviderSettingsState) getProviderEnabled(cfg *config.Config, providerID string) string {
	for _, prov := range cfg.Providers {
		if prov.ID == providerID {
			if prov.Enabled {
				return "true"
			}
			return "false"
		}
	}

	// Fallback for old configs: if Providers list is empty, assume Ollama is enabled
	if len(cfg.Providers) == 0 && providerID == "ollama" {
		return "true"
	}

	return "false"
}

// maskAPIKey masks API key for display
func (p *ProviderSettingsState) maskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) < 8 {
		return "***"
	}
	return key[:3] + strings.Repeat("*", len(key)-7) + key[len(key)-4:]
}
