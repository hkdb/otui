package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv unsets all OTUI env configuration for a test
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{EnvDataDir, EnvProvider, EnvModel, EnvBaseURL, EnvAPIKey, EnvOllamaHost, EnvOllamaModel} {
		t.Setenv(name, "")
	}
}

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	clearEnv(t)
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestReadEnvConfig(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		want    *EnvConfig
		wantErr string
	}{
		{name: "nothing set", vars: nil},
		{
			name: "full new set",
			vars: map[string]string{EnvDataDir: "/d", EnvProvider: "openai-compatible", EnvModel: "m", EnvBaseURL: "http://h:8080/v1", EnvAPIKey: "k"},
			want: &EnvConfig{DataDir: "/d", Provider: "openai-compatible", Model: "m", BaseURL: "http://h:8080/v1", APIKey: "k"},
		},
		{
			name: "deprecated ollama aliases",
			vars: map[string]string{EnvDataDir: "/d", EnvOllamaHost: "http://o:11434", EnvOllamaModel: "qwen3:8b"},
			want: &EnvConfig{DataDir: "/d", Provider: "ollama", Model: "qwen3:8b", BaseURL: "http://o:11434"},
		},
		{
			name: "new variables win over aliases",
			vars: map[string]string{EnvDataDir: "/d", EnvProvider: "ollama", EnvModel: "new", EnvBaseURL: "http://new", EnvOllamaHost: "http://old", EnvOllamaModel: "old"},
			want: &EnvConfig{DataDir: "/d", Provider: "ollama", Model: "new", BaseURL: "http://new"},
		},
		{
			name: "aliases ignored for another provider",
			vars: map[string]string{EnvDataDir: "/d", EnvProvider: "openrouter", EnvModel: "x/y", EnvOllamaHost: "http://old"},
			want: &EnvConfig{DataDir: "/d", Provider: "openrouter", Model: "x/y", BaseURL: "https://openrouter.ai/api/v1"},
		},
		{
			name:    "partial legacy set",
			vars:    map[string]string{EnvOllamaHost: "http://o:11434", EnvOllamaModel: "qwen3:8b"},
			wantErr: "OTUI_DATA_DIR",
		},
		{
			name:    "missing model",
			vars:    map[string]string{EnvDataDir: "/d", EnvProvider: "openai"},
			wantErr: "OTUI_MODEL",
		},
		{
			name:    "unknown provider",
			vars:    map[string]string{EnvDataDir: "/d", EnvProvider: "llamacpp", EnvModel: "m"},
			wantErr: "unknown OTUI_PROVIDER",
		},
		{
			name:    "base url required for openai-compatible",
			vars:    map[string]string{EnvDataDir: "/d", EnvProvider: "openai-compatible", EnvModel: "m"},
			wantErr: "OTUI_BASE_URL is required",
		},
		{
			name: "cloud provider default base url",
			vars: map[string]string{EnvDataDir: "/d", EnvProvider: "anthropic", EnvModel: "claude-x"},
			want: &EnvConfig{DataDir: "/d", Provider: "anthropic", Model: "claude-x", BaseURL: "https://api.anthropic.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.vars)
			got, err := ReadEnvConfig()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// setupHome points HOME at a temp dir with an optional settings.toml
// pointing at settingsDataDir
func setupHome(t *testing.T, settingsDataDir string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "otui"), 0700); err != nil {
		t.Fatal(err)
	}
	if settingsDataDir != "" {
		settings := "data_directory = \"" + settingsDataDir + "\"\n"
		if err := os.WriteFile(GetSettingsFilePath(), []byte(settings), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestLoadWithEnvFreshHome(t *testing.T) {
	home := setupHome(t, "")
	envData := filepath.Join(home, "envdata")
	setEnv(t, map[string]string{
		EnvDataDir: envData, EnvProvider: "openai-compatible",
		EnvModel: "fake-llama", EnvBaseURL: "http://127.0.0.1:8080/v1",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir() != envData || cfg.DefaultProvider != "openai-compatible" || cfg.DefaultModel != "fake-llama" {
		t.Errorf("got dataDir=%q provider=%q model=%q", cfg.DataDir(), cfg.DefaultProvider, cfg.DefaultModel)
	}
	if GetProviderBaseURL(cfg.Providers, "openai-compatible") != "http://127.0.0.1:8080/v1" {
		t.Errorf("provider base url = %q", GetProviderBaseURL(cfg.Providers, "openai-compatible"))
	}
	enabled := false
	for _, p := range cfg.Providers {
		if p.ID == "openai-compatible" && p.Enabled {
			enabled = true
		}
	}
	if !enabled {
		t.Error("env provider not enabled")
	}
	if FileExists(GetSettingsFilePath()) {
		t.Error("settings.toml was created in env mode")
	}
}

func TestLoadEnvOverridesExistingProfile(t *testing.T) {
	home := setupHome(t, "") // set below
	otherData := filepath.Join(home, "other")
	envData := filepath.Join(home, "envdata")
	setupHomeSettings(t, otherData)

	// The env data dir has its own config with settings that must still apply
	if err := os.MkdirAll(envData, 0700); err != nil {
		t.Fatal(err)
	}
	profile := `default_provider = "openrouter"
default_model = "x/y"
default_system_prompt = "be brief"
require_approval = true
max_iterations = 7
enable_multi_step = true

[ollama]
host = "http://profile-host:11434"

[[providers]]
id = "ollama"
name = "Ollama"
enabled = false
base_url = "http://profile-host:11434"
`
	if err := os.WriteFile(filepath.Join(envData, "config.toml"), []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}

	setEnv(t, map[string]string{EnvDataDir: envData, EnvOllamaHost: "http://env-host:11434", EnvOllamaModel: "qwen3:8b"})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir() != envData {
		t.Errorf("data dir = %q, want env %q (not settings.toml's)", cfg.DataDir(), envData)
	}
	if cfg.OllamaHost != "http://env-host:11434" || cfg.DefaultModel != "qwen3:8b" || cfg.DefaultProvider != "ollama" {
		t.Errorf("env not applied: host=%q model=%q provider=%q", cfg.OllamaHost, cfg.DefaultModel, cfg.DefaultProvider)
	}
	for _, p := range cfg.Providers {
		if p.ID == "ollama" && (!p.Enabled || p.BaseURL != "http://env-host:11434") {
			t.Errorf("ollama provider = %+v, want enabled with env host", p)
		}
	}
	if cfg.DefaultSystemPrompt != "be brief" || cfg.MaxIterations != 7 {
		t.Errorf("profile settings lost: prompt=%q maxIter=%d", cfg.DefaultSystemPrompt, cfg.MaxIterations)
	}

	// Env values are never written back to the profile
	onDisk, err := LoadUserConfigFromPath(filepath.Join(envData, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.DefaultModel != "x/y" || onDisk.Ollama.Host != "http://profile-host:11434" {
		t.Errorf("env values leaked into config.toml: model=%q host=%q", onDisk.DefaultModel, onDisk.Ollama.Host)
	}
}

// setupHomeSettings writes settings.toml for the current HOME
func setupHomeSettings(t *testing.T, dataDir string) {
	t.Helper()
	settings := "data_directory = \"" + dataDir + "\"\n"
	if err := os.WriteFile(GetSettingsFilePath(), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadWithoutEnvUnchanged(t *testing.T) {
	home := setupHome(t, "")
	clearEnv(t)
	dataDir := filepath.Join(home, "profile")
	setupHomeSettings(t, dataDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != nil {
		t.Error("Env should be nil without OTUI_* variables")
	}
	if cfg.DataDir() != dataDir {
		t.Errorf("data dir = %q, want %q", cfg.DataDir(), dataDir)
	}
}

func TestAPIKeyFromEnvIsNotPersisted(t *testing.T) {
	home := setupHome(t, "")
	envData := filepath.Join(home, "envdata")
	setEnv(t, map[string]string{
		EnvDataDir: envData, EnvProvider: "openrouter", EnvModel: "x/y", EnvAPIKey: "sk-env-secret",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.CredentialStore.Set("anthropic", "sk-stored"); err != nil {
		t.Fatal(err)
	}

	if got := cfg.APIKey("openrouter"); got != "sk-env-secret" {
		t.Errorf("APIKey(openrouter) = %q, want env key", got)
	}
	if got := cfg.APIKey("anthropic"); got != "sk-stored" {
		t.Errorf("APIKey(anthropic) = %q, want stored key", got)
	}

	// Saving credentials (e.g. Provider Settings) must not write the env key
	if err := cfg.CredentialStore.Save(cfg.DataDir()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(envData, "credentials.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-env-secret") {
		t.Error("OTUI_API_KEY was written to credentials.toml")
	}
}
