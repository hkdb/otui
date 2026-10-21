package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// TestUserConfigTemplateDecodesCleanly guards against settings in the
// template ending up under a [table] where TOML silently ignores them
func TestUserConfigTemplateDecodesCleanly(t *testing.T) {
	cfg := &UserConfig{}
	md, err := toml.Decode(GenerateUserConfigTemplate(), cfg)
	if err != nil {
		t.Fatalf("template doesn't parse: %v", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		t.Errorf("template keys ignored by UserConfig: %v", undecoded)
	}

	defaults := DefaultUserConfig()
	if cfg.RequireApproval != defaults.RequireApproval ||
		cfg.EnableMultiStep != defaults.EnableMultiStep ||
		cfg.MaxIterations != defaults.MaxIterations ||
		cfg.PluginsEnabled != defaults.PluginsEnabled ||
		cfg.NotifyOnComplete != defaults.NotifyOnComplete ||
		cfg.FirstTokenTimeout != defaults.FirstTokenTimeout ||
		cfg.StreamIdleTimeout != defaults.StreamIdleTimeout {
		t.Errorf("template values differ from defaults: %+v", cfg)
	}
	for _, key := range []string{"require_approval", "enable_multi_step", "max_iterations", "plugins_enabled", "notify_on_complete", "default_system_prompt", "first_token_timeout", "stream_idle_timeout"} {
		if !md.IsDefined(key) {
			t.Errorf("template doesn't define top-level %q", key)
		}
	}
}

// oldTemplate is the layout written by earlier versions: top-level settings
// after the [ollama] header
const oldTemplate = `# OTUI User Configuration
default_provider = "ollama"
default_model = "llama3.1:latest"
last_used_provider = "ollama"

[ollama]
# Ollama server URL
host = "http://localhost:11434"

# Default system prompt for new sessions (optional)
default_system_prompt = "be brief"

plugins_enabled = false

# Tool Permissions
require_approval = false

allowed_tools = ["fs.read"]

enable_multi_step = true  # Allow LLM to execute multiple steps in sequence
max_iterations = 20       # Maximum steps per user message

notify_on_complete = true

[security]
credential_storage = "plaintext"
`

func TestLoadUserConfigHonorsMisplacedOllamaKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(oldTemplate), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadUserConfig(dir)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.RequireApproval || cfg.MaxIterations != 20 || cfg.DefaultSystemPrompt != "be brief" ||
		!cfg.NotifyOnComplete || len(cfg.AllowedTools) != 1 || cfg.AllowedTools[0] != "fs.read" {
		t.Errorf("misplaced settings not honored: %+v", cfg)
	}
	if cfg.Ollama.Host != "http://localhost:11434" {
		t.Errorf("Ollama.Host = %q", cfg.Ollama.Host)
	}

	// The file (and its comments) is left alone
	data, _ := os.ReadFile(path)
	if string(data) != oldTemplate {
		t.Error("config.toml was rewritten")
	}
}

func TestTopLevelKeyWinsOverMisplacedKey(t *testing.T) {
	dir := t.TempDir()
	content := "require_approval = true\nmax_iterations = 5\nenable_multi_step = true\n\n[ollama]\nhost = \"h\"\nrequire_approval = false\nmax_iterations = 20\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadUserConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RequireApproval || cfg.MaxIterations != 5 {
		t.Errorf("top-level keys should win: require_approval=%v max_iterations=%d", cfg.RequireApproval, cfg.MaxIterations)
	}
}

func TestMultiStepMigrationInsertsTopLevelKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "default_model = \"m\"\n\n[ollama]\nhost = \"h\"\n\n[security]\ncredential_storage = \"plaintext\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadUserConfig(dir); err != nil {
		t.Fatal(err)
	}

	// Re-read the migrated file: the keys must be top-level, not in [security]
	migrated := &UserConfig{}
	md, err := toml.DecodeFile(path, migrated)
	if err != nil {
		t.Fatalf("migrated file doesn't parse: %v", err)
	}
	if !md.IsDefined("enable_multi_step") || !md.IsDefined("max_iterations") {
		data, _ := os.ReadFile(path)
		t.Fatalf("migration keys not top-level:\n%s", data)
	}
	if !migrated.EnableMultiStep || migrated.MaxIterations != 10 {
		t.Errorf("migrated values = %v/%d", migrated.EnableMultiStep, migrated.MaxIterations)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		t.Errorf("keys ignored after migration: %v", undecoded)
	}
}

func TestInsertBeforeFirstTable(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a = 1\n[t]\nb = 2\n", "a = 1\nX\n[t]\nb = 2\n"},
		{"  [t]\n", "X\n  [t]\n"},
		{"a = 1", "a = 1\n\nX\n"},
	}
	for _, tt := range tests {
		if got := insertBeforeFirstTable(tt.in, "X\n"); got != tt.want {
			t.Errorf("insertBeforeFirstTable(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if !strings.HasPrefix(insertBeforeFirstTable("[[providers]]\n", "X\n"), "X\n[[providers]]") {
		t.Error("array of tables header not detected")
	}
}

func TestResponseTimeoutDefaults(t *testing.T) {
	c := &Config{}
	c.applyUserConfig(&UserConfig{}) // Older config.toml without the keys
	if c.FirstTokenTimeout() != DefaultFirstTokenTimeout*time.Second || c.StreamIdleTimeout() != DefaultStreamIdleTimeout*time.Second {
		t.Errorf("defaults = %v / %v", c.FirstTokenTimeout(), c.StreamIdleTimeout())
	}
	c.applyUserConfig(&UserConfig{FirstTokenTimeout: 30, StreamIdleTimeout: 20})
	if c.FirstTokenTimeout() != 30*time.Second || c.StreamIdleTimeout() != 20*time.Second {
		t.Errorf("configured = %v / %v", c.FirstTokenTimeout(), c.StreamIdleTimeout())
	}
}
