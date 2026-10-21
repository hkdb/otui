package mcp

import (
	"reflect"
	"testing"
)

func TestBuildDockerCommand(t *testing.T) {
	github := &Plugin{InstallType: "docker", Package: "ghcr.io/github/github-mcp-server",
		Environment: "GITHUB_PERSONAL_ACCESS_TOKEN GITHUB_HOST"}

	tests := []struct {
		name     string
		plugin   *Plugin
		config   map[string]string
		wantCmd  string
		wantArgs []string
	}{
		{"env by name, unset keys skipped", github, map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "secret"},
			"docker", []string{"run", "-i", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "ghcr.io/github/github-mcp-server"}},
		{"options before image, container args after --",
			&Plugin{InstallType: "docker", Package: "img:1", Args: "--network=host -v value::'Dir':/data -- stdio --read-only"},
			map[string]string{"ARG_0": "/srv/my files"},
			"docker", []string{"run", "-i", "--rm", "--network=host", "-v", "/srv/my files:/data", "img:1", "stdio", "--read-only"}},
		{"configured command used as is", github, map[string]string{"__command__": "podman run -i --rm my/image"},
			"podman", []string{"run", "-i", "--rm", "my/image"}},
		{"plugin command used as is", &Plugin{InstallType: "docker", Command: "docker run -i --rm x"}, nil,
			"docker", []string{"run", "-i", "--rm", "x"}},
		{"no image", &Plugin{InstallType: "docker"}, nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := (&MCPManager{}).buildPluginCommand(tt.plugin, nil, tt.config)
			if cmd != tt.wantCmd || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("got %q %q, want %q %q", cmd, args, tt.wantCmd, tt.wantArgs)
			}
		})
	}
}
