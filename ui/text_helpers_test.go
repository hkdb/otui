package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func TestSafeRepeat(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{-5, ""},
		{0, ""},
		{3, "───"},
	}
	for _, tt := range tests {
		if got := safeRepeat("─", tt.count); got != tt.want {
			t.Errorf("safeRepeat(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}

func TestTruncateWidth(t *testing.T) {
	inputs := []string{
		"plain ascii session name",
		"Très long nom de session",
		"日本語のセッション名です",
		"🚀🚀🚀 rocket session",
	}
	for _, s := range inputs {
		for maxWidth := -2; maxWidth <= 12; maxWidth++ {
			got := truncateWidth(s, maxWidth)
			if !utf8.ValidString(got) {
				t.Errorf("truncateWidth(%q, %d) = %q splits a character", s, maxWidth, got)
			}
			if w := runewidth.StringWidth(got); w > max(maxWidth, 0) {
				t.Errorf("truncateWidth(%q, %d) = %q has width %d", s, maxWidth, got, w)
			}
		}
	}

	if got := truncateWidth("short", 10); got != "short" {
		t.Errorf("fitting string changed: %q", got)
	}
	if got := truncateWidth("abcdefghij", 6); got != "abc..." {
		t.Errorf("truncateWidth = %q, want %q", got, "abc...")
	}
}

func TestTerminalTooSmall(t *testing.T) {
	if !terminalTooSmall(39, 30) || !terminalTooSmall(100, 11) {
		t.Error("expected too small below minimum")
	}
	if terminalTooSmall(40, 12) {
		t.Error("40x12 should be big enough")
	}
}
