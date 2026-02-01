package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"otui/mcp"
)

// renderPluginTabs renders the tab selection (Installed | Curated | Official | etc.)
func (a *AppView) renderPluginTabs() string {
	tabs := []struct {
		name  string
		label string
	}{
		{"installed", "Installed"},
		{"curated", "Curated"},
		{"official", "Official"},
		{"automatic", "Automatic"},
		{"manual", "Manual"},
		{"custom", "Custom"},
		{"all", "All"},
	}

	var tabStrs []string
	for _, tab := range tabs {
		style := lipgloss.NewStyle().Padding(0, 2)
		if tab.name == a.pluginManagerState.selection.viewMode {
			style = style.Foreground(accentColor).Bold(true).Underline(true)
		} else {
			style = style.Foreground(dimColor)
		}
		tabStrs = append(tabStrs, style.Render(tab.label))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabStrs...)
}

// renderPluginList renders the main plugin list view
func (a *AppView) renderPluginList(plugins []mcp.Plugin, height, width int) string {
	if len(plugins) == 0 {
		// Check if registry is empty (no plugins fetched yet)
		allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
		message := "No plugins available."
		if len(allPlugins) == 0 {
			message = fmt.Sprintf("No plugins available. Press %s to fetch plugins from the official OTUI registry.", a.formatKeyDisplay("primary", "R"))
		}

		empty := lipgloss.NewStyle().
			Foreground(accentColor).
			Italic(true).
			Padding(0, 2).
			Render(message)
		return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Center, empty)
	}

	a.pluginManagerState.selection.selectedPluginIdx = min(a.pluginManagerState.selection.selectedPluginIdx, len(plugins)-1)
	a.pluginManagerState.selection.selectedPluginIdx = max(a.pluginManagerState.selection.selectedPluginIdx, 0)

	if a.pluginManagerState.selection.selectedPluginIdx < a.pluginManagerState.selection.scrollOffset {
		a.pluginManagerState.selection.scrollOffset = a.pluginManagerState.selection.selectedPluginIdx
	}
	if a.pluginManagerState.selection.selectedPluginIdx >= a.pluginManagerState.selection.scrollOffset+height {
		a.pluginManagerState.selection.scrollOffset = a.pluginManagerState.selection.selectedPluginIdx - height + 1
	}

	var items []string
	visibleEnd := a.pluginManagerState.selection.scrollOffset + height
	visibleEnd = min(visibleEnd, len(plugins))

	for i := a.pluginManagerState.selection.scrollOffset; i < visibleEnd; i++ {
		plugin := plugins[i]
		items = append(items, a.renderPluginItem(plugin, i == a.pluginManagerState.selection.selectedPluginIdx, width))
	}

	return strings.Join(items, "\n")
}

// stripEmojisAndSymbols removes emoji and special symbols from a string
func stripEmojisAndSymbols(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 0x1F300 && r <= 0x1FAFF) ||
			(r >= 0x1F900 && r <= 0x1F9FF) ||
			(r >= 0x2600 && r <= 0x27BF) ||
			(r >= 0xFE00 && r <= 0xFE0F) {
			continue
		}
		result.WriteRune(r)
	}
	return strings.TrimSpace(result.String())
}

// renderPluginItem renders a single plugin item in the list
func (a *AppView) renderPluginItem(plugin mcp.Plugin, selected bool, width int) string {
	installed := a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID)

	indicator := "  "
	if selected {
		indicator = "▶ "
	}

	var statusText string
	var statusColor lipgloss.Color

	if a.pluginManagerState.selection.viewMode == "installed" {
		if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Config.GetPluginEnabled(plugin.ID) {
			statusText = "✓ Enabled"
			statusColor = successColor
		} else {
			statusText = "✗ Disabled"
			statusColor = dangerColor
		}
	} else {
		if installed {
			statusText = "✓ Installed"
			statusColor = successColor
		} else {
			statusText = "Available"
			statusColor = dimColor
		}
	}

	// Show plugin name in Installed tab, description in other tabs
	displayText := plugin.Description
	if a.pluginManagerState.selection.viewMode == "installed" {
		displayText = plugin.Name
	}
	description := stripEmojisAndSymbols(displayText)
	descWidth := 50

	if runewidth.StringWidth(description) > descWidth {
		description = runewidth.Truncate(description, descWidth, "...")
	}

	description = strings.TrimRight(description, " ")

	category := fmt.Sprintf("[%s]", plugin.Category)

	stars := ""
	if plugin.Stars > 0 {
		stars = fmt.Sprintf("⭐ %d", plugin.Stars)
	}

	statusWidth := 11
	categoryWidth := 15

	statusPadded := statusText + strings.Repeat(" ", statusWidth-runewidth.StringWidth(statusText))
	statusStyled := lipgloss.NewStyle().Foreground(statusColor).Render(statusPadded)

	descPadded := description + strings.Repeat(" ", descWidth-runewidth.StringWidth(description))
	categoryPadded := category + strings.Repeat(" ", categoryWidth-runewidth.StringWidth(category))

	if selected {
		descPadded = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(descPadded)
		categoryPadded = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(categoryPadded)
		stars = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(stars)
	}

	line := fmt.Sprintf("%s%s  %s  %s  %s", indicator, statusStyled, descPadded, categoryPadded, stars)

	styledLine := line

	return lipgloss.NewStyle().Padding(0, 2).Render(styledLine)
}

// renderPluginFooter renders the footer with keyboard shortcuts
func (a *AppView) renderPluginFooter() string {
	if a.pluginManagerState.selection.filterMode {
		footer := FormatFooter(
			a.formatKeyDisplay("primary", "j/k"), "Navigate",
			"Enter", "Details",
			a.formatKeyDisplay("primary", "i"), "Install",
			"Esc", "Cancel",
		)
		return lipgloss.NewStyle().Padding(1, 2, 0, 2).Render(footer)
	}

	// Base keys for all modes
	footerParts := []string{
		"j/k", "Navigate",
		"←/→", "Switch View",
		"Enter", "Details",
	}

	// Add context-specific keys
	switch a.pluginManagerState.selection.viewMode {
	case "custom":
		footerParts = append(footerParts, "a", "Add Custom")
		plugins := a.getVisiblePlugins()
		if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if plugin.Custom {
				footerParts = append(footerParts, "e", "Edit")
			}
			if !a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID) {
				footerParts = append(footerParts, "i", "Install")
			}
			footerParts = append(footerParts, "d", "Delete")
		}
	case "installed":
		plugins := a.getVisiblePlugins()
		if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Config.GetPluginEnabled(plugin.ID) {
				footerParts = append(footerParts, "d", "Disable")
			} else {
				footerParts = append(footerParts, "e", "Enable")
			}
		}
	default:
		plugins := a.getVisiblePlugins()
		if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if !a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID) {
				footerParts = append(footerParts, "i", "Install")
			}
		}
	}

	// Add common keys
	footerParts = append(footerParts, a.formatKeyDisplay("primary", "R"), "Refresh", "Esc", "Close")

	footer := FormatFooter(footerParts...)
	return lipgloss.NewStyle().Padding(1, 2, 0, 2).Render(footer)
}

// renderPluginDetails renders the detailed view of a selected plugin
func (a *AppView) renderPluginDetails() string {
	if a.pluginManagerState.detailsModal.plugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.detailsModal.plugin

	// Build message lines
	modalWidth := 80
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor)

	details := []struct {
		label string
		value string
	}{
		{"Description", plugin.Description},
		{"Category", plugin.Category},
		{"Author", plugin.Author},
		{"Repository", plugin.Repository},
		{"Install Type", plugin.InstallType},
		{"License", plugin.License},
	}

	if plugin.Stars > 0 {
		details = append(details, struct{ label, value string }{"Stars", fmt.Sprintf("%d", plugin.Stars)})
	}

	var messageLines []string
	for _, d := range details {
		if d.value == "" {
			continue
		}

		line := fmt.Sprintf("%s: %s", labelStyle.Render(d.label), d.value)
		wrapped := wrapText(line, modalWidth-4)
		for _, wl := range wrapped {
			messageLines = append(messageLines, messageStyle.Render(wl))
		}
	}

	// Dynamic footer based on install status
	installed := a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID)
	var footer string

	if !installed {
		footer = FormatFooter("i", "Install", "Esc", "Close")
		return RenderThreeSectionModal(plugin.Name, messageLines, footer, ModalTypeInfo, 80, a.width, a.height)
	}

	// Plugin is installed - show uninstall and configure options
	footer = FormatFooter("u", "Uninstall", "c", "Configure", "Esc", "Close")

	// Add edit option for custom plugins
	if plugin.Custom {
		footer = FormatFooter("u", "Uninstall", "c", "Configure", "e", "Edit", "Esc", "Close")
	}

	return RenderThreeSectionModal(plugin.Name, messageLines, footer, ModalTypeInfo, 80, a.width, a.height)
}

// renderProgressBar renders a progress bar with percentage
func renderProgressBar(percent int, width int) string {
	if width < 10 {
		width = 10
	}

	filled := (percent * width) / 100
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	percentStr := fmt.Sprintf("%d%%", percent)

	return lipgloss.NewStyle().
		Foreground(accentColor).
		Render(fmt.Sprintf("[%s] %s", bar, percentStr))
}

// wrapText wraps text to fit within a given width
func wrapText(text string, width int) []string {
	if runewidth.StringWidth(text) <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	var currentLine string

	for _, word := range words {
		wordWidth := runewidth.StringWidth(word)
		currentWidth := runewidth.StringWidth(currentLine)

		if wordWidth > width {
			if currentLine != "" {
				lines = append(lines, currentLine)
				currentLine = ""
			}
			for wordWidth > width {
				chunk := runewidth.Truncate(word, width, "")
				lines = append(lines, chunk)
				word = word[len(chunk):]
				wordWidth = runewidth.StringWidth(word)
			}
			currentLine = word
		} else if currentWidth+wordWidth+1 <= width {
			if currentLine != "" {
				currentLine += " "
			}
			currentLine += word
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
