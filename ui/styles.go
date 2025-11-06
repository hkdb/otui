package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	dimColor       = lipgloss.Color("7")
	accentColor    = lipgloss.Color("12")
	successColor   = lipgloss.Color("10")
	warningColor   = lipgloss.Color("11")
	dangerColor    = lipgloss.Color("9")
	highlightColor = lipgloss.Color("13")

	// User message style
	UserStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)
	// NO .Background() = transparent!

	// Assistant message style
	AssistantStyle = lipgloss.NewStyle().
			Foreground(accentColor)
	// NO .Background() = transparent!

	// System/timestamp style
	DimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	// Border style
	BorderStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	// Title style
	TitleStyle = lipgloss.NewStyle().
			Bold(true)

	// Status bar style
	StatusStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	HelpStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	HighlightStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Bold(true)
)

// FormatFooter formats a footer string with alternating keys and descriptions.
// Keys remain default color, descriptions are rendered in assistant blue+bold.
// Used for navigation/management screens (not main chat).
// Usage: FormatFooter("j/k", "Navigate", "Enter", "Select", "Esc", "Close")
// Result: "j/k Navigate  Enter Select  Esc Close" (with descriptions in assistant blue+bold)
func FormatFooter(parts ...string) string {
	descStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true) // Assistant blue
	var result []string
	for i := 0; i < len(parts); i += 2 {
		if i+1 < len(parts) {
			result = append(result, parts[i]+" "+descStyle.Render(parts[i+1]))
		}
	}
	return strings.Join(result, "  ")
}
