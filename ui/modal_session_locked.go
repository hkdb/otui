package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// sessionLockedModalWidth is the preferred width of the session locked modal
const sessionLockedModalWidth = 64

// showSessionLocked opens the session locked modal for the given session
func (a *AppView) showSessionLocked(sessionID string, pid int) {
	a.showSessionLockedModal = true
	a.lockedSessionID = sessionID
	a.lockedSessionPID = pid
	a.lockedSessionName = ""

	for _, s := range a.sessionList {
		if s.ID == sessionID {
			a.lockedSessionName = s.Name
			break
		}
	}
	if a.lockedSessionName == "" && a.dataModel.SessionStorage != nil {
		if session, err := a.dataModel.SessionStorage.Load(sessionID); err == nil {
			a.lockedSessionName = session.Name
		}
	}
}

// clearSessionLocked closes the session locked modal
func (a *AppView) clearSessionLocked() {
	a.showSessionLockedModal = false
	a.lockedSessionID = ""
	a.lockedSessionPID = 0
	a.lockedSessionName = ""
}

// renderSessionLockedModal renders the modal shown when a session is locked by
// another OTUI instance (mirrors the instance locked modal shown at startup)
func (a AppView) renderSessionLockedModal() string {
	modalWidth := sessionLockedModalWidth
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}
	if modalWidth < 10 {
		modalWidth = 10
	}

	name := a.lockedSessionName
	if name == "" {
		name = "This session"
	} else {
		name = fmt.Sprintf("%q", name)
	}

	inUse := fmt.Sprintf("%s is in use by another OTUI instance.", name)
	if a.lockedSessionPID > 0 {
		inUse += fmt.Sprintf(" Lock held by PID %d.", a.lockedSessionPID)
	}

	paragraphs := []string{
		inUse,
		"Only one instance can use a session at a time.",
		"If that instance crashed or is no longer running, press D to force delete the session lock and open the session anyway.",
	}

	var messageLines []string
	for i, p := range paragraphs {
		if i > 0 {
			messageLines = append(messageLines, safeRepeat(" ", modalWidth))
		}
		for _, line := range wrapWords(p, modalWidth-4) {
			messageLines = append(messageLines, centerTextLine(line, modalWidth))
		}
	}

	footer := FormatFooter("D", "Force delete lock & open", "Esc", "Cancel")

	return RenderThreeSectionModal(
		"⚠️  Session In Use  ⚠️",
		messageLines,
		footer,
		ModalTypeWarning,
		modalWidth,
		a.width,
		a.height,
	)
}

// handleSessionLockedModalUpdate handles key presses while the session locked modal is shown
func (a AppView) handleSessionLockedModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "d", "D":
		sessionID := a.lockedSessionID
		a.clearSessionLocked()

		if config.DebugLog != nil {
			config.DebugLog.Printf("[Session Lock] Force deleting lock and loading session %s", sessionID)
		}

		// Don't leave behind the empty "New Session" that was created on
		// startup in place of the locked session
		if cur := a.dataModel.CurrentSession; cur != nil && cur.ID == a.startupFallbackSessionID &&
			len(cur.Messages) == 0 && len(a.dataModel.Messages) == 0 {
			a.discardSessionOnSwitch = cur.ID
		}
		a.startupFallbackSessionID = ""
		// pendingScrollToMessageIdx is kept so global search jumps still work
		return a, a.dataModel.ForceLoadSession(sessionID)

	case "esc", "enter":
		a.clearSessionLocked()
		a.pendingScrollToMessageIdx = -1
		a.startupFallbackSessionID = "" // User keeps the new session

		// Loading was attempted with no session open (e.g. after deleting the
		// current session) - fall back to a new session
		if a.dataModel.CurrentSession == nil {
			newSession, err := a.dataModel.CreateAndSaveNewSession("New Session", "", []string{})
			if err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[Session Lock] Failed to create new session: %v", err)
				}
				return a, nil
			}
			a.dataModel.Messages = []Message{}
			a.setCurrentSession(newSession)
			a.dataModel.SessionDirty = false
			a.updateViewportContent(true)
			return a, a.dataModel.FetchSessionList()
		}
		return a, nil
	}

	return a, nil
}

// wrapWords wraps text at word boundaries so no line exceeds width columns
func wrapWords(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 || width <= 0 {
		return []string{text}
	}

	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(lines, line)
}
