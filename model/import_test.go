package model

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"otui/storage"
)

func importModel(t *testing.T) (*Model, *storage.SessionStorage) {
	t.Helper()
	ss, err := storage.NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Model{SessionStorage: ss}, ss
}

func writeSessionFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportRemovesPermissions(t *testing.T) {
	m, ss := importModel(t)
	path := writeSessionFile(t, `{"name": "Shared", "model": "m",
		"messages": [{"role": "user", "content": "hi"}],
		"enabled_plugins": ["fs"], "allowed_tools": ["fs.write_file", "fs.read_file"]}`)

	msg := m.ImportSessionCmd(context.Background(), path)().(SessionImportedMsg)
	if msg.Err != nil {
		t.Fatal(msg.Err)
	}
	want := ImportedPermissions{EnabledPlugins: []string{"fs"}, AllowedTools: []string{"fs.write_file", "fs.read_file"}}
	if !reflect.DeepEqual(msg.Permissions, want) {
		t.Errorf("Permissions = %+v, want %+v", msg.Permissions, want)
	}

	saved, err := ss.Load(msg.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.AllowedTools) != 0 || len(saved.EnabledPlugins) != 0 {
		t.Fatalf("imported session saved with permissions: %v / %v", saved.EnabledPlugins, saved.AllowedTools)
	}

	// The user chose to keep them
	if res := m.RestoreImportedPermissionsCmd(msg.Session.ID, msg.Permissions)().(SessionsListMsg); res.Err != nil {
		t.Fatal(res.Err)
	}
	saved, _ = ss.Load(msg.Session.ID)
	if !reflect.DeepEqual(saved.AllowedTools, want.AllowedTools) || !reflect.DeepEqual(saved.EnabledPlugins, want.EnabledPlugins) {
		t.Errorf("after keeping: %v / %v", saved.EnabledPlugins, saved.AllowedTools)
	}
}

func TestImportWithoutPermissions(t *testing.T) {
	m, _ := importModel(t)
	path := writeSessionFile(t, `{"name": "Plain", "model": "m", "messages": [{"role": "user", "content": "hi"}]}`)
	msg := m.ImportSessionCmd(context.Background(), path)().(SessionImportedMsg)
	if msg.Err != nil || !msg.Permissions.Empty() {
		t.Errorf("err %v, permissions %+v", msg.Err, msg.Permissions)
	}
}
