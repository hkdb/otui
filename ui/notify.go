package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// notifyCompleteMsg is sent after the notification has been emitted
type notifyCompleteMsg struct{}

// notifyResponseComplete emits BEL + OSC 9 to stderr.
// stderr avoids interfering with Bubble Tea's stdout rendering.
// Works through Docker because -ti connects stderr to host terminal.
func notifyResponseComplete() tea.Cmd {
	return func() tea.Msg {
		// BEL character - universally supported by terminal emulators
		fmt.Fprint(os.Stderr, "\a")

		// OSC 9 - richer notification for terminals that support it
		// (iTerm2, Windows Terminal, foot, etc.)
		fmt.Fprint(os.Stderr, "\033]9;OTUI: Ready for review\033\\")

		return notifyCompleteMsg{}
	}
}
