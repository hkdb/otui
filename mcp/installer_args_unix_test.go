//go:build !windows

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"otui/storage"
)

// fakeTools puts logging npm/pip/go/python3 scripts on PATH. pip and venv
// are emulated: python3 -m venv creates venv/bin/pip as the logging script.
func fakeTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	logger := "#!/bin/sh\necho \"$(basename \"$0\") $*\" >> " + log + "\n"
	for _, name := range []string{"npm", "go"} {
		os.WriteFile(filepath.Join(dir, name), []byte(logger), 0o755)
	}
	python := "#!/bin/sh\n" + `case "$*" in
  --version) echo "Python 3.12.0"; exit 0 ;;
  "-m venv --help") exit 0 ;;
esac
` + strings.TrimPrefix(logger, "#!/bin/sh\n") + `if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then mkdir -p "$3/bin"; cp "$0" "$3/bin/pip"; fi` + "\n"
	os.WriteFile(filepath.Join(dir, "python3"), []byte(python), 0o755)
	os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\necho v22.0.0\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "npx"), []byte("#!/bin/sh\necho 10.0.0\n"), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestInstallersPassPackageAfterDashDash(t *testing.T) {
	log := fakeTools(t)
	for _, plugin := range []*Plugin{
		{ID: "n", Name: "n", InstallType: "npm", Package: "@scope/pkg"},
		{ID: "p", Name: "p", InstallType: "pip", Package: "mcp-server-fetch"},
		{ID: "g", Name: "g", InstallType: "go", Package: "example.com/x/cmd/y@latest"},
	} {
		inst, _ := newTestInstaller(t)
		inst.runtimeChecker = NewRuntimeChecker() // Picks up the fake tools
		if err := inst.InstallWithContext(context.Background(), plugin, nil); err != nil {
			t.Fatalf("%s install: %v", plugin.InstallType, err)
		}
	}

	argv, _ := os.ReadFile(log)
	for _, want := range []string{"npm install -- @scope/pkg", "pip install -- mcp-server-fetch", "go install -- example.com/x/cmd/y@latest"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("missing %q in:\n%s", want, argv)
		}
	}
	if n := strings.Count(string(argv), "npm install"); n != 1 {
		t.Errorf("npm install ran %d times, want 1", n)
	}
}

func TestInstallRejectsOptionPackage(t *testing.T) {
	log := fakeTools(t)
	inst, _ := newTestInstaller(t)
	err := inst.InstallWithContext(context.Background(), &Plugin{ID: "x", InstallType: "npm", Package: "--prefix=/"}, nil)
	if err == nil {
		t.Fatal("package starting with - accepted")
	}
	if argv, _ := os.ReadFile(log); len(argv) != 0 {
		t.Errorf("tools ran for an invalid package: %s", argv)
	}
}

func TestUninstallInvalidIDKeepsFiles(t *testing.T) {
	inst, ps := newTestInstaller(t)

	// A directory next to the plugins directory that ".." would point to
	sentinel := filepath.Join(filepath.Dir(inst.pluginsDir), "keep-me")
	os.MkdirAll(inst.pluginsDir, 0700)
	if err := os.WriteFile(sentinel, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ps.Save(storageInstalled("..")); err != nil {
		t.Fatal(err)
	}

	_ = inst.UninstallWithContext(context.Background(), "..", nil)
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("uninstalling plugin %q deleted files outside the plugins directory", "..")
	}
}

func storageInstalled(id string) storage.InstalledPlugin {
	return storage.InstalledPlugin{ID: id, Name: id, InstallPath: "", InstallMethod: "manual"}
}
