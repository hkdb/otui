package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryLoadSanitizesPlugins(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "registry"), 0700)
	data := `[{"id": "x", "name": "Nice\u001b[2J plugin", "description": "desc\r\u001b]52;c;aGk=\u0007\nmore", "install_type": "npm"}]`
	if err := os.WriteFile(filepath.Join(dir, "registry", "plugin_registry.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := r.GetByID("x")
	if p == nil || strings.ContainsAny(p.Name+p.Description, "\x1b\x07\r\n") {
		t.Errorf("plugin text not cleaned: %+v", p)
	}
}
