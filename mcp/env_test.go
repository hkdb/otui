package mcp

import (
	"strings"
	"testing"
)

func TestFilterEnv(t *testing.T) {
	environ := []string{"PATH=/bin", "HOME=/h", "LC_ALL=C", "XDG_RUNTIME_DIR=/run/1", "HTTPS_PROXY=http://p",
		"OTUI_API_KEY=sk-1", "AWS_SECRET_ACCESS_KEY=x", "GITHUB_TOKEN=y", "RANDOM=z"}
	got := strings.Join(filterEnv(environ, false), " ")
	if got != "PATH=/bin HOME=/h LC_ALL=C XDG_RUNTIME_DIR=/run/1 HTTPS_PROXY=http://p" {
		t.Errorf("unix env = %q", got)
	}

	windows := strings.Join(filterEnv([]string{"Path=C:\\\\bin", "SystemRoot=C:\\\\Windows", "OTUI_API_KEY=sk"}, true), " ")
	if windows != "Path=C:\\\\bin SystemRoot=C:\\\\Windows" {
		t.Errorf("windows env = %q", windows)
	}
}

func TestConfigToEnvKeepsSecretsOut(t *testing.T) {
	t.Setenv("OTUI_API_KEY", "sk-otui")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws")
	env := strings.Join(configToEnv(map[string]string{"TEST_MODE": "1"},
		map[string]string{"API_KEY": "plugin-key", "ARG_0": "/srv", "__command__": "x", "server_url": "http://x"}), "\n")

	for _, leaked := range []string{"sk-otui", "AWS_SECRET", "ARG_0", "__command__", "server_url"} {
		if strings.Contains(env, leaked) {
			t.Errorf("plugin env contains %s", leaked)
		}
	}
	for _, want := range []string{"PATH=", "API_KEY=plugin-key", "TEST_MODE=1"} {
		if !strings.Contains(env, want) {
			t.Errorf("plugin env missing %s", want)
		}
	}
}

func TestRemotePreviewWarnsAboutHTTP(t *testing.T) {
	plugin := &Plugin{InstallType: "remote", ServerURL: "http://mcp.example.com/sse"}
	if _, _, w := CommandPreview(plugin, map[string]string{"Authorization": "Bearer x"}, ""); len(w) != 1 {
		t.Errorf("no warning for credentials over http: %v", w)
	}
	if _, _, w := CommandPreview(plugin, nil, ""); len(w) != 0 {
		t.Errorf("warning without credentials: %v", w)
	}
	local := &Plugin{InstallType: "remote", ServerURL: "http://localhost:8080/sse"}
	if _, _, w := CommandPreview(local, map[string]string{"Authorization": "x"}, ""); len(w) != 0 {
		t.Errorf("warning for localhost: %v", w)
	}
}
