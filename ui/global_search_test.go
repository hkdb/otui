package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"otui/storage"
)

func TestGlobalSearchIsDebounced(t *testing.T) {
	ss, err := storage.NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ss.Save(&storage.Session{ID: "s1", Name: "One", Messages: []storage.Message{{Role: "user", Content: "find the needle"}}})

	a := typingAppView()
	a.dataModel.SearchIndex = storage.NewSearchIndex(ss)

	// Two quick queries: only the latest is searched
	first := a.scheduleGlobalSearch("nee")
	second := a.scheduleGlobalSearch("needle")
	if b, cmd := a.handleGlobalSearchMessage(first()); cmd != nil || len(b.globalSearchResults) != 0 {
		t.Error("outdated query was searched")
	}
	b, search := a.handleGlobalSearchMessage(second())
	if search == nil {
		t.Fatal("latest query not searched")
	}
	b, _ = b.handleGlobalSearchMessage(search())
	if len(b.globalSearchResults) != 1 || b.globalSearchResults[0].SessionID != "s1" {
		t.Errorf("results = %+v", b.globalSearchResults)
	}

	// Results of an older query arriving late are dropped
	b.scheduleGlobalSearch("other")
	if c, _ := b.handleGlobalSearchMessage(globalSearchResultsMsg{gen: b.globalSearchGen - 1}); len(c.globalSearchResults) != 1 {
		t.Error("stale results replaced the current ones")
	}

	var _ tea.Msg = globalSearchTickMsg{}
}
