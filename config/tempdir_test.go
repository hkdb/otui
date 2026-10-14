package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// resetTempDir isolates the per-process temp dir state for a test
func resetTempDir(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", "")
	tempDir = ""
	t.Cleanup(func() { tempDir = "" })
}

func TestInitTempDirPerInstance(t *testing.T) {
	resetTempDir(t)

	if err := InitTempDir(); err != nil {
		t.Fatal(err)
	}
	first, firstEditor := GetTempDir(), GetEditorTempFile()

	tempDir = "" // simulate a second instance
	if err := InitTempDir(); err != nil {
		t.Fatal(err)
	}
	second, secondEditor := GetTempDir(), GetEditorTempFile()

	if first == second || firstEditor == secondEditor {
		t.Errorf("instances share a temp dir: %s / %s", first, second)
	}
	if filepath.Dir(first) != tempBaseDir() {
		t.Errorf("temp dir %s not under %s", first, tempBaseDir())
	}
	if info, err := os.Stat(second); err != nil || info.Mode().Perm() != 0700 {
		t.Errorf("temp dir mode = %v, err = %v", info.Mode().Perm(), err)
	}
}

func TestCleanupStaleTempDirs(t *testing.T) {
	resetTempDir(t)
	base := tempBaseDir()

	dead := filepath.Join(base, "999999999-x")                     // no such process
	live := filepath.Join(base, fmt.Sprintf("%d-x", os.Getppid())) // running
	ownOld := filepath.Join(base, fmt.Sprintf("%d-old", os.Getpid()))
	unrelated := filepath.Join(base, "notes")
	for _, dir := range []string{dead, live, ownOld, unrelated} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	if err := InitTempDir(); err != nil {
		t.Fatal(err)
	}

	for dir, wantExists := range map[string]bool{dead: false, ownOld: false, live: true, unrelated: true, GetTempDir(): true} {
		_, err := os.Stat(dir)
		if exists := err == nil; exists != wantExists {
			t.Errorf("%s exists = %v, want %v", filepath.Base(dir), exists, wantExists)
		}
	}
}

func TestCleanupTempDirOnlyRemovesOwn(t *testing.T) {
	resetTempDir(t)
	live := filepath.Join(tempBaseDir(), fmt.Sprintf("%d-other", os.Getppid()))
	if err := os.MkdirAll(live, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(live, "editor.txt"), []byte("other instance's draft"), 0600)

	if err := InitTempDir(); err != nil {
		t.Fatal(err)
	}
	own := GetTempDir()
	if err := CleanupTempDir(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Error("own temp dir not removed")
	}
	if data, err := os.ReadFile(filepath.Join(live, "editor.txt")); err != nil || string(data) != "other instance's draft" {
		t.Error("other instance's draft was touched")
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("own process reported dead")
	}
	if processAlive(999999999) || processAlive(0) || processAlive(-1) {
		t.Error("nonexistent process reported alive")
	}
}
