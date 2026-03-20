package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type ConfirmationState struct {
	Active  bool
	Title   string
	Message string
}

func RenderConfirmationModal(state ConfirmationState, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
		if modalWidth < 10 {
			modalWidth = 10
		}
	}

	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(state.Title)

	// Message section (with top border)
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center)

	// Add message lines (split by newline)
	for _, line := range strings.Split(state.Message, "\n") {
		messageLines = append(messageLines, messageStyle.Render(line))
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border)
	footer := FormatFooter("y", "Yes", "n", "No")
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// RenderToolWarningModal shows a warning when switching to a model without tool support
func RenderToolWarningModal(modelName string, enabledPlugins []string, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
		if modalWidth < 10 {
			modalWidth = 10
		}
	}

	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("⚠  Warning: Limited Tool Support")

	// Message section (with top border)
	var contentLines []string
	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Top padding

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	// Build message lines
	contentLines = append(contentLines, messageStyle.Render(lipgloss.NewStyle().Bold(true).Render(modelName)+" may not support tool calling."))
	contentLines = append(contentLines, messageStyle.Render(""))

	if len(enabledPlugins) > 0 {
		contentLines = append(contentLines, messageStyle.Render("Your enabled plugins:"))
		for _, plugin := range enabledPlugins {
			contentLines = append(contentLines, messageStyle.Render("  • "+plugin))
		}
		contentLines = append(contentLines, messageStyle.Render(""))
		contentLines = append(contentLines, messageStyle.Render("may not work with this model."))
		contentLines = append(contentLines, messageStyle.Render(""))
	}

	contentLines = append(contentLines, messageStyle.Render(lipgloss.NewStyle().Foreground(accentColor).Render("Recommended:")+" qwen3-coder, llama3.1, mistral"))
	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(contentLines, "\n"))

	// Footer section (with top border)
	footer := FormatFooter("Enter", "Continue Anyway", "Esc", "Cancel")
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// RenderUnsavedChangesModal shows confirmation when user tries to exit with unsaved changes
// Reusable by Settings screen and Provider Settings screen
func RenderUnsavedChangesModal(width, height int) string {
	state := ConfirmationState{
		Active:  true,
		Title:   "Unsaved Changes",
		Message: "You have unsaved changes. Discard them?",
	}
	return RenderConfirmationModal(state, width, height)
}

// RenderCompactionConfirmModal shows compaction confirmation with before/after stats
func RenderCompactionConfirmModal(
	currentTokens, contextWindow int,
	afterTokens int,
	compactedCount, keepCount int,
	width, height int,
) string {
	currentPct := float64(currentTokens) / float64(contextWindow) * 100
	afterPct := float64(afterTokens) / float64(contextWindow) * 100

	message := fmt.Sprintf(
		"Current: %d / %d tokens (%.0f%%)\n"+
			"After:   %d / %d tokens (%.0f%%)\n\n"+
			"Will compact %d messages\n"+
			"Keeping last %d messages\n\n"+
			"⚠  Compacted messages won't be sent to LLM\n"+
			"but remain visible in history\n\n"+
			"Press Y to compact, N to cancel",
		currentTokens, contextWindow, currentPct,
		afterTokens, contextWindow, afterPct,
		compactedCount, keepCount,
	)

	state := ConfirmationState{
		Active:  true,
		Title:   "Compact Session",
		Message: message,
	}

	return RenderConfirmationModal(state, width, height)
}
