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

		if msg.Role == "system" && msg.Content == "Waiting for response..." {
			renderedContent = fmt.Sprintf("%s %s", a.loadingSpinner.View(), msg.Content)
		}

		if msg.Role == "user" {
			formattedUser := formatUserMessage(highlightPrefix, timestamp, role, renderedContent)
			content.WriteString(formattedUser)
		} else {
			content.WriteString(fmt.Sprintf("%s%s %s\n%s\n\n", highlightPrefix, timestamp, role, renderedContent))
		}
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
