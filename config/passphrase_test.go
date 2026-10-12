package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

const testPassphrase = "correct horse"

// writePassphraseKey writes a passphrase protected ed25519 key and returns its path
func writePassphraseKey(t *testing.T, dir, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "otui_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return keyPath
}

// writeEncryptedCredentials saves credentials.enc encrypted with keyPath
func writeEncryptedCredentials(t *testing.T, dataDir, keyPath, passphrase string) {
	t.Helper()
	store := NewCredentialStore(SecuritySSHKey, keyPath)
	store.SetPassphrase(passphrase)
	if err := store.Set("openrouter", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(dataDir); err != nil {
		t.Fatalf("save encrypted credentials: %v", err)
	}
}

func TestCredentialStorePassphraseErrors(t *testing.T) {
	dir := t.TempDir()
	keyPath := writePassphraseKey(t, dir, testPassphrase)
	writeEncryptedCredentials(t, dir, keyPath, testPassphrase)

	t.Run("missing passphrase", func(t *testing.T) {
		err := NewCredentialStore(SecuritySSHKey, keyPath).Load(dir)
		if err == nil || !strings.Contains(err.Error(), "passphrase required") {
			t.Fatalf("err = %v, want passphrase required", err)
		}
		if IsIncorrectPassphrase(err) {
			t.Error("missing passphrase reported as incorrect passphrase")
		}
	})

	t.Run("wrong passphrase", func(t *testing.T) {
		store := NewCredentialStore(SecuritySSHKey, keyPath)
		store.SetPassphrase("wrong")
		err := store.Load(dir)
		if !IsIncorrectPassphrase(err) {
			t.Fatalf("err = %v, want incorrect passphrase", err)
		}
	})

	t.Run("correct passphrase", func(t *testing.T) {
		store := NewCredentialStore(SecuritySSHKey, keyPath)
		store.SetPassphrase(testPassphrase)
		if err := store.Load(dir); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := store.Get("openrouter"); got != "sk-test" {
			t.Errorf("Get = %q, want sk-test", got)
		}
	})

	t.Run("credentials from a different key", func(t *testing.T) {
		otherDir := t.TempDir()
		otherKey := writePassphraseKey(t, otherDir, testPassphrase)

		store := NewCredentialStore(SecuritySSHKey, otherKey)
		store.SetPassphrase(testPassphrase)
		err := store.Load(dir) // credentials.enc was encrypted with keyPath
		if err == nil {
			t.Fatal("expected decrypt error")
		}
		if IsIncorrectPassphrase(err) {
			t.Errorf("decrypt failure reported as incorrect passphrase: %v", err)
		}
		if !strings.Contains(err.Error(), "failed to decrypt") {
			t.Errorf("err = %v, want failed to decrypt", err)
		}
	})
}

// TestLoadPartialConfigHasKeybindings covers the startup crash: the config
// returned with "passphrase required" must still have keybindings
func TestLoadPartialConfigHasKeybindings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OTUI_OLLAMA_HOST", "")
	t.Setenv("OTUI_OLLAMA_MODEL", "")
	t.Setenv("OTUI_DATA_DIR", "")

	dataDir := filepath.Join(home, "data")
	if err := os.MkdirAll(filepath.Join(home, ".config", "otui"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	keyPath := writePassphraseKey(t, home, testPassphrase)
	writeEncryptedCredentials(t, dataDir, keyPath, testPassphrase)

	settings := "data_directory = \"" + dataDir + "\"\n"
	if err := os.WriteFile(GetSettingsFilePath(), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	userCfg := "[security]\ncredential_storage = \"ssh_key\"\nssh_key_path = \"" + keyPath + "\"\n"
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(userCfg), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err == nil || !strings.Contains(err.Error(), "passphrase required") {
		t.Fatalf("Load err = %v, want passphrase required", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config with passphrase required")
	}
	if cfg.Keybindings == nil {
		t.Fatal("partial config has nil Keybindings (UI would panic)")
	}
	if cfg.CredentialStore == nil {
		t.Fatal("partial config has nil CredentialStore")
	}

	// Retrying with the passphrase completes the config
	cfg.CredentialStore.SetPassphrase(testPassphrase)
	if err := cfg.CredentialStore.Load(cfg.DataDir()); err != nil {
		t.Fatalf("retry with passphrase: %v", err)
	}
}
