package ui

import (
	"strings"
	"testing"

	"otui/mcp"
)

func securityWarningFor(plugin *mcp.Plugin) string {
	a := typingAppView()
	a.width, a.height = 120, 60
	a.pluginManagerState.warnings.securityPlugin = plugin
	return a.renderSecurityWarning()
}

func TestSecurityWarningShowsCommands(t *testing.T) {
	out := securityWarningFor(&mcp.Plugin{ID: "upstash-context7", Name: "upstash/context7", InstallType: "npm",
		Package: "@upstash/context7-mcp", Args: "--api-key value::'API Key'"})
	for _, want := range []string{"npm install -- @upstash/context7-mcp", "<plugin-folder>/node_modules/.bin/", "--api-key <API Key>"} {
		if !strings.Contains(out, want) {
			t.Errorf("security warning missing %q:\n%s", want, out)
		}
	}

	out = securityWarningFor(&mcp.Plugin{ID: "risky", Name: "Risky", InstallType: "docker",
		Package: "example/mcp:latest", Args: "-v /:/host"})
	if !strings.Contains(out, "docker pull -- example/mcp:latest") || !strings.Contains(out, "⚠ This plugin") {
		t.Errorf("docker security warning missing preview or warning:\n%s", out)
	}
}

func TestPreviewConfig(t *testing.T) {
	cfg := previewConfig(&mcp.Plugin{InstallType: "npm", Environment: "API_KEY REGION;;'us'",
		Args: "--dir value::'Directory';;'./' --key value::'Key'"})
	if cfg["ARG_0"] != "./" || cfg["ARG_1"] != "<Key>" || cfg["REGION"] != "us" {
		t.Errorf("previewConfig = %v", cfg)
	}
	if _, ok := cfg["API_KEY"]; ok {
		t.Error("unset env var included")
	}
}
