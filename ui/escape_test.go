package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPermissionPromptCantBeSpoofed(t *testing.T) {
	content := buildPermissionContent("fs.write\x1b[2K", "Save\nthe file\r",
		map[string]string{"path": "a\n╰── Tool: fs.read\r\x1b[2K"})

	if strings.ContainsAny(content, "\r") || strings.Contains(content, "\x1b[2K") {
		t.Errorf("control characters in prompt: %q", content)
	}
	// The argument stays on its own (quoted) line: no extra "Tool:" line
	n := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "╰── Tool:") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("prompt has %d Tool lines, want 1:\n%s", n, content)
	}
	if !strings.Contains(content, `"a\n╰── Tool: fs.read[2K"`) {
		t.Errorf("argument not shown quoted on one line:\n%s", content)
	}
}

func TestViewKeepsOnlyColors(t *testing.T) {
	// Every screen's View passes its output through the same filter
	m, _ := NewErrorModal("Error", "server said: \x1b]52;c;aGk=\x07\x1b[2Jboom").Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := m.View()
	if strings.Contains(out, "\x1b]52") || strings.Contains(out, "\x1b[2J") {
		t.Errorf("View passed control sequences to the terminal: %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Error("message text missing")
	}
}
