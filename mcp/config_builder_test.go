package mcp

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"otui/storage"
)

func TestTokenizeArgs(t *testing.T) {
	got := tokenizeArgs("--api-key value::'API Key' --dir value::'Directory';;'/tmp/x y' -y")
	want := []string{"--api-key", "value::'API Key'", "--dir", "value::'Directory';;'/tmp/x y'", "-y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokenizeArgs = %q, want %q", got, want)
	}
}

func TestSubstituteArgs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		config   map[string]string
		want     []string
	}{
		{"label with spaces", "--api-key value::'API Key'", map[string]string{"ARG_0": "secret"},
			[]string{"--api-key", "secret"}},
		{"value with spaces stays one arg", "value::'Directory'", map[string]string{"ARG_0": "/home/me/My Files"},
			[]string{"/home/me/My Files"}},
		{"default used when unset", "--dir value::'Directory';;'./'", nil,
			[]string{"--dir", "./"}},
		{"value overrides default", "--dir value::'Directory';;'./'", map[string]string{"ARG_0": "/srv"},
			[]string{"--dir", "/srv"}},
		{"empty value drops its flag", "--api-key value::'API Key' --dir value::'Directory' -y",
			map[string]string{"ARG_1": "/srv"},
			[]string{"--dir", "/srv", "-y"}},
		{"empty positional dropped", "value::'A' value::'B'", map[string]string{"ARG_1": "b"},
			[]string{"b"}},
		{"tag inside a token", "--dir=value::'Directory'", map[string]string{"ARG_0": "/srv"},
			[]string{"--dir=/srv"}},
		{"fixed args unchanged", "-p 47334:47334 -p 47335:47335", nil,
			[]string{"-p", "47334:47334", "-p", "47335:47335"}},
		{"empty template", "", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubstituteArgs(tt.template, tt.config); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SubstituteArgs(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestArgsRoundTrip(t *testing.T) {
	args := []ArgPair{
		{Flag: "-y", Type: "none"},
		{Flag: "--api-key", Type: "value", Label: "API Key"},
		{Flag: "--dir", Type: "value", Label: "Directory", Default: "/tmp/x y"},
		{Flag: "--mode", Type: "fixed", Value: "fast"},
		{Type: "value", Label: "Root"},
		{Type: "fixed", Value: "serve"},
	}
	s := ArgsToString(args)
	if got := StringToArgs(s); !reflect.DeepEqual(got, args) {
		t.Fatalf("StringToArgs(%q) = %+v, want %+v", s, got, args)
	}
}

func TestStringToArgsRegistryTemplates(t *testing.T) {
	got := StringToArgs("value::'Directory'")
	want := []ArgPair{{Type: "value", Label: "Directory"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGoBinaryName(t *testing.T) {
	tests := map[string]string{
		"github.com/u/mcp-x/cmd/server@latest": "server",
		"github.com/u/mcp-x/v2@v2.1.0":         "mcp-x",
		"github.com/u/mcp-x@latest":            "mcp-x",
		"github.com/u/mcp-x":                   "mcp-x",
		"example.com/tool/v1@v1.0.0":           "v1",
		"example.com/vfox@latest":              "vfox",
	}
	for pkg, want := range tests {
		if runtime.GOOS == "windows" {
			want += ".exe"
		}
		if got := goBinaryName(pkg); got != want {
			t.Errorf("goBinaryName(%q) = %q, want %q", pkg, got, want)
		}
	}
}

func TestBuildPluginCommandGo(t *testing.T) {
	m := &MCPManager{}
	installed := &storage.InstalledPlugin{InstallPath: "/data/plugins/x"}

	cmd, args := m.buildPluginCommand(&Plugin{InstallType: "go", Package: "github.com/u/mcp-x/cmd/server@latest",
		Args: "--root value::'Root'"}, installed, map[string]string{"ARG_0": "/srv"})
	want := filepath.Join("/data/plugins/x", "bin", goBinaryName("github.com/u/mcp-x/cmd/server@latest"))
	if cmd != want || !reflect.DeepEqual(args, []string{"--root", "/srv"}) {
		t.Fatalf("go command = %q %q, want %q [--root /srv]", cmd, args, want)
	}

	cmd, _ = m.buildPluginCommand(&Plugin{InstallType: "go", Package: "github.com/u/mcp-x@latest", Command: "mcp"}, installed, nil)
	if want := filepath.Join("/data/plugins/x", "bin", "mcp"); cmd != want {
		t.Fatalf("go command override = %q, want %q", cmd, want)
	}
}

func TestBuildPluginCommandManualArgs(t *testing.T) {
	m := &MCPManager{}
	installed := &storage.InstalledPlugin{InstallPath: "/data/plugins/x"}
	cmd, args := m.buildPluginCommand(&Plugin{InstallType: "manual", Command: "python3 server.py",
		Args: "--key value::'Key'"}, installed, map[string]string{"ARG_0": "k"})
	if cmd != "python3" || !reflect.DeepEqual(args, []string{"server.py", "--key", "k"}) {
		t.Fatalf("manual command = %q %q", cmd, args)
	}
}
