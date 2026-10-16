package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
	appmodel "otui/model"
	"otui/storage"
)

// streamingAppView returns an AppView in session A with a response in
// progress and session B in the session list
func streamingAppView() AppView {
	sessionA := &storage.Session{ID: "a", Name: "A"}
	a := AppView{dataModel: &appmodel.Model{
		Config:         &config.Config{Keybindings: config.DefaultKeybindings()},
		CurrentSession: sessionA,
		Streaming:      true,
	}}
	a.sessionList = []storage.SessionMetadata{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	a.selectedSessionIdx = 1
	a.globalSearchResults = []storage.SessionMessageMatch{{SessionID: "b"}}
	return a
}

func assertSwitchBlocked(t *testing.T, name string, a AppView, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		t.Errorf("%s: returned a command (session switch) while a response is in progress", name)
	}
	if !a.showAcknowledgeModal {
		t.Errorf("%s: no warning shown", name)
	}
	if a.dataModel.CurrentSession.ID != "a" || !a.dataModel.Streaming {
		t.Errorf("%s: session or response changed", name)
	}
}

func TestSessionSwitchBlockedWhileStreaming(t *testing.T) {
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	a := streamingAppView()
	a.showSessionManager = true
	b, cmd := a.handleSessionManagerUpdate(enter)
	assertSwitchBlocked(t, "session manager Enter", b, cmd)

	a = streamingAppView()
	a.showSessionManager = true
	a.sessionFilterMode = true
	b, cmd = a.handleSessionManagerUpdate(enter)
	assertSwitchBlocked(t, "session filter Enter", b, cmd)

	a = streamingAppView()
	a.showSessionManager = true
	b, cmd = a.handleSessionManagerUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	assertSwitchBlocked(t, "session manager n", b, cmd)
	if b.showNewSessionModal {
		t.Error("new session modal opened while a response is in progress")
	}

	a = streamingAppView()
	a.showGlobalSearch = true
	b, cmd = a.handleGlobalSearchUpdate(enter)
	assertSwitchBlocked(t, "global search Enter", b, cmd)

	a = streamingAppView()
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n"), Alt: true})
	assertSwitchBlocked(t, "Alt+N", m.(AppView), cmd)
}

func TestSessionReplacedWhileStreamingCancelsRequest(t *testing.T) {
	a := streamingAppView()
	old := a.dataModel.RequestID()
	a.setCurrentSession(&storage.Session{ID: "b"})
	if a.dataModel.Streaming || a.dataModel.RequestID() == old {
		t.Error("request of the previous session not cancelled")
	}
}

func TestModalGetsKeysWhilePermissionPending(t *testing.T) {
	a := streamingAppView()
	a.waitingForPermission = true
	a.pendingPermission = &toolPermissionRequestMsg{ToolName: "x.y"}
	a.showAcknowledgeModal = true

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	b := m.(AppView)
	if b.showAcknowledgeModal {
		t.Error("Enter didn't close the modal while a permission request was pending")
	}
	if !b.waitingForPermission || b.pendingPermission == nil {
		t.Error("permission request was answered by a key meant for the modal")
	}
}
