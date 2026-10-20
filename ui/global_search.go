package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"otui/storage"
)

// Global search runs in the background once typing pauses: searching
// every session on each keystroke would block the UI.

// globalSearchDelay is how long typing must pause before searching
const globalSearchDelay = 150 * time.Millisecond

// globalSearchTickMsg starts the search for query if it's still the latest
type globalSearchTickMsg struct {
	gen   int
	query string
}

// globalSearchResultsMsg carries the results of search gen
type globalSearchResultsMsg struct {
	gen     int
	results []storage.SessionMessageMatch
	err     error
}

// scheduleGlobalSearch debounces a change of the search query
func (a *AppView) scheduleGlobalSearch(query string) tea.Cmd {
	a.globalSearchGen++
	if query == "" || a.dataModel.SearchIndex == nil {
		a.globalSearchResults = []storage.SessionMessageMatch{}
		a.selectedGlobalIdx = 0
		return nil
	}
	gen := a.globalSearchGen
	return tea.Tick(globalSearchDelay, func(time.Time) tea.Msg {
		return globalSearchTickMsg{gen: gen, query: query}
	})
}

// handleGlobalSearchMessage handles the debounce tick and the results
func (a AppView) handleGlobalSearchMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case globalSearchTickMsg:
		if msg.gen != a.globalSearchGen || a.dataModel.SearchIndex == nil {
			return a, nil // Typed more since
		}
		index := a.dataModel.SearchIndex
		return a, func() tea.Msg {
			results, err := index.SearchAllSessions(msg.query)
			return globalSearchResultsMsg{gen: msg.gen, results: results, err: err}
		}

	case globalSearchResultsMsg:
		if msg.gen != a.globalSearchGen || msg.err != nil {
			return a, nil
		}
		a.globalSearchResults = msg.results
		a.selectedGlobalIdx = 0
		a.globalSearchScrollIdx = 0
		return a, nil
	}
	return a, nil
}
