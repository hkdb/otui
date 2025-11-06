package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type ConfirmationState struct {
	Active  bool
	Title   string
	Message string
}

func RenderConfirmationModal(state ConfirmationState, width, height int) string {
	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
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
	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
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
