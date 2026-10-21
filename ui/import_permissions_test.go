package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appmodel "otui/model"
	"otui/storage"
)

func importedAppView(t *testing.T, perms appmodel.ImportedPermissions) AppView {
	t.Helper()
	ss, err := storage.NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := typingAppView()
	a.dataModel.SessionStorage = ss
	m, _ := a.handleSessionMessage(sessionImportedMsg{
		Session:     &storage.Session{ID: "imp", Name: "Shared", Messages: []storage.Message{{Role: "user", Content: "hi"}}},
		Permissions: perms,
	})
	return m
}

func TestImportSuccessOffersToKeepPermissions(t *testing.T) {
	perms := appmodel.ImportedPermissions{EnabledPlugins: []string{"fs"}, AllowedTools: []string{"fs.write_file"}}

	a := importedAppView(t, perms)
	if a.sessionImportPicker.Success == nil || a.sessionImportPicker.SuccessFooter == "" {
		t.Fatal("success screen doesn't offer to keep the permissions")
	}
	m, cmd := a.handleSessionImportMode(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if cmd == nil {
		t.Error("k didn't keep the permissions")
	}
	if m.(AppView).sessionImportPicker.Success != nil {
		t.Error("success screen still open after k")
	}

	a = importedAppView(t, perms)
	if _, cmd := a.handleSessionImportMode(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("Enter kept the permissions")
	}
}

func TestImportSuccessWithoutPermissions(t *testing.T) {
	a := importedAppView(t, appmodel.ImportedPermissions{})
	if a.sessionImportPicker.SuccessFooter != "" {
		t.Error("offered to keep permissions that don't exist")
	}
	if _, cmd := a.handleSessionImportMode(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}); cmd != nil {
		t.Error("k did something without permissions")
	}
}
