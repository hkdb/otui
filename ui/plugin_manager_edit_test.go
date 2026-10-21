package ui

import (
	"reflect"
	"testing"

	"otui/mcp"
)

func TestCustomPluginEnvVarsOnlyEnvironmentKeys(t *testing.T) {
	plugin := &mcp.Plugin{Environment: "API_KEY REGION;;'us'"}
	cfg := map[string]string{"API_KEY": "k", "ARG_0": "/srv", "server_url": "http://x"}
	got := customPluginEnvVars(plugin, cfg)
	want := []EnvVarPair{{Key: "API_KEY", Value: "k"}, {Key: "REGION", Value: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("customPluginEnvVars = %+v, want %+v", got, want)
	}
}

func TestMergeCustomPluginConfigKeepsOtherSettings(t *testing.T) {
	existing := map[string]string{"API_KEY": "old", "OLD_VAR": "x", "ARG_0": "/srv", "server_url": "http://x"}
	got := mergeCustomPluginConfig(existing, "API_KEY OLD_VAR", []EnvVarPair{{Key: "API_KEY", Value: "new"}})
	want := map[string]string{"API_KEY": "new", "ARG_0": "/srv", "server_url": "http://x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeCustomPluginConfig = %v, want %v", got, want)
	}
}
