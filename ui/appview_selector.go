package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"otui/ollama"
)

func renderModelSelector(models []ollama.ModelInfo, selectedIdx int, currentModel string, filterMode bool, filterInput textinput.Model, filteredModels []ollama.ModelInfo, multiProvider bool, width, height int) string {
	// Modal dimensions
	modalWidth := width - 10
	if modalWidth > 80 {
		modalWidth = 80
	}
	modalHeight := height - 6

	// Title
	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Select Model")

	// Header: show filter input or count
	var header string
	if filterMode {
		header = filterInput.View()
	} else {
		displayList := models
		if len(filteredModels) > 0 {
			displayList = filteredModels
		}
		if len(models) == len(displayList) {
			header = fmt.Sprintf("%d models", len(models))
		} else {
			header = fmt.Sprintf("%d of %d models", len(displayList), len(models))
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
	displayList := models
	if filterMode && len(filteredModels) > 0 {
		displayList = filteredModels
	}

	// Model list
	var modelLines []string
	maxLines := modalHeight - 8 // Reserve space for title, borders, header, footer

	if len(displayList) == 0 {
		emptyMsg := ""
		if filterMode {
			emptyMsg = "No matches found"
		} else {
			emptyMsg = "No models available"
		}
		emptyMsgStyled := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Align(lipgloss.Center).
			Width(modalWidth).
			Render(emptyMsg)
		modelLines = append(modelLines, emptyMsgStyled)
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
			model := displayList[i]

			// Format model line
			indicator := "  "
			if i == selectedIdx {
				indicator = "▶ "
			}

			// Parse name and tag
			name := model.Name
			tag := ""
			if parts := strings.Split(model.Name, ":"); len(parts) == 2 {
				name = parts[0]
				tag = parts[1]
			}

			// Format size
			size := formatSize(model.Size)

			// Check if current model
			currentMarker := ""
			if model.Name == currentModel {
				currentMarker = " (current)"
			}

			// Check if model supports tool calling
			toolIndicator := ""
			toolIndicatorWidth := 0
			if ollama.ModelSupportsToolCalling(model.Name) {
				toolIndicator = " [🔧]"
				toolIndicatorWidth = 5 // Visual width: space(1) + bracket(1) + emoji(2) + bracket(1) = 5
			}

			// Build line with proper spacing
			nameWithTag := name
			if tag != "" {
				nameWithTag += ":" + tag
			}

			// Add provider suffix if multiple providers enabled
			providerSuffix := ""
			if multiProvider && model.Provider != "" {
				providerSuffix = fmt.Sprintf(" (%s)", model.Provider)
			}

			maxNameWidth := modalWidth - 20 // Reserve space for size
			if len(nameWithTag)+len(providerSuffix) > maxNameWidth {
				nameWithTag = nameWithTag[:maxNameWidth-len(providerSuffix)-3] + "..."
			}

			// Calculate spacing based on actual tool indicator width (0 if not present, 8 if present)
			spacing := modalWidth - len(indicator) - len(nameWithTag) - len(providerSuffix) - toolIndicatorWidth - len(currentMarker) - len(size) - 4
			if spacing < 1 {
				spacing = 1
			}

			line := fmt.Sprintf("%s%s%s%s%s%s%s",
				indicator,
				nameWithTag,
				providerSuffix,
				toolIndicator,
				currentMarker,
				strings.Repeat(" ", spacing),
				size,
			)

			// Style the line
			lineStyle := lipgloss.NewStyle()
			if i == selectedIdx {
				lineStyle = lineStyle.Foreground(successColor).Bold(true) // Green bold for active selector
			} else if model.Name == currentModel {
				lineStyle = lineStyle.Foreground(accentColor).Bold(true) // Cyan bold for current model
			}

			paddedLine := lipgloss.NewStyle().
				Width(modalWidth).
				Render(lineStyle.Render(line))

			modelLines = append(modelLines, paddedLine)
		}
	}

	// Add empty line before and after list
	emptyLine := strings.Repeat(" ", modalWidth)
	modelLines = append([]string{emptyLine}, modelLines...)
	modelLines = append(modelLines, emptyLine)

	// Footer
	var footerText string
	if filterMode {
		footerText = FormatFooter("Type", "to filter", "Alt+J/K", "Navigate", "Enter", "Select", "Esc", "Cancel")
	} else {
		footerText = FormatFooter("/", "Filter", "j/k", "Navigate", "Enter", "Select", "🔧", "Tool Support", "Esc", "Exit")
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
	for _, line := range modelLines {
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

// formatSize converts bytes to human-readable format
func formatSize(bytes int64) string {
	// Return empty string for unknown sizes (OpenRouter, etc.)
	if bytes == 0 {
		return ""
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
