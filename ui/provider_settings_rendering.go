package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderProviderSettings renders the provider configuration sub-screen
func (a *AppView) renderProviderSettings(width, height int) string {
	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
	}
	if modalWidth < 40 {
		modalWidth = 40
	}

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Provider Settings")

	// Tab bar
	tabBar := a.renderProviderTabs(modalWidth)

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(dimColor).
		Render(strings.Repeat("─", modalWidth))

	// Fields for current provider (from cache map)
	fields := a.providerSettingsState.currentFieldsMap[a.providerSettingsState.selectedProviderID]
	fieldList := a.renderProviderFields(fields, modalWidth)

	// Footer
	footerText := "h/l Tabs  j/k Navigate  Enter Edit/Toggle  Alt+Enter Save  Esc Close"
	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(footerText)

	// Assemble
	var content strings.Builder
	content.WriteString(title + "\n")
	content.WriteString(separator + "\n")
	content.WriteString(tabBar + "\n")
	content.WriteString(separator + "\n")
	content.WriteString(strings.Repeat(" ", modalWidth) + "\n") // Top padding
	content.WriteString(fieldList + "\n")
	content.WriteString(strings.Repeat(" ", modalWidth) + "\n") // Bottom padding
	content.WriteString(separator + "\n")
	content.WriteString(footer)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
	)
}

// renderProviderTabs renders the tab bar (reuses plugin manager pattern)
func (a *AppView) renderProviderTabs(width int) string {
	var tabStrs []string

	for _, providerID := range providerTabs {
		displayName := providerNames[providerID]
		style := lipgloss.NewStyle().Padding(0, 2)

		if providerID == a.providerSettingsState.selectedProviderID {
			style = style.Foreground(accentColor).Bold(true).Underline(true)
		} else {
			style = style.Foreground(dimColor)
		}

		tabStrs = append(tabStrs, style.Render(displayName))
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Top, tabStrs...)

	// Left-align tabs with small indent, pad to full width
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Left).
		Render("  " + tabs)
}

// renderProviderFields renders the field list for current provider
func (a *AppView) renderProviderFields(fields []ProviderField, width int) string {
	if len(fields) == 0 {
		return lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Render("No fields for this provider")
	}

	// Standard label width for alignment (matches Settings screen)
	maxLabelWidth := 20

	var lines []string

	for i, field := range fields {
		line := a.renderProviderField(field, i == a.providerSettingsState.selectedFieldIdx, width, maxLabelWidth)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderProviderField renders a single provider field
func (a *AppView) renderProviderField(field ProviderField, selected bool, width int, maxLabelWidth int) string {
	indicator := "  "
	if selected {
		indicator = "▶ "
	}

	// Edit mode
	if selected && a.providerSettingsState.editMode {
		labelPadding := strings.Repeat(" ", 20-len(field.Label))
		inputBox := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Width(width - 24).
			Render(a.providerSettingsState.editInput.View())
		return fmt.Sprintf("  %s%s%s", field.Label, labelPadding, inputBox)
	}

	// Display mode
	label := fmt.Sprintf("%s%s", indicator, field.Label)
	// Pad label to standard width for value alignment
	if len(label) < maxLabelWidth {
		label = label + strings.Repeat(" ", maxLabelWidth-len(label))
	}

	value := field.Value
	maxValueWidth := width - maxLabelWidth - 4
	if len(value) > maxValueWidth {
		value = value[:maxValueWidth-3] + "..."
	}

	// Build line (label + value, no styling yet)
	line := label + value

	// Apply Enabled field coloring ONLY when NOT selected (follows Settings pattern)
	if !selected && field.Type == ProviderFieldTypeEnabled {
		// Color just the value portion based on true/false
		switch value {
		case "true":
			value = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(value)
		case "false":
			value = lipgloss.NewStyle().Foreground(dimColor).Render(value)
		}
		line = label + value
	}

	// Apply selection styling to ENTIRE line (follows Settings pattern)
	if selected {
		line = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true).
			Render(line)
	}

	// Pad to full width with left alignment to keep fields left-aligned within modal
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Left).
		Render(line)
}
