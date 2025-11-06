package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ModalType determines the color and styling of a modal
type ModalType int

const (
	ModalTypeInfo ModalType = iota
	ModalTypeWarning
	ModalTypeError
)

// RenderAcknowledgeModal renders a modal that requires only acknowledgement (Enter to dismiss)
// Used for informational messages, warnings, and errors that don't need user confirmation
func RenderAcknowledgeModal(title, message string, modalType ModalType, width, height int) string {
	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Determine title color based on modal type
	var titleColor lipgloss.Color
	switch modalType {
	case ModalTypeInfo:
		titleColor = accentColor
	case ModalTypeWarning:
		titleColor = warningColor
	case ModalTypeError:
		titleColor = dangerColor
	}

	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(title)

	// Message section (with top border)
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center)

	for _, line := range strings.Split(message, "\n") {
		styledLine := messageStyle.Render(line)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border only)
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render("Press Enter to acknowledge")

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// RenderPluginSystemModal renders plugin system state change modals (startup/shutdown)
// Used by BOTH app quit and Settings toggle for consistent UX
// Phases:
//   - Operation "starting" + Phase "waiting": Spinner + "Starting plugin system..."
//   - Operation "starting" + Phase "error": Error modal (startup failed)
//   - Operation "stopping" + Phase "waiting": Spinner + "Shutting down plugins..."
//   - Operation "stopping" + Phase "unresponsive": Warning + plugin list + y/n choice
func RenderPluginSystemModal(state PluginSystemState, width, height int) string {
	// Starting plugins
	if state.Operation == "starting" {
		if state.Phase == "waiting" {
			return renderSpinner("⚙️  Starting plugin system...", state.Spinner.View(), width, height)
		}
		if state.Phase == "error" {
			return RenderAcknowledgeModal(
				"⚠️  Plugin Startup Failed",
				state.ErrorMsg,
				ModalTypeError,
				width,
				height,
			)
		}
	}

	// Stopping plugins
	if state.Operation == "stopping" {
		if state.Phase == "waiting" {
			return renderSpinner("🔌 Shutting down plugins...", state.Spinner.View(), width, height)
		}
		if state.Phase == "unresponsive" {
			return renderUnresponsiveWarning(state.UnresponsivePlugins, state.ErrorMsg, width, height)
		}
	}

	return ""
}

// renderSpinner renders a simple one-line spinner modal (no borders)
func renderSpinner(message, spinnerView string, width, height int) string {
	content := spinnerView + " " + message
	modalWidth := 40
	if width < modalWidth+10 {
		modalWidth = width - 10
	}
	paddedContent := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center).
		Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, paddedContent)
}

// renderUnresponsiveWarning renders the unresponsive plugins warning modal (borderless three-section)
func renderUnresponsiveWarning(plugins []string, errorMsg string, width, height int) string {
	modalWidth := 55
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Build message lines
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	msg := "These plugins are not responding to shutdown:"
	messageLines = append(messageLines, lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left).
		Render(msg))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Plugin list
	for _, name := range plugins {
		messageLines = append(messageLines, lipgloss.NewStyle().
			Width(modalWidth).
			Align(lipgloss.Left).
			Render("  • "+name))
	}
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Error reason (if any)
	if errorMsg != "" {
		wrapped := wordWrap("Reason: "+errorMsg, modalWidth-4)
		for _, line := range strings.Split(wrapped, "\n") {
			messageLines = append(messageLines, lipgloss.NewStyle().
				Width(modalWidth).
				Foreground(dimColor).
				Align(lipgloss.Left).
				Render(line))
		}
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth))
	}

	// Use RenderThreeSectionModal for consistent borderless pattern
	footer := FormatFooter("y", "Wait", "n", "Quit Now")
	return RenderThreeSectionModal(
		"⚠️  Plugin Shutdown Warning",
		messageLines,
		footer,
		ModalTypeWarning,
		modalWidth,
		width,
		height,
	)
}

// RenderThreeSectionModal renders a borderless modal with title, message, and footer sections
// This is the standard OTUI modal pattern: Title (no border) → Message (BorderTop) → Footer (BorderTop)
// messageLines should be pre-formatted content lines (without padding - padding is added automatically)
// footer should be pre-formatted using FormatFooter() or a simple string
// desiredWidth: preferred modal width (0 = default 60)
func RenderThreeSectionModal(title string, messageLines []string, footer string, modalType ModalType, desiredWidth, width, height int) string {
	modalWidth := desiredWidth
	if modalWidth == 0 {
		modalWidth = 60 // default width
	}
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Determine title color based on modal type
	var titleColor lipgloss.Color
	switch modalType {
	case ModalTypeInfo:
		titleColor = accentColor
	case ModalTypeWarning:
		titleColor = warningColor
	case ModalTypeError:
		titleColor = dangerColor
	}

	// Title section - manually centered using runewidth for accurate emoji handling
	titleVisualWidth := runewidth.StringWidth(title)
	leftPad := (modalWidth-titleVisualWidth)/2 - 2 // Shift 2 spaces left for visual alignment
	if leftPad < 0 {
		leftPad = 0 // Safety check for very long titles
	}
	rightPad := modalWidth - titleVisualWidth - leftPad
	centeredTitle := strings.Repeat(" ", leftPad) + title + strings.Repeat(" ", rightPad)

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		Render(centeredTitle)

	// Message section (with top border and padding)
	var contentLines []string
	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Top padding

	// Add message lines (already formatted by caller)
	for _, line := range messageLines {
		contentLines = append(contentLines, line)
	}

	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(contentLines, "\n"))

	// Footer section (with top border only)
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

// RenderPluginOperationModal renders the plugin enable/disable operation modal
// Shows spinner during operation, success message, or error
func RenderPluginOperationModal(phase string, spinnerView string, pluginName string, errorMsg string, width, height int) string {
	modalWidth := 50
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	var content string

	switch phase {
	case "enabling":
		// Show spinner during enable (spinner is green, text is default)
		content = spinnerView + " Enabling " + pluginName + "..."
	case "disabling":
		// Show spinner during disable - default color
		content = spinnerView + " Disabling " + pluginName + "..."
	case "error":
		// Show error message (no borders)
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(dangerColor).
			Align(lipgloss.Center).
			Width(modalWidth)

		msgStyle := lipgloss.NewStyle().
			Width(modalWidth).
			Align(lipgloss.Left)

		footerStyle := lipgloss.NewStyle().
			Foreground(dimColor).
			Align(lipgloss.Center).
			Width(modalWidth)

		var box strings.Builder
		box.WriteString(titleStyle.Render("⚠️  Plugin Operation Failed") + "\n\n")
		box.WriteString(msgStyle.Render("Plugin: "+pluginName) + "\n\n")

		// Word wrap error message
		wrappedError := lipgloss.NewStyle().Width(modalWidth - 4).Render(errorMsg)
		box.WriteString(msgStyle.Render(wrappedError) + "\n\n")

		box.WriteString(footerStyle.Render("Press Enter to acknowledge"))

		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box.String())
	}

	// For enabling/disabling phase - simple one-line with spinner (no borders)
	paddedContent := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, paddedContent)
}

// buildSystemPromptToolWarningLines creates the formatted message lines for the system prompt + tools warning modal
// Returns lines ready for RenderThreeSectionModal (pre-formatted with proper alignment)
func buildSystemPromptToolWarningLines(systemPrompt string, enabledPlugins []string, modalWidth int) []string {
	var lines []string

	// Style definitions
	leftAlignedStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	// Main warning text
	mainWarning := "Custom system prompts can interfere with plugin tool calls in some models."
	lines = append(lines, leftAlignedStyle.Render(mainWarning))
	lines = append(lines, strings.Repeat(" ", modalWidth)) // Empty line

	// Show the system prompt
	lines = append(lines, leftAlignedStyle.Render("Your system prompt:"))
	quotedPrompt := "\"" + systemPrompt + "\""
	lines = append(lines, leftAlignedStyle.Render(quotedPrompt))
	lines = append(lines, strings.Repeat(" ", modalWidth)) // Empty line

	// Show enabled plugins
	lines = append(lines, leftAlignedStyle.Render("Enabled plugins:"))
	for _, plugin := range enabledPlugins {
		pluginLine := "  • " + plugin
		lines = append(lines, leftAlignedStyle.Render(pluginLine))
	}
	lines = append(lines, strings.Repeat(" ", modalWidth)) // Empty line

	// Strong warning about consequences
	warningStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left).
		Bold(true)
	lines = append(lines, warningStyle.Render("Warning: This may break your session and make it unrecoverable."))
	lines = append(lines, strings.Repeat(" ", modalWidth)) // Empty line

	// Recommendation
	recommendation := "Recommended: Remove the system prompt (Alt+E) or disable plugins."
	lines = append(lines, leftAlignedStyle.Render(recommendation))
	lines = append(lines, strings.Repeat(" ", modalWidth)) // Empty line

	// Explanation
	explanation := "This is a known limitation with Ollama and certain models."
	dimStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left).
		Foreground(dimColor)
	lines = append(lines, dimStyle.Render(explanation))

	return lines
}

// wordWrap wraps text to fit within the specified width while preserving newlines
// Shared helper function for all modals that need text wrapping
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	// Split by newlines first to preserve them
	paragraphs := strings.Split(text, "\n")

	for i, paragraph := range paragraphs {
		if paragraph == "" {
			// Preserve empty lines
			if i > 0 {
				result.WriteString("\n")
			}
			continue
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= width {
				currentLine += " " + word
			} else {
				result.WriteString(currentLine + "\n")
				currentLine = word
			}
		}
		result.WriteString(currentLine)

		// Add newline between paragraphs (but not after the last one)
		if i < len(paragraphs)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}
