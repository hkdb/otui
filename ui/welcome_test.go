package ui

import (
	"testing"

	"otui/config"
)

func TestWelcomeBuildUserConfigKeepsDefaults(t *testing.T) {
	m := NewWelcomeModel(config.DefaultKeybindings())
	m.securityMethod = config.SecurityPlainText
	m.defaultProvider = "openai-compatible"
	m.defaultModel = "qwen3-8b"
	m.ollamaHost = "http://localhost:11434"
	for i, p := range m.providers {
		if p.ID == "openai-compatible" {
			m.providerCheckboxes[i] = true
		}
	}

	cfg := m.buildUserConfig()
	defaults := config.DefaultUserConfig()

	if !cfg.RequireApproval {
		t.Error("RequireApproval = false, want true")
	}
	if cfg.EnableMultiStep != defaults.EnableMultiStep || cfg.MaxIterations != defaults.MaxIterations {
		t.Errorf("multi-step = %v/%d, want %v/%d", cfg.EnableMultiStep, cfg.MaxIterations, defaults.EnableMultiStep, defaults.MaxIterations)
	}
	if cfg.Compaction != defaults.Compaction {
		t.Errorf("Compaction = %+v, want %+v", cfg.Compaction, defaults.Compaction)
	}

	// Wizard choices are applied
	if cfg.DefaultProvider != "openai-compatible" || cfg.LastUsedProvider != "openai-compatible" || cfg.DefaultModel != "qwen3-8b" {
		t.Errorf("provider/model = %q/%q/%q", cfg.DefaultProvider, cfg.LastUsedProvider, cfg.DefaultModel)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "openai-compatible" || !cfg.Providers[0].Enabled {
		t.Errorf("Providers = %+v, want only enabled openai-compatible", cfg.Providers)
	}
	if cfg.Security.CredentialStorage != string(config.SecurityPlainText) {
		t.Errorf("CredentialStorage = %q", cfg.Security.CredentialStorage)
	}
}
