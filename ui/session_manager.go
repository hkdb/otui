package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"otui/mcp"
	"otui/storage"
)

func renderSessionManager(sessions []storage.SessionMetadata, selectedIdx int, currentSessionID string, renameMode bool, renameInput textinput.Model, exportMode bool, exportInput textinput.Model, exporting bool, exportCleaningUp bool, exportSpinner spinner.Model, exportSuccess string, importPicker FilePickerState, importSuccess *storage.Session, confirmDelete *storage.SessionMetadata, filterMode bool, filterInput textinput.Model, filteredSessions []storage.SessionMetadata, width, height int) string {
	// Modal dimensions
	modalWidth := width - 10
	if modalWidth > 110 {
		modalWidth = 110
	}
	modalHeight := height - 6

	// Show delete confirmation if set
	if confirmDelete != nil {
		warningText := lipgloss.NewStyle().Foreground(dangerColor).Render("This action cannot be undone.")
		return RenderConfirmationModal(ConfirmationState{
			Active:  true,
			Title:   "⚠ Delete Session",
			Message: fmt.Sprintf("Are you sure you want to delete:\n\n\"%s\"\n\n%s", confirmDelete.Name, warningText),
		}, width, height)
	}

	// Show import modal if in import mode
	if importPicker.Active {
		return RenderFilePickerModal(importPicker, width, height)
	}

	// Show export modal if in export mode
	if exportMode {
		return renderExportModal(exportInput, exporting, exportCleaningUp, exportSpinner, exportSuccess, width, height)
	}

	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Session Manager")

	// Header: show filter input or count
	var header string
	if filterMode {
		header = filterInput.View()
	} else {
		displayList := sessions
		if len(filteredSessions) > 0 {
			displayList = filteredSessions
		}
		if len(sessions) == len(displayList) {
			header = fmt.Sprintf("%d sessions", len(sessions))
		} else {
			header = fmt.Sprintf("%d of %d sessions", len(displayList), len(sessions))
		}
	}

	// Header section (with top and bottom borders)
	headerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(header)

	// Determine which list to display
	displayList := sessions
	if filterMode && len(filteredSessions) > 0 {
		displayList = filteredSessions
	}

	// Session list
	var sessionLines []string
	maxLines := modalHeight - 8 // Reserve space for title, borders, header, footer

	if len(displayList) == 0 {
		emptyMsg := ""
		if filterMode {
			emptyMsg = "No matches found"
		} else {
			emptyMsg = "No sessions yet. Start chatting to create one!"
		}
		emptyMsgStyled := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Align(lipgloss.Center).
			Width(modalWidth).
			Render(emptyMsg)
		sessionLines = append(sessionLines, emptyMsgStyled)
	} else {
		startIdx := 0
		endIdx := len(displayList)

		// Scroll if needed
		if len(displayList) > maxLines {
			if selectedIdx < maxLines/2 {
				endIdx = maxLines
			} else if selectedIdx >= len(displayList)-maxLines/2 {
				startIdx = len(displayList) - maxLines
			} else {
				startIdx = selectedIdx - maxLines/2
				endIdx = startIdx + maxLines
			}
		}

		for i := startIdx; i < endIdx && i < len(displayList); i++ {
			session := displayList[i]

			// Format session line
			indicator := "  "
			if i == selectedIdx {
				indicator = "▶ "
			}

			// Session name (truncate if needed)
			name := session.Name
			maxNameWidth := modalWidth - 40 // Reserve space for metadata + padding (no side borders)

			// Show textinput if in rename mode for this session
			var nameDisplay string
			var hasBullet bool
			if renameMode && i == selectedIdx {
				// Show textinput inline with accent color
				styledInput := lipgloss.NewStyle().
					Foreground(accentColor).
					Bold(true).
					Render(renameInput.View())
				nameDisplay = styledInput
			} else {
				if len(name) > maxNameWidth {
					name = name[:maxNameWidth-3] + "..."
				}
				nameDisplay = name

				// Check if session has system prompt (will add bullet after spacing calculation)
				if session.SystemPrompt != "" {
					hasBullet = true
				}
			}

			// Check if this is the current session (will add marker after spacing calculation)
			hasCurrentMarker := false
			if session.ID == currentSessionID && !renameMode {
				hasCurrentMarker = true
			}

			// Message count
			msgCount := fmt.Sprintf("%d msgs", session.MessageCount)
			if session.MessageCount == 1 {
				msgCount = "1 msg"
			}

			// Model (truncate)
			model := session.Model
			if strings.Contains(model, ":") {
				parts := strings.Split(model, ":")
				model = parts[0] // Just the base name
			}
			if len(model) > 10 {
				model = model[:10]
			}

			// Time ago
			timeAgo := formatTimeAgo(session.UpdatedAt)

			// Style the name display individually BEFORE building leftSide
			nameStyled := nameDisplay
			if i == selectedIdx {
				nameStyled = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(nameDisplay)
			} else if session.ID == currentSessionID {
				nameStyled = lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(nameDisplay)
			}

			// Left side: indicator + styled name (no marker yet - added after spacing)
			leftSide := fmt.Sprintf("%s%s", indicator, nameStyled)

			// Right side: msgCount, model, timeAgo (right-aligned)
			rightSide := fmt.Sprintf("%s  %10s  %8s", msgCount, model, timeAgo)

			// Calculate spacing using VISUAL width (not including ANSI codes)
			leftVisualWidth := len(indicator) + len(nameDisplay)
			spacing := modalWidth - 4 - leftVisualWidth - len(rightSide) // No side borders, just padding

			// Account for VISUAL width of styled markers we'll add (prevents line wrapping from ANSI codes)
			if hasCurrentMarker {
				spacing -= 10 // " (current)" = 10 visible characters
			}
			if hasBullet {
				spacing -= 2 // " •" = 2 visible characters
			}

			if spacing < 2 {
				spacing = 2
			}

			// Add styled markers after spacing calculation
			if hasCurrentMarker {
				// Use green when selected, blue when current but not selected
				markerColor := accentColor // Default to blue for current session
				if i == selectedIdx {
					markerColor = successColor // Green when selected
				}
				currentStyled := lipgloss.NewStyle().Foreground(markerColor).Render("(current)")
				leftSide = leftSide + " " + currentStyled
			}
			if hasBullet {
				bulletStyled := lipgloss.NewStyle().Foreground(accentColor).Render("•")
				leftSide = leftSide + " " + bulletStyled
			}

			// Style the right side individually BEFORE building line
			rightSideStyled := rightSide
			if i == selectedIdx {
				rightSideStyled = lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(rightSide)
			} else if session.ID == currentSessionID {
				rightSideStyled = lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(rightSide)
			}

			// Build the final line with all styled components
			styledLine := fmt.Sprintf("  %s%s%s  ", leftSide, strings.Repeat(" ", spacing), rightSideStyled)

			paddedLine := lipgloss.NewStyle().
				Width(modalWidth).
				Render(styledLine)

			sessionLines = append(sessionLines, paddedLine)
		}
	}

	// Add empty line before and after list
	emptyLine := strings.Repeat(" ", modalWidth)
	sessionLines = append([]string{emptyLine}, sessionLines...)
	sessionLines = append(sessionLines, emptyLine)

	// Footer
	var footerText string
	if renameMode {
		footerText = FormatFooter("Alt+U", "Clear", "Enter", "Save", "Esc", "Cancel")
	} else if filterMode {
		footerText = FormatFooter("Type", "to filter", "Alt+J/K", "Navigate", "Enter", "Load", "Esc", "Cancel")
	} else {
		footerText = FormatFooter("/", "Filter", "j/k", "Navigate", "Enter", "Load", "e", "Edit", "i", "Import", "n", "New", "r", "Rename", "x", "Export", "d", "Delete", "Esc", "Exit")
	}
	// Footer section (with top border only)
	footerSection := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footerText)

	// Combine all sections
	var sections []string
	sections = append(sections, titleSection)
	sections = append(sections, headerSection)
	for _, line := range sessionLines {
		sections = append(sections, line)
	}
	sections = append(sections, footerSection)

	content := strings.Join(sections, "\n")

	// Center the modal
	modalStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return modalStyle.Render(content)
}

// formatTimeAgo formats a time as a relative string (e.g., "2h ago", "3d ago")
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		mins := int(duration.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		return fmt.Sprintf("%dh ago", hours)
	} else if duration < 7*24*time.Hour {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	} else if duration < 30*24*time.Hour {
		weeks := int(duration.Hours() / 24 / 7)
		return fmt.Sprintf("%dw ago", weeks)
	} else {
		months := int(duration.Hours() / 24 / 30)
		return fmt.Sprintf("%dmo ago", months)
	}
}

func renderExportModal(exportInput textinput.Model, exporting bool, cleaningUp bool, exportSpinner spinner.Model, successPath string, width, height int) string {
	// Check for success state first
	if successPath != "" {
		return renderExportSuccess(successPath, "Export", width, height)
	}

	modalWidth := width - 10
	if modalWidth > 80 {
		modalWidth = 80
	}

	// State 3: Cleaning up (BorderTop/BorderBottom pattern like import cleanup)
	if cleaningUp {
		var contentLines []string
		contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Top padding

		cleanupLine := fmt.Sprintf("%s Cleaning up...", exportSpinner.View())
		styledCleanup := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Align(lipgloss.Center).
			Width(modalWidth).
			Render(cleanupLine)

		contentLines = append(contentLines, styledCleanup)
		contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Bottom padding

		content := lipgloss.NewStyle().
			BorderTop(true).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(dimColor).
			Width(modalWidth).
			Render(strings.Join(contentLines, "\n"))

		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}

	// State 2: Processing export (borderless 3-section)
	if exporting {
		title := "Processing Export"

		var messageLines []string
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

		exportLine := fmt.Sprintf("%s Exporting session...", exportSpinner.View())
		styledExport := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Align(lipgloss.Center).
			Width(modalWidth).
			Render(exportLine)

		messageLines = append(messageLines, styledExport)
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

		footer := "Press Esc to cancel"

		return RenderThreeSectionModal(
			title,
			messageLines,
			footer,
			ModalTypeInfo,
			modalWidth,
			width,
			height,
		)
	}

	// State 1: Input mode (borderless 3-section)
	title := "Export Session to JSON"

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	promptStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	messageLines = append(messageLines, promptStyle.Render("  Export to:"))

	inputLine := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Width(modalWidth).
		Align(lipgloss.Left).
		Render("  " + exportInput.View())

	messageLines = append(messageLines, inputLine)
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	footer := "Esc Cancel  Enter Export  Alt+U Clear"

	return RenderThreeSectionModal(
		title,
		messageLines,
		footer,
		ModalTypeInfo,
		modalWidth,
		width,
		height,
	)
}

func renderExportSuccess(exportPath string, operationType string, width, height int) string {
	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Title is dynamic: "✓ Export Successful"
	successTitle := "✓ " + operationType + " Successful"

	// Build message lines with file path
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	pathMsg := fmt.Sprintf("Exported to:\n%s", exportPath)
	wrappedMsg := wordWrap(pathMsg, modalWidth-4)
	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Foreground(accentColor).
		Align(lipgloss.Left)

	for _, line := range strings.Split(wrappedMsg, "\n") {
		styledLine := messageStyle.Render("  " + line)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	// Format footer
	footer := "Press Enter to acknowledge"

	// Custom rendering to use successColor for title (same as renderFilePickerSuccess)
	var titleColor lipgloss.Color = successColor

	// Title section - manually centered
	titleVisualWidth := lipgloss.Width(successTitle)
	leftPad := (modalWidth - titleVisualWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	rightPad := modalWidth - titleVisualWidth - leftPad
	centeredTitle := strings.Repeat(" ", leftPad) + successTitle + strings.Repeat(" ", rightPad)

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		Render(centeredTitle)

	// Message section (with top border)
	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border)
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

func renderImportSuccess(session *storage.Session, width, height int) string {
	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	titleStyled := lipgloss.NewStyle().
		Bold(true).
		Foreground(successColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("✓ Import Successful")

	// Build message with session details (matches mockup)
	message := fmt.Sprintf("Imported: %s\nMessages: %d\nModel: %s",
		session.Name,
		len(session.Messages),
		session.Model)

	wrappedMsg := wordWrap(message, modalWidth-4)
	messageStyled := lipgloss.NewStyle().
		Width(modalWidth - 4).
		Foreground(accentColor).
		Render(wrappedMsg)

	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth - 2).
		Render("Press Enter to acknowledge")

	borderStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Width(modalWidth)

	topBorder := borderStyle.Render("┌" + strings.Repeat("─", modalWidth-2) + "┐")
	middleBorder := borderStyle.Render("├" + strings.Repeat("─", modalWidth-2) + "┤")
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", modalWidth-2) + "┘")
	emptyLine := "│" + strings.Repeat(" ", modalWidth-2) + "│"

	var content strings.Builder
	content.WriteString(topBorder + "\n")
	content.WriteString("│" + titleStyled + "│\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString(emptyLine + "\n")

	// Add message lines
	for _, line := range strings.Split(messageStyled, "\n") {
		paddedLine := lipgloss.NewStyle().
			Width(modalWidth - 2).
			Render("  " + line)
		content.WriteString("│" + paddedLine + "│\n")
	}

	content.WriteString(emptyLine + "\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString("│" + footer + "│\n")
	content.WriteString(bottomBorder)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
	)
}

func renderSessionModal(title string, nameInput textinput.Model, promptInput textarea.Model, focusedField int, width, height int, availablePlugins []mcp.Plugin, enabledPluginIDs []string, unavailablePluginIDs []string, selectedPluginIdx int) string {
	modalWidth := width - 10
	if modalWidth > 80 {
		modalWidth = 80
	}

	// 1. Title section - centered, bold, accentColor
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(title)

	// 2. Message section - build content lines
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	// Session Name field
	nameLabel := "Session Name:"
	nameLabelStyle := lipgloss.NewStyle().Width(modalWidth)
	if focusedField == 0 {
		nameLabelStyle = nameLabelStyle.Foreground(successColor).Bold(true)
	}
	messageLines = append(messageLines, nameLabelStyle.Render("  "+nameLabel))

	nameStyle := lipgloss.NewStyle().Width(modalWidth)
	if focusedField == 0 {
		nameStyle = nameStyle.Foreground(accentColor).Bold(true)
	}
	messageLines = append(messageLines, nameStyle.Render("  "+nameInput.View()))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Spacing

	// System Prompt field
	promptLabel := "System Prompt:"
	promptLabelStyle := lipgloss.NewStyle().Width(modalWidth)
	if focusedField == 1 {
		promptLabelStyle = promptLabelStyle.Foreground(successColor).Bold(true)
	}
	messageLines = append(messageLines, promptLabelStyle.Render("  "+promptLabel))

	promptStyle := lipgloss.NewStyle().Width(modalWidth)
	if focusedField == 1 {
		promptStyle = promptStyle.Foreground(accentColor).Bold(true)
	}
	promptView := promptStyle.Render(promptInput.View())
	for _, line := range strings.Split(promptView, "\n") {
		messageLines = append(messageLines, lipgloss.NewStyle().Width(modalWidth).Render("  "+line))
	}

	// Add plugin section only for edit mode (when we have plugins to show)
	if availablePlugins != nil || len(unavailablePluginIDs) > 0 {
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Spacing

		// Plugin section header
		pluginHeaderLabel := "Session Plugins:"
		pluginHeaderStyle := lipgloss.NewStyle().Width(modalWidth)
		if focusedField == 2 {
			pluginHeaderStyle = pluginHeaderStyle.Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, pluginHeaderStyle.Render("  "+pluginHeaderLabel))
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Spacing

		// Available plugins subsection
		if len(availablePlugins) > 0 {
			availableHeader := lipgloss.NewStyle().
				Width(modalWidth).
				Render("    Available:")
			messageLines = append(messageLines, availableHeader)

			for i, plugin := range availablePlugins {
				// Check if this plugin is enabled
				isEnabled := false
				for _, id := range enabledPluginIDs {
					if id == plugin.ID {
						isEnabled = true
						break
					}
				}

				// Build plugin line
				indicator := "  "
				if i == selectedPluginIdx {
					indicator = "▶ "
				}

				// Determine if this plugin should be highlighted
				isSelected := i == selectedPluginIdx && focusedField == 2

				var checkboxStyled, statusStyled, nameStyled string
				if isSelected {
					// When selected and focused, make everything green
					if isEnabled {
						checkboxStyled = lipgloss.NewStyle().Foreground(successColor).Render("✓")
						statusStyled = lipgloss.NewStyle().Foreground(successColor).Render("[Enabled]")
					} else {
						checkboxStyled = lipgloss.NewStyle().Foreground(successColor).Render("✗")
						statusStyled = lipgloss.NewStyle().Foreground(successColor).Render("[Disabled]")
					}
					nameStyled = lipgloss.NewStyle().Foreground(successColor).Render(plugin.Name)
				} else {
					// Normal colors when not selected
					if isEnabled {
						checkboxStyled = lipgloss.NewStyle().Foreground(successColor).Render("✓")
						statusStyled = lipgloss.NewStyle().Foreground(successColor).Render("[Enabled]")
					} else {
						checkboxStyled = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("✗")
						statusStyled = lipgloss.NewStyle().Foreground(dimColor).Render("[Disabled]")
					}
					nameStyled = plugin.Name // Plain text
				}

				// Format: "    ▶ ✓ plugin-name [Enabled]"
				pluginLine := fmt.Sprintf("    %s%s %s %s", indicator, checkboxStyled, nameStyled, statusStyled)

				lineStyle := lipgloss.NewStyle().Width(modalWidth)
				if isSelected {
					lineStyle = lineStyle.Bold(true)
				}
				messageLines = append(messageLines, lineStyle.Render(pluginLine))
			}
		}

		// Unavailable plugins subsection (manager disabled)
		if len(unavailablePluginIDs) > 0 {
			if len(availablePlugins) > 0 {
				messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Spacing
			}

			unavailableHeader := lipgloss.NewStyle().
				Foreground(dangerColor). // RED header
				Width(modalWidth).
				Render("    Unavailable:")
			messageLines = append(messageLines, unavailableHeader)

			for _, pluginID := range unavailablePluginIDs {
				// Check if this plugin is enabled in the session
				isEnabledInSession := false
				for _, id := range enabledPluginIDs {
					if id == pluginID {
						isEnabledInSession = true
						break
					}
				}

				checkbox := "✗"
				statusText := "[Disabled]"
				if isEnabledInSession {
					statusText = "[Enabled]"
				}

				pluginLine := fmt.Sprintf("      %s %s %s", checkbox, pluginID, statusText)

				styledLine := lipgloss.NewStyle().
					Foreground(dimColor). // Dark gray for dimmed appearance
					Width(modalWidth).
					Render(pluginLine)

				messageLines = append(messageLines, styledLine)
			}
		}
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// 3. Footer section - using FormatFooter helper
	var footer string
	if title == "Edit session" {
		if focusedField == 2 {
			// In plugins section
			footer = FormatFooter("j/k", "Navigate", "e", "Enable", "d", "Disable", "Tab", "Next Field", "Alt+Enter", "Save", "Esc", "Cancel")
		} else {
			footer = FormatFooter("Tab/Shift+Tab", "Switch Fields", "Alt+U", "Clear", "Alt+Enter", "Save", "Esc", "Cancel")
		}
	} else {
		footer = FormatFooter("Tab/Shift+Tab", "Switch Fields", "Enter", "Create", "Esc", "Cancel")
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// 4. Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
