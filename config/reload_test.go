package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// passphraseProfile creates a HOME with an ssh_key profile whose key needs a
// passphrase, and returns the data dir
func passphraseProfile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t)

	dataDir := filepath.Join(home, "data")
	os.MkdirAll(filepath.Join(home, ".config", "otui"), 0700)
	os.MkdirAll(dataDir, 0700)
	keyPath := writePassphraseKey(t, home, testPassphrase)
	writeEncryptedCredentials(t, dataDir, keyPath, testPassphrase)

	setupHomeSettings(t, dataDir)
	userCfg := "[security]\ncredential_storage = \"ssh_key\"\nssh_key_path = \"" + keyPath + "\"\n"
	os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(userCfg), 0600)
	return dataDir
}

// unlockedConfig loads a passphrase profile the way main.go does
func unlockedConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err == nil || !strings.Contains(err.Error(), "passphrase required") {
		t.Fatalf("Load err = %v, want passphrase required", err)
	}
	cfg.CredentialStore.SetPassphrase(testPassphrase)
	if err := cfg.CredentialStore.Load(cfg.DataDir()); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestReloadKeepsUnlockedCredentialStore(t *testing.T) {
	dataDir := passphraseProfile(t)
	cfg := unlockedConfig(t)

	// A settings change on disk (e.g. Provider Settings enabling OpenRouter)
	if err := UpdateProviderField(dataDir, "openrouter", "enabled", "true"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Reload(cfg.CredentialStore)
	if err != nil {
		t.Fatalf("Reload: %v", err) // Load() would fail: passphrase required
	}
	if reloaded.CredentialStore != cfg.CredentialStore {
		t.Error("Reload replaced the credential store")
	}
	if reloaded.Keybindings == nil {
		t.Error("Reload has no keybindings")
	}
	enabled := false
	for _, p := range reloaded.Providers {
		if p.ID == "openrouter" && p.Enabled {
			enabled = true
		}
	}
	if !enabled {
		t.Error("Reload didn't pick up the config.toml change")
	}
	if reloaded.APIKey("openrouter") != "sk-test" {
		t.Errorf("APIKey = %q, want sk-test", reloaded.APIKey("openrouter"))
	}
}

func TestSaveProviderAPIKeysWithPassphrase(t *testing.T) {
	dataDir := passphraseProfile(t)
	cfg := unlockedConfig(t)
	encPath := filepath.Join(dataDir, "credentials.enc")

	// Unchanged keys don't rewrite the file
	before, _ := os.Stat(encPath)
	time.Sleep(10 * time.Millisecond)
	if err := SaveProviderAPIKeys(cfg.CredentialStore, dataDir, map[string]string{"openrouter": "sk-test"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(encPath)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("credentials.enc rewritten although nothing changed")
	}

	// A changed key is saved encrypted and readable after a restart
	if err := SaveProviderAPIKeys(cfg.CredentialStore, dataDir, map[string]string{"openrouter": "sk-new"}); err != nil {
		t.Fatalf("SaveProviderAPIKeys: %v", err)
	}
	restarted := unlockedConfig(t)
	if got := restarted.CredentialStore.Get("openrouter"); got != "sk-new" {
		t.Errorf("after restart key = %q, want sk-new", got)
	}
}

func TestSaveProviderAPIKeysOnlyWritesGivenDir(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	store := NewCredentialStore(SecurityPlainText, "")
	if err := SaveProviderAPIKeys(store, dirA, map[string]string{"openai": "sk-a"}); err != nil {
		t.Fatal(err)
	}
	if !FileExists(filepath.Join(dirA, "credentials.toml")) {
		t.Error("credentials not written to the given dir")
	}
	if FileExists(filepath.Join(dirB, "credentials.toml")) {
		t.Error("credentials written to another dir")
	}
}

func TestUpdateProviderFieldRejectsAPIKey(t *testing.T) {
	if err := UpdateProviderField(t.TempDir(), "openrouter", "apikey", "sk"); err == nil {
		t.Error("UpdateProviderField should not handle API keys")
	}
}
