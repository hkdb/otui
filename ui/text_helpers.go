package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// safeRepeat is strings.Repeat that returns "" for counts <= 0 instead of
// panicking. Layout code computes counts from the terminal size, which can go
// negative in small terminals.
func safeRepeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(s, count)
}

// truncateWidth shortens s to at most maxWidth display columns, ending with
// "..." when there's room for it. Never splits a multi-byte character.
func truncateWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth < 3 {
		return runewidth.Truncate(s, maxWidth, "")
	}
	return runewidth.Truncate(s, maxWidth, "...")
}

// Minimum terminal size the UI is laid out for. Below this a "too small"
// message is shown instead of a broken layout.
const (
	minTerminalWidth  = 40
	minTerminalHeight = 12
)

// terminalTooSmall reports whether the terminal is below the minimum size
func terminalTooSmall(width, height int) bool {
	return width < minTerminalWidth || height < minTerminalHeight
}

// renderTerminalTooSmall renders the message shown when the terminal is too small
func renderTerminalTooSmall(width, height int) string {
	msg := fmt.Sprintf("Terminal too small\n\nResize to at least %d×%d\n(currently %d×%d)",
		minTerminalWidth, minTerminalHeight, width, height)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().Foreground(warningColor).Align(lipgloss.Center).Render(msg))
}
