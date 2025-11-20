package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	markdown "github.com/MichaelMure/go-term-markdown"
	tea "github.com/charmbracelet/bubbletea"
	gomarkdown "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"

	"otui/config"
)

// Pre-compiled regex patterns for better performance
var (
	inlineCodeRegex = regexp.MustCompile(`(?s)\x1b\[44;3m(.*?)\x1b\[0m`)
	mdLinkRegex     = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\)]+)\)`)
	urlRegex        = regexp.MustCompile(`(https?://[^\s]+)`)
)

func (a *AppView) updateViewportContent(gotoBottom bool) {
	if len(a.dataModel.Messages) == 0 {
		a.viewport.SetContent("No messages yet. Start chatting!")
		return
	}

	var content strings.Builder

	for i, msg := range a.dataModel.Messages {
		highlightPrefix := ""
		if i == a.highlightedMessageIdx && a.highlightFlashCount%2 == 1 {
			highlightPrefix = HighlightStyle.Render(">>> ")
		}

		timestamp := DimStyle.Render(msg.Timestamp.Format("[15:04]"))

		var roleStyle = DimStyle
		var roleName string
		switch msg.Role {
		case "user":
			roleStyle = UserStyle
			roleName = "You"
		case "assistant":
			roleStyle = AssistantStyle
			roleName = "Assistant"
		default:
			roleStyle = DimStyle
			roleName = "System"
		}

		role := roleStyle.Render(roleName)

		renderedContent := msg.Rendered

		// Special handling for permission request messages
		if msg.Role == "system" && strings.HasPrefix(msg.Content, "🔒 Permission Request") {
			formattedPermission := formatPermissionMessage(timestamp, renderedContent)
			content.WriteString(formattedPermission)
			continue
		}

		// Special handling for loading spinner
		if msg.Role == "system" && msg.Content == "Waiting for response..." {
			renderedContent = fmt.Sprintf("%s %s", a.loadingSpinner.View(), msg.Content)
		}

		// Special handling for step message spinner (Phase 2)
		if msg.Role == "system" && strings.HasPrefix(msg.Content, "🔧 ") && strings.HasSuffix(msg.Content, "...") {
			renderedContent = fmt.Sprintf("%s %s", msg.Content, a.loadingSpinner.View())
		}

		// Special handling for "Analyzing results..." spinner (Phase 2)
		if msg.Role == "system" && msg.Content == "Analyzing results..." {
			renderedContent = fmt.Sprintf("%s %s", a.loadingSpinner.View(), msg.Content)
		}

		// User messages with vertical bar formatting
		if msg.Role == "user" {
			formattedUser := formatUserMessage(highlightPrefix, timestamp, role, renderedContent)
			content.WriteString(formattedUser)
			continue
		}

		// Default formatting for assistant and other system messages
		content.WriteString(fmt.Sprintf("%s%s %s\n%s\n\n", highlightPrefix, timestamp, role, renderedContent))
	}

	a.viewport.SetContent(content.String())
	if gotoBottom {
		a.viewport.GotoBottom()
	}
}

func (a *AppView) updateStreamingMessage() {
	if len(a.dataModel.Messages) == 0 {
		return
	}

	var content strings.Builder

	// Render all previous messages
	for _, msg := range a.dataModel.Messages {
		timestamp := DimStyle.Render(msg.Timestamp.Format("[15:04]"))

		var roleStyle = DimStyle
		var roleName string
		switch msg.Role {
		case "user":
			roleStyle = UserStyle
			roleName = "You"
		case "assistant":
			roleStyle = AssistantStyle
			roleName = "Assistant"
		default:
			roleStyle = DimStyle
			roleName = "System"
		}

		role := roleStyle.Render(roleName)

		if msg.Role == "user" {
			formattedUser := formatUserMessage("", timestamp, role, msg.Rendered)
			content.WriteString(formattedUser)
		} else {
			content.WriteString(fmt.Sprintf("%s %s\n%s\n\n", timestamp, role, msg.Rendered))
		}
	}

	// Add streaming message (assistant - flush left)
	timestamp := DimStyle.Render(time.Now().Format("[15:04]"))
	role := AssistantStyle.Render("Assistant")

	// Show spinner while waiting for first chunk, then show text with cursor
	streamContent := a.loadingSpinner.View()
	if a.currentResp.String() != "" {
		streamContent = a.currentResp.String() + "▋"
	}

	content.WriteString(fmt.Sprintf("%s %s\n%s\n\n", timestamp, role, streamContent))

	a.viewport.SetContent(content.String())
	a.viewport.GotoBottom()
}

func formatUserMessage(highlightPrefix, timestamp, role, content string) string {
	greenBold := "\x1b[32;1m"
	reset := "\x1b[0m"
	bar := greenBold + "┃" + reset

	lines := strings.Split(content, "\n")

	var result strings.Builder
	result.WriteString(fmt.Sprintf("%s%s %s %s\n", highlightPrefix, bar, timestamp, role))

	for _, line := range lines {
		result.WriteString(fmt.Sprintf("%s %s\n", bar, line))
	}

	result.WriteString("\n")

	return result.String()
}

func postProcessMarkdown(rendered string, width int) string {
	// 1. Fix inline code: Blue background → Red text (glamour style)
	rendered = fixInlineCode(rendered)

	// 2. Color plain URLs red (autolink disabled keeps URLs plain)
	rendered = fixMarkdownLinks(rendered)

	// 3. Frame code blocks with green horizontal lines
	rendered = frameCodeBlocks(rendered, width)

	return rendered
}

func preprocessLinks(content string) string {
	// Strip markdown link syntax [text](url) → just url
	// This ensures all links appear as plain URLs that will be colored red
	return mdLinkRegex.ReplaceAllString(content, "$2")
}

func fixInlineCode(s string) string {
	// Replace: \x1b[44;3m...text...\x1b[0m (Blue BG + Italic)
	// With:    \x1b[31m...text...\x1b[0m (Red text)
	return inlineCodeRegex.ReplaceAllString(s, "\x1b[31m$1\x1b[0m")
}

func fixMarkdownLinks(s string) string {
	// Color plain URLs red for visual distinction
	// Autolink is disabled in parser, so URLs are plain text (not wrapped in [url](url))
	redColor := "\x1b[31m"
	reset := "\x1b[0m"

	lines := strings.Split(s, "\n")

	for i, line := range lines {
		// Skip code blocks (they have ┃ prefix from glamour rendering)
		if !strings.Contains(line, "┃") {
			lines[i] = urlRegex.ReplaceAllString(line, redColor+"$1"+reset)
		}
	}

	return strings.Join(lines, "\n")
}

func frameCodeBlocks(s string, width int) string {
	lines := strings.Split(s, "\n")
	var result []string
	var codeBlockLines []string
	inCodeBlock := false

	// Dark gray ANSI code for subtle borders
	darkGray := "\x1b[90m" // Bright black (dark gray)
	reset := "\x1b[0m"

	for _, line := range lines {
		// Detect code block line (contains green ┃)
		if strings.Contains(line, "┃") {
			if !inCodeBlock {
				// Start of code block - add margin, top border with [code] label, and padding
				inCodeBlock = true
				codeBlockLines = []string{}
				result = append(result, "") // Add blank line for top margin (outside border)

				// Create top border with [code] label centered
				label := "[code]"
				labelLen := len(label)
				lineLen := width - 4
				leftLen := (lineLen - labelLen) / 2
				rightLen := lineLen - labelLen - leftLen
				border := darkGray + strings.Repeat("━", leftLen) + reset + label + darkGray + strings.Repeat("━", rightLen) + reset

				result = append(result, border)
				result = append(result, "") // Add blank line for top padding (inside border)
			}

			// Strip ┃ prefix and keep syntax highlighting
			cleanLine := stripCodeBlockPrefix(line)
			codeBlockLines = append(codeBlockLines, cleanLine)

		} else {
			if inCodeBlock {
				// End of code block - add padding, bottom border, and margin
				result = append(result, codeBlockLines...)
				result = append(result, "") // Add blank line for bottom padding (inside border)
				border := darkGray + strings.Repeat("━", width-4) + reset
				result = append(result, border)
				result = append(result, "") // Add blank line for bottom margin (outside border)

				codeBlockLines = nil
				inCodeBlock = false
			}
			result = append(result, line)
		}
	}

	// Handle code block at end of content
	if inCodeBlock && len(codeBlockLines) > 0 {
		result = append(result, codeBlockLines...)
		result = append(result, "") // Add blank line for bottom padding (inside border)
		border := darkGray + strings.Repeat("━", width-4) + reset
		result = append(result, border)
		result = append(result, "") // Add blank line for bottom margin (outside border)
	}

	return strings.Join(result, "\n")
}

func stripCodeBlockPrefix(line string) string {
	// Find ┃ and remove everything before and including it (plus the space)
	idx := strings.Index(line, "┃")
	if idx >= 0 {
		// Skip past "┃"
		after := idx + len("┃")
		// Skip the space after ┃ if present
		if after < len(line) && line[after] == ' ' {
			after++
		}
		if after < len(line) {
			return line[after:]
		}
		return ""
	}
	return line
}

func (a AppView) renderMarkdownAsync(messageIndex int, content string) tea.Cmd {
	return func() tea.Msg {
		if config.DebugLog != nil {
			config.DebugLog.Printf("Starting async markdown render for message %d - length: %d chars", messageIndex, len(content))
		}
		startTime := time.Now()

		// Preprocess: strip markdown link syntax [text](url) → url
		// This ensures all links appear as plain red URLs regardless of format
		content = preprocessLinks(content)

		// Render with go-term-markdown (simple, fast, lightweight)
		// Disable autolink extension to keep plain URLs as plain text
		// This allows terminal emulators to handle URL detection and clickability
		defaultExt := markdown.Extensions()
		customExt := defaultExt &^ parser.Autolink
		p := parser.NewWithExtensions(customExt)
		r := markdown.NewRenderer(a.width-4, 0)
		doc := p.Parse([]byte(content))
		rendered := gomarkdown.Render(doc, r)

		// Post-process: fix inline code colors and frame code blocks
		processed := postProcessMarkdown(string(rendered), a.width)

		elapsed := time.Since(startTime)
		if config.DebugLog != nil {
			config.DebugLog.Printf("Markdown rendered and post-processed in %v", elapsed)
		}

		return markdownRenderedMsg{
			MessageIndex: messageIndex,
			Rendered:     processed,
		}
	}
}

// wordWrapWithIndent wraps text to maxWidth while preserving indentation for continuation lines
func wordWrapWithIndent(text string, prefix string, maxWidth int) string {
	// Calculate available width for text
	prefixLen := len(stripANSI(prefix))
	availableWidth := maxWidth - prefixLen

	if availableWidth <= 0 {
		return prefix + text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return prefix
	}

	var result strings.Builder
	var currentLine strings.Builder
	indent := strings.Repeat(" ", prefixLen)
	isFirstLine := true

	for _, word := range words {
		// Check if adding this word would exceed width
		testLen := currentLine.Len()
		if testLen > 0 {
			testLen++ // Space before word
		}
		testLen += len(word)

		if testLen > availableWidth && currentLine.Len() > 0 {
			// Flush current line
			if isFirstLine {
				result.WriteString(prefix)
				isFirstLine = false
			} else {
				result.WriteString(indent)
			}
			result.WriteString(currentLine.String())
			result.WriteString("\n")
			currentLine.Reset()
		}

		// Add word to current line
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	// Flush remaining line
	if currentLine.Len() > 0 {
		if isFirstLine {
			result.WriteString(prefix)
		} else {
			result.WriteString(indent)
		}
		result.WriteString(currentLine.String())
		result.WriteString("\n")
	}

	return result.String()
}

// stripANSI removes ANSI escape codes for accurate length calculation
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

// buildPermissionContent creates the formatted permission request content
func buildPermissionContent(toolName, purpose string, details map[string]string) string {
	var content strings.Builder
	maxWidth := 60 // Conservative width for wrapping

	// Title
	content.WriteString("🔒 Permission Request\n\n")

	// Tool name
	toolLine := wordWrapWithIndent(toolName, "╰── Tool: ", maxWidth)
	content.WriteString(toolLine)

	// Purpose (with word wrapping)
	if purpose != "" {
		purposeLine := wordWrapWithIndent(purpose, "╰── Purpose: ", maxWidth)
		content.WriteString(purposeLine)
	}

	// Details (file paths, commands, etc.)
	if len(details) > 0 {
		for key, value := range details {
			// Capitalize first letter of key
			displayKey := strings.ToUpper(key[:1]) + key[1:]
			detailLine := wordWrapWithIndent(value, "╰── "+displayKey+": ", maxWidth)
			content.WriteString(detailLine)
		}
	}

	// Action prompt
	content.WriteString("\n")
	greenBold := "\x1b[32;1m"
	blueBold := "\x1b[34;1m"
	redBold := "\x1b[31;1m"
	reset := "\x1b[0m"

	content.WriteString(greenBold + "[y]" + reset + " Yes    ")
	content.WriteString(blueBold + "[a]" + reset + " Always    ")
	content.WriteString(redBold + "[n]" + reset + " No")

	return content.String()
}

// formatPermissionMessage renders a permission request with vertical bar (like formatUserMessage)
func formatPermissionMessage(timestamp, content string) string {
	// Use accent color (cyan/assistant color) for permission messages
	accentBold := "\x1b[36;1m"
	reset := "\x1b[0m"
	bar := accentBold + "│" + reset

	lines := strings.Split(content, "\n")

	var result strings.Builder
	result.WriteString(fmt.Sprintf("%s %s %s\n", bar, timestamp, DimStyle.Render("System:")))

	for _, line := range lines {
		result.WriteString(fmt.Sprintf("%s %s\n", bar, line))
	}

	result.WriteString("\n")

	return result.String()
}

// buildIterationSummary formats multi-step summary (Phase 2)
func buildIterationSummary(summary IterationSummaryMsg) string {
	var b strings.Builder

	// Header
	if summary.MaxReached {
		b.WriteString("⚠️  Multi-step execution stopped (max iterations reached)\n")
	}
	if !summary.MaxReached {
		b.WriteString(fmt.Sprintf("✓ Execution complete - %d step(s)\n", summary.TotalSteps))
	}

	// Step details (show PURPOSE, not tool names)
	for _, step := range summary.Steps {
		durationStr := formatDuration(step.Duration)

		if step.Success {
			b.WriteString(fmt.Sprintf("╰─ Step %d: %s (%s)\n",
				step.StepNumber,
				step.Purpose,
				durationStr,
			))
		}
		if !step.Success {
			b.WriteString(fmt.Sprintf("╰─ Step %d: %s (failed: %s)\n",
				step.StepNumber,
				step.Purpose,
				step.ErrorMsg,
			))
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// formatDuration formats duration for display
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	seconds := float64(d.Milliseconds()) / 1000.0
	return fmt.Sprintf("%.1fs", seconds)
}
