package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var tmp []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			tmp = append(tmp, e.Name())
		}
	}
	return tmp
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	t.Run("creates new file", func(t *testing.T) {
		if err := WriteFileAtomic(path, []byte("a = 1\n"), 0600); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "a = 1\n" {
			t.Errorf("content = %q", data)
		}
	})

	t.Run("replaces existing file and tightens permissions", func(t *testing.T) {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if err := WriteFileAtomic(path, []byte("a = 2\n"), 0600); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "a = 2\n" {
			t.Errorf("content = %q", data)
		}
		if runtime.GOOS != "windows" {
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0600 {
				t.Errorf("mode = %v, want 0600", info.Mode().Perm())
			}
		}
	})

	t.Run("no temporary files left", func(t *testing.T) {
		if tmp := tempFiles(t, dir); len(tmp) > 0 {
			t.Errorf("leftover temp files: %v", tmp)
		}
	})

	t.Run("writes through symlinks", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges on Windows")
		}
		targetDir := t.TempDir()
		target := filepath.Join(targetDir, "real.toml")
		if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "linked.toml")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}

		if err := WriteFileAtomic(link, []byte("new"), 0600); err != nil {
			t.Fatal(err)
		}
		if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
			t.Error("symlink was replaced by a regular file")
		}
		if data, _ := os.ReadFile(target); string(data) != "new" {
			t.Errorf("target content = %q, want new", data)
		}
	})

	t.Run("error leaves nothing behind", func(t *testing.T) {
		missing := filepath.Join(dir, "missing-dir", "file")
		if err := WriteFileAtomic(missing, []byte("x"), 0600); err == nil {
			t.Error("expected error for missing directory")
		}
		if tmp := tempFiles(t, dir); len(tmp) > 0 {
			t.Errorf("leftover temp files: %v", tmp)
		}
	})
}

func TestEmptyCredentialsFileDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "credentials.toml"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	store := NewCredentialStore(SecurityPlainText, "")
	if err := store.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := store.Set("openrouter", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPlugin("fs", "token", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(dir); err != nil {
		t.Fatal(err)
	}

	reloaded := NewCredentialStore(SecurityPlainText, "")
	if err := reloaded.Load(dir); err != nil {
		t.Fatal(err)
	}
	if reloaded.Get("openrouter") != "sk-test" || reloaded.GetPlugin("fs", "token") != "abc" {
		t.Error("credentials not saved after loading an empty file")
	}
}

func TestEncryptedNullCredentialsDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	keyPath := writePassphraseKey(t, dir, testPassphrase)

	// Encrypt the JSON "null" as the credentials payload
	em := NewEncryptionManager(EncryptionSSHKey, keyPath)
	em.SetPassphrase(testPassphrase)
	if err := em.Initialize(); err != nil {
		t.Fatal(err)
	}
	encrypted, err := em.Encrypt([]byte("null"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(encryptedCredentialsPath(dir), encrypted, 0600); err != nil {
		t.Fatal(err)
	}

	store := NewCredentialStore(SecuritySSHKey, keyPath)
	store.SetPassphrase(testPassphrase)
	if err := store.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := store.Set("openai", "sk-test"); err != nil {
		t.Fatal(err)
	}
}
