package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"otui/storage"
)

func TestValidatePluginID(t *testing.T) {
	for _, id := range []string{"modelcontextprotocol-server-filesystem", "upstash-context7", "a.b_c-1", "X"} {
		if err := ValidatePluginID(id); err != nil {
			t.Errorf("%q rejected: %v", id, err)
		}
	}
	for _, id := range []string{"", ".", "..", "../x", "a/b", `a\\b`, "-x", ".hidden", "a b", strings.Repeat("a", 101)} {
		if ValidatePluginID(id) == nil {
			t.Errorf("%q accepted", id)
		}
	}
}

func TestValidatePackage(t *testing.T) {
	ok := []struct{ typ, pkg string }{
		{"npm", "@upstash/context7-mcp"}, {"npm", "bitcoin-mcp@latest"}, {"npm", "git+https://github.com/u/r.git"},
		{"pip", "mcp-server-fetch"}, {"pip", "markitdown==0.1.0"},
		{"go", "github.com/u/mcp-x/cmd/server@latest"}, {"go", "example.com/b6echo/cmd/b6echo@v1.2.3"},
		{"docker", "ghcr.io/github/github-mcp-server"}, {"docker", "mindsdb/mindsdb:latest"}, {"docker", "localhost:5000/img:1"},
		{"docker", ""}, {"manual", "--anything"},
	}
	for _, tt := range ok {
		if err := ValidatePackage(tt.typ, tt.pkg); err != nil {
			t.Errorf("%s %q rejected: %v", tt.typ, tt.pkg, err)
		}
	}
	bad := []struct{ typ, pkg string }{
		{"npm", "--prefix=/"}, {"npm", "-g"}, {"npm", "pkg --global"}, {"npm", ""},
		{"pip", "--target=/tmp"}, {"pip", "-e."},
		{"go", "-toolexec=sh"}, {"go", "github.com/x y"}, {"go", "example.com/x;rm"},
		{"docker", "--privileged"}, {"docker", "-v"}, {"docker", "img --privileged"},
	}
	for _, tt := range bad {
		if ValidatePackage(tt.typ, tt.pkg) == nil {
			t.Errorf("%s %q accepted", tt.typ, tt.pkg)
		}
	}
}

func TestPluginDirStaysInside(t *testing.T) {
	inst := &Installer{pluginsDir: "/data/plugins"}
	if dir, err := inst.pluginDir("good-id"); err != nil || dir != filepath.Join("/data/plugins", "good-id") {
		t.Errorf("pluginDir(good-id) = %q, %v", dir, err)
	}
	for _, id := range []string{"..", "../x", "a/../../b", ""} {
		if dir, err := inst.pluginDir(id); err == nil {
			t.Errorf("pluginDir(%q) = %q, want error", id, dir)
		}
	}
}

func TestRegistryPluginPathsStayInside(t *testing.T) {
	m := &MCPManager{}
	installed := &storage.InstalledPlugin{InstallPath: "/data/plugins/x"}

	for _, plugin := range []*Plugin{
		{InstallType: "npm", Package: "pkg", Command: "../../../../bin/sh"},
		{InstallType: "npm", Package: "pkg", Command: "/bin/sh"},
		{InstallType: "go", Package: "example.com/x@latest", Command: "/usr/bin/env"},
		{InstallType: "pip", Package: "../../../../bin/sh"},
		{InstallType: "binary", Package: "../../outside"},
		{InstallType: "manual", Package: "../../outside"},
	} {
		if cmd, _ := m.buildPluginCommand(plugin, installed, nil); cmd != "" {
			t.Errorf("registry %s plugin %+v runs %q outside its folder", plugin.InstallType, plugin, cmd)
		}
	}

	// Custom plugins you defined may point anywhere
	custom := &Plugin{InstallType: "npm", Package: "pkg", Command: "/usr/local/bin/my-server", Custom: true}
	if cmd, _ := m.buildPluginCommand(custom, installed, nil); cmd != "/usr/local/bin/my-server" {
		t.Errorf("custom plugin command = %q", cmd)
	}
	// A registry manual plugin's command is shown in the preview, not refused
	manual := &Plugin{InstallType: "manual", Command: "uvx mcp-server-time"}
	if cmd, _ := m.buildPluginCommand(manual, installed, nil); cmd != "uvx" {
		t.Errorf("manual command = %q", cmd)
	}
}

func TestCommandPreview(t *testing.T) {
	context7 := &Plugin{InstallType: "npm", Package: "@upstash/context7-mcp", Args: "--api-key value::'API Key'"}
	install, run, warnings := CommandPreview(context7, map[string]string{"ARG_0": "sk-secret"}, "")
	if install != "npm install -- @upstash/context7-mcp" {
		t.Errorf("install = %q", install)
	}
	if !strings.Contains(run, "--api-key ****") || strings.Contains(run, "sk-secret") || len(warnings) != 0 {
		t.Errorf("run = %q (warnings %v), want masked key", run, warnings)
	}

	risky := &Plugin{InstallType: "docker", Package: "example/mcp:latest", Environment: "API_TOKEN",
		Args: "-v /:/host --privileged --network=host"}
	install, run, warnings = CommandPreview(risky, map[string]string{"API_TOKEN": "t"}, "")
	if install != "docker pull -- example/mcp:latest" || run != "docker run -i --rm -e API_TOKEN -v /:/host --privileged --network=host example/mcp:latest" {
		t.Errorf("install %q run %q", install, run)
	}
	joined := strings.Join(warnings, "; ")
	for _, want := range []string{"-v /:/host", "--privileged", "--network=host"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q missing %s", joined, want)
		}
	}

	remote := &Plugin{InstallType: "remote", ServerURL: "https://mcp.example.com/sse"}
	if _, run, _ := CommandPreview(remote, nil, ""); run != "Connects to: https://mcp.example.com/sse" {
		t.Errorf("remote run = %q", run)
	}
}
