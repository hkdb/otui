package safetext

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"plain":            {"Hello, world!\n\tindented", "Hello, world!\n\tindented"},
		"erase line + CR":  {"approve\r\x1b[2Kdeny", "approve[2Kdeny"},
		"cursor moves":     {"a\x1b[3A\x1b[10;5Hb", "a[3A[10;5Hb"},
		"OSC 52 BEL":       {"x\x1b]52;c;aGk=\x07y", "x]52;c;aGk=y"},
		"OSC 52 ST":        {"x\x1b]52;c;aGk=\x1b\\y", "x]52;c;aGk=\\y"},
		"DCS":              {"\x1bPq#0\x1b\\done", "Pq#0\\done"},
		"C1 CSI":           {"a\u009b2Jb", "a2Jb"},
		"DEL and BEL":      {"a\x7f\x07b", "ab"},
		"bidi override":    {"user\u202egnp.exe", "usergnp.exe"},
		"bidi isolate":     {"a\u2066b\u2069c", "abc"},
		"SGR removed":      {"\x1b[31mred\x1b[0m", "[31mred[0m"},
		"CRLF":             {"line1\r\nline2", "line1\nline2"},
		"emoji and CJK":    {"👨‍👩‍👧 ❤️ 日本語 ✓", "👨‍👩‍👧 ❤️ 日本語 ✓"},
		"invalid UTF-8":    {"a\xffb", "a�b"},
		"replacement char": {"a�b", "a�b"},
	}
	for name, tt := range tests {
		if got := Clean(tt.in); got != tt.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", name, tt.in, got, tt.want)
		}
	}
}

func TestCleanSplitSequence(t *testing.T) {
	// A sequence split across stream chunks stays harmless
	got := Clean("ok\x1b") + Clean("]52;c;aGk=\x07")
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("split sequence left control characters: %q", got)
	}
}

func TestLine(t *testing.T) {
	if got := Line("one\ntwo\tthree\r\x1b[2K"); got != "one two three[2K" {
		t.Errorf("Line = %q", got)
	}
}

func TestSGROnly(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"keeps colors":     {"\x1b[1;32mok\x1b[0m \x1b[38;5;196mx\x1b[m", "\x1b[1;32mok\x1b[0m \x1b[38;5;196mx\x1b[m"},
		"drops cursor":     {"a\x1b[2Kb\x1b[1;1Hc", "abc"},
		"drops clear":      {"\x1b[2J\x1b[31mred", "\x1b[31mred"},
		"drops private m":  {"\x1b[?25mx", "x"},
		"drops OSC 52":     {"a\x1b]52;c;aGk=\x07b", "ab"},
		"drops OSC 8 ST":   {"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "link"},
		"drops DCS":        {"\x1bPq#0\x1b\\done", "done"},
		"drops two-byte":   {"a\x1b7b\x1bcc", "abc"},
		"drops C1 and CR":  {"a\u009b2J\rb", "a2Jb"},
		"unterminated OSC": {"a\x1b]52;c;aGk=", "a"},
		"trailing ESC":     {"a\x1b", "a"},
	}
	for name, tt := range tests {
		if got := SGROnly(tt.in); got != tt.want {
			t.Errorf("%s: SGROnly(%q) = %q, want %q", name, tt.in, got, tt.want)
		}
	}
}
