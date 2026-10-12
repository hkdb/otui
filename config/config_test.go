package config

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- defaults.go ---

func TestDefaultSystemConfig(t *testing.T) {
	cfg := DefaultSystemConfig()
	if cfg == nil {
		t.Fatal("DefaultSystemConfig() returned nil")
	}
	if cfg.DataDirectory != "~/.local/share/otui" {
		t.Errorf("expected data directory '~/.local/share/otui', got %q", cfg.DataDirectory)
	}
}

func TestDefaultUserConfig(t *testing.T) {
	cfg := DefaultUserConfig()
	if cfg == nil {
		t.Fatal("DefaultUserConfig() returned nil")
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DefaultProvider", cfg.DefaultProvider, "ollama"},
		{"DefaultModel", cfg.DefaultModel, "llama3.1:latest"},
		{"LastUsedProvider", cfg.LastUsedProvider, "ollama"},
		{"OllamaHost", cfg.Ollama.Host, "http://localhost:11434"},
		{"CredentialStorage", cfg.Security.CredentialStorage, "plaintext"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	if cfg.PluginsEnabled {
		t.Error("expected PluginsEnabled to be false")
	}
	if !cfg.RequireApproval {
		t.Error("expected RequireApproval to be true")
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", cfg.MaxIterations)
	}
	if !cfg.EnableMultiStep {
		t.Error("expected EnableMultiStep to be true")
	}
	if cfg.Providers == nil {
		t.Error("expected Providers to be non-nil")
	}
	if cfg.ModelContextOverrides == nil {
		t.Error("expected ModelContextOverrides to be non-nil")
	}
}

func TestGenerateSystemConfigTemplate(t *testing.T) {
	tmpl := GenerateSystemConfigTemplate()
	if !strings.Contains(tmpl, "data_directory") {
		t.Error("template missing 'data_directory'")
	}
	if !strings.Contains(tmpl, "~/.local/share/otui") {
		t.Error("template missing default data directory path")
	}
}

func TestGenerateUserConfigTemplate(t *testing.T) {
	tmpl := GenerateUserConfigTemplate()
	checks := []string{
		"default_provider",
		"default_model",
		"plugins_enabled",
		"[security]",
		"[ollama]",
		"[compaction]",
	}
	for _, check := range checks {
		if !strings.Contains(tmpl, check) {
			t.Errorf("template missing %q", check)
		}
	}
}

// --- credentials.go ---

func TestCredentialStore(t *testing.T) {
	store := NewCredentialStore(SecurityPlainText, "")
	if store == nil {
		t.Fatal("NewCredentialStore returned nil")
	}
	if store.GetMethod() != SecurityPlainText {
		t.Errorf("expected method %q, got %q", SecurityPlainText, store.GetMethod())
	}

	t.Run("SetAndGet", func(t *testing.T) {
		_ = store.Set("openai", "sk-test-key")
		got := store.Get("openai")
		if got != "sk-test-key" {
			t.Errorf("expected 'sk-test-key', got %q", got)
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		got := store.Get("nonexistent")
		if got != "" {
			t.Errorf("expected empty string for missing key, got %q", got)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		_ = store.Set("todelete", "value")
		_ = store.Delete("todelete")
		got := store.Get("todelete")
		if got != "" {
			t.Errorf("expected empty after delete, got %q", got)
		}
	})

	t.Run("PluginSetAndGet", func(t *testing.T) {
		_ = store.SetPlugin("mcp-fs", "api_key", "secret123")
		got := store.GetPlugin("mcp-fs", "api_key")
		if got != "secret123" {
			t.Errorf("expected 'secret123', got %q", got)
		}
	})

	t.Run("DeletePluginAll", func(t *testing.T) {
		_ = store.SetPlugin("mcp-mem", "key1", "val1")
		_ = store.SetPlugin("mcp-mem", "key2", "val2")
		_ = store.DeletePluginAll("mcp-mem")
		if store.GetPlugin("mcp-mem", "key1") != "" {
			t.Error("expected plugin key1 to be deleted")
		}
		if store.GetPlugin("mcp-mem", "key2") != "" {
			t.Error("expected plugin key2 to be deleted")
		}
	})
}

// --- plugins.go ---

func TestPluginsConfig(t *testing.T) {
	pc := &PluginsConfig{Plugins: make(map[string]PluginConfigEntry)}

	t.Run("GetPluginEnabled_missing", func(t *testing.T) {
		if pc.GetPluginEnabled("nonexistent") {
			t.Error("expected false for missing plugin")
		}
	})

	t.Run("SetPluginEnabled", func(t *testing.T) {
		pc.SetPluginEnabled("mcp-fs", true)
		if !pc.GetPluginEnabled("mcp-fs") {
			t.Error("expected plugin to be enabled")
		}
		pc.SetPluginEnabled("mcp-fs", false)
		if pc.GetPluginEnabled("mcp-fs") {
			t.Error("expected plugin to be disabled")
		}
	})

	t.Run("GetPluginConfig_missing", func(t *testing.T) {
		cfg := pc.GetPluginConfig("nonexistent")
		if cfg == nil {
			t.Error("expected non-nil map for missing plugin")
		}
		if len(cfg) != 0 {
			t.Error("expected empty map for missing plugin")
		}
	})

	t.Run("SetPluginConfig", func(t *testing.T) {
		configMap := map[string]string{"host": "localhost", "port": "8080"}
		pc.SetPluginConfig("mcp-db", configMap)
		got := pc.GetPluginConfig("mcp-db")
		if got["host"] != "localhost" {
			t.Errorf("expected host 'localhost', got %q", got["host"])
		}
	})

	t.Run("DeletePlugin", func(t *testing.T) {
		pc.SetPluginEnabled("to-delete", true)
		pc.DeletePlugin("to-delete")
		if pc.GetPluginEnabled("to-delete") {
			t.Error("expected plugin to be deleted")
		}
	})

	t.Run("DeletePlugin_nil_map", func(t *testing.T) {
		nilPC := &PluginsConfig{}
		nilPC.DeletePlugin("anything") // should not panic
	})
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"API_KEY", true},
		{"api_key", true},
		{"TOKEN", true},
		{"access_token", true},
		{"SECRET", true},
		{"PASSWORD", true},
		{"AUTH_HEADER", true},
		{"CREDENTIAL", true},
		{"BEARER_TOKEN", true},
		{"host", false},
		{"port", false},
		{"base_url", false},
		{"model_name", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := isSensitiveKey(tt.key)
			if got != tt.want {
				t.Errorf("isSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// --- encryption.go ---

func TestEncryptDecryptAESGCM(t *testing.T) {
	// Generate a random 32-byte key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	t.Run("RoundTrip", func(t *testing.T) {
		plaintext := []byte("hello, this is a secret message!")
		ciphertext, err := encryptAESGCM(plaintext, key)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		decrypted, err := decryptAESGCM(ciphertext, key)
		if err != nil {
			t.Fatalf("decrypt failed: %v", err)
		}

		if string(decrypted) != string(plaintext) {
			t.Errorf("decrypted text %q does not match original %q", decrypted, plaintext)
		}
	})

	t.Run("CiphertextTooShort", func(t *testing.T) {
		_, err := decryptAESGCM([]byte("short"), key)
		if err == nil {
			t.Error("expected error for short ciphertext")
		}
		if !strings.Contains(err.Error(), "ciphertext too short") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("EmptyPlaintext", func(t *testing.T) {
		ciphertext, err := encryptAESGCM([]byte{}, key)
		if err != nil {
			t.Fatalf("encrypt empty failed: %v", err)
		}
		decrypted, err := decryptAESGCM(ciphertext, key)
		if err != nil {
			t.Fatalf("decrypt empty failed: %v", err)
		}
		if len(decrypted) != 0 {
			t.Errorf("expected empty decrypted, got %d bytes", len(decrypted))
		}
	})
}

// --- paths.go ---

func TestExpandPath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		got := ExpandPath("")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("TildeExpansion", func(t *testing.T) {
		got := ExpandPath("~/test/path")
		if strings.HasPrefix(got, "~/") {
			t.Errorf("tilde was not expanded: %q", got)
		}
		if !strings.HasSuffix(got, "test/path") {
			t.Errorf("path suffix lost: %q", got)
		}
	})

	t.Run("NoTilde", func(t *testing.T) {
		got := ExpandPath("/absolute/path")
		if got != "/absolute/path" {
			t.Errorf("absolute path changed: %q", got)
		}
	})
}

func TestGetSettingsFilePath(t *testing.T) {
	path := GetSettingsFilePath()
	if !strings.Contains(path, "settings.toml") {
		t.Errorf("expected path to contain 'settings.toml', got %q", path)
	}
	if !strings.Contains(path, "otui") {
		t.Errorf("expected path to contain 'otui', got %q", path)
	}
}

func TestUpdateProviderFieldOpenAICompatible(t *testing.T) {
	dataDir := t.TempDir()
	const customURL = "http://gpu-box:8000/v1"

	// Same order the provider settings screen saves fields in
	if err := UpdateProviderField(dataDir, "openai-compatible", "base_url", customURL); err != nil {
		t.Fatalf("update base_url: %v", err)
	}
	if err := UpdateProviderField(dataDir, "openai-compatible", "enabled", "true"); err != nil {
		t.Fatalf("update enabled: %v", err)
	}

	cfg, err := LoadUserConfig(dataDir)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}

	var found *ProviderConfig
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == "openai-compatible" {
			if found != nil {
				t.Fatal("duplicate openai-compatible provider entries")
			}
			found = &cfg.Providers[i]
		}
	}
	if found == nil {
		t.Fatal("openai-compatible provider not saved")
	}
	if !found.Enabled {
		t.Error("expected provider to be enabled")
	}
	if found.BaseURL != customURL {
		t.Errorf("BaseURL = %q, want %q (enabled must not reset it)", found.BaseURL, customURL)
	}
	if got := GetProviderBaseURL(cfg.Providers, "openai-compatible"); got != customURL {
		t.Errorf("GetProviderBaseURL = %q, want %q", got, customURL)
	}

	// Empty value falls back to the default endpoint
	if err := UpdateProviderField(dataDir, "openai-compatible", "base_url", ""); err != nil {
		t.Fatalf("update empty base_url: %v", err)
	}
	cfg, _ = LoadUserConfig(dataDir)
	if got := GetProviderBaseURL(cfg.Providers, "openai-compatible"); got != DefaultOpenAICompatibleBaseURL {
		t.Errorf("GetProviderBaseURL = %q, want default %q", got, DefaultOpenAICompatibleBaseURL)
	}

	// base_url is only valid for the compatible provider
	if err := UpdateProviderField(dataDir, "openai", "base_url", customURL); err == nil {
		t.Error("expected error setting base_url on openai provider")
	}
}

// buggyWizardConfig is what the old "Custom Settings" wizard wrote
const buggyWizardConfig = `default_provider = "ollama"
default_model = "llama3.1:latest"
last_used_provider = "ollama"
plugins_enabled = false
require_approval = false
max_iterations = 0
enable_multi_step = false
notify_on_complete = false

[ollama]
  host = "http://localhost:11434"

[security]
  credential_storage = "plaintext"

[compaction]
  auto_compact = false
  auto_compact_threshold = 0.0
  keep_percentage = 0.0
  warn_at_percentage = 0.0
`

func TestLoadUserConfigRepairsWizardDefaults(t *testing.T) {
	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(buggyWizardConfig), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	check := func(t *testing.T, cfg *UserConfig) {
		t.Helper()
		if !cfg.RequireApproval {
			t.Error("RequireApproval = false, want true")
		}
		if !cfg.EnableMultiStep {
			t.Error("EnableMultiStep = false, want true")
		}
		if cfg.MaxIterations != 10 {
			t.Errorf("MaxIterations = %d, want 10", cfg.MaxIterations)
		}
		if cfg.Compaction.AutoCompactThreshold != 0.75 || cfg.Compaction.KeepPercentage != 0.50 || cfg.Compaction.WarnAtPercentage != 0.85 {
			t.Errorf("Compaction = %+v, want default thresholds", cfg.Compaction)
		}
		if cfg.DefaultModel != "llama3.1:latest" || cfg.Ollama.Host != "http://localhost:11434" {
			t.Errorf("unrelated settings changed: model=%q host=%q", cfg.DefaultModel, cfg.Ollama.Host)
		}
	}

	cfg, err := LoadUserConfig(dataDir)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	check(t, cfg)

	// The repair must be persisted to disk
	onDisk, err := LoadUserConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadUserConfigFromPath: %v", err)
	}
	check(t, onDisk)
}

func TestLoadUserConfigKeepsDeliberateApprovalOff(t *testing.T) {
	dataDir := t.TempDir()
	content := strings.NewReplacer(
		"max_iterations = 0", "max_iterations = 10",
		"enable_multi_step = false", "enable_multi_step = true",
	).Replace(buggyWizardConfig)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadUserConfig(dataDir)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.RequireApproval {
		t.Error("RequireApproval = true, want false (user's explicit choice)")
	}
}
