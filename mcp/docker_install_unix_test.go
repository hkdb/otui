//go:build !windows

package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"otui/config"
	"otui/storage"
)

// fakeDocker puts a "docker" script on PATH: pull always fails, and
// "image inspect" succeeds only for localImage
func fakeDocker(t *testing.T, localImage string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" >> "` + filepath.Join(dir, "argv.log") + `"
for last; do :; done
case "$1" in
  --version) echo "Docker version 29.0.0, build test" ;;
  pull) echo "pull access denied for $last" >&2; exit 1 ;;
  image) [ "$last" = "` + localImage + `" ] || exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newTestInstaller(t *testing.T) (*Installer, *storage.PluginStorage) {
	t.Helper()
	dataDir := t.TempDir()
	ps, err := storage.NewPluginStorage(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	pc, _ := config.LoadPluginsConfig(dataDir)
	return NewInstaller(ps, pc, dataDir), ps
}

func TestInstallDockerLocalImage(t *testing.T) {
	fakeDocker(t, "my/local-image")
	inst, ps := newTestInstaller(t)

	plugin := &Plugin{ID: "local", Name: "local", InstallType: "docker", Package: "my/local-image"}
	if err := inst.InstallWithContext(context.Background(), plugin, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	installed, err := ps.Load("local")
	if err != nil || installed == nil || installed.InstallMethod != "docker" {
		t.Fatalf("installed = %+v, err %v", installed, err)
	}

	// The image is passed after "--", so it can't be read as an option
	argv, _ := os.ReadFile(filepath.Join(filepath.Dir(dockerPath(t)), "argv.log"))
	for _, want := range []string{"pull -- my/local-image", "image inspect -- my/local-image"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("docker not called with %q:\n%s", want, argv)
		}
	}
}

func TestInstallDockerMissingImage(t *testing.T) {
	fakeDocker(t, "my/local-image")
	inst, ps := newTestInstaller(t)

	plugin := &Plugin{ID: "missing", Name: "missing", InstallType: "docker", Package: "nobody/missing"}
	err := inst.InstallWithContext(context.Background(), plugin, nil)
	if err == nil || !strings.Contains(err.Error(), "build it first") {
		t.Fatalf("err = %v, want pull failure with build hint", err)
	}
	if ps.IsInstalled("missing") {
		t.Fatal("plugin recorded as installed after failed pull")
	}
}

func dockerPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
