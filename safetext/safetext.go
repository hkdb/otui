// Package safetext removes terminal control sequences from untrusted text
// (model output, tool arguments, session files, registry data) before it's
// shown, so it can't move the cursor, erase or overwrite the screen, write
// the clipboard (OSC 52) or reorder text (bidi overrides).
package safetext

import (
	"strings"
	"unicode/utf8"
)

const esc = 0x1b

// Clean removes control characters from multi-line text: ESC and C0
// controls except newline and tab (carriage returns are dropped), DEL, C1
// controls and bidi overrides. Invalid UTF-8 becomes U+FFFD.
//
// Removing ESC alone neutralizes escape sequences: what's left (e.g. "[2K")
// is plain text, also when a sequence is split across calls.
func Clean(s string) string {
	if isClean(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToValidUTF8(s, "�") {
		if keep(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Line is Clean for single-line fields: newlines and tabs become spaces
func Line(s string) string {
	s = Clean(s)
	if !strings.ContainsAny(s, "\n\t") {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, s)
}

// SGROnly is Clean for text that may contain OTUI's own colors (rendered
// markdown, styled system messages): SGR sequences (ESC [ params m) are kept,
// every other escape sequence (cursor movement, erase, OSC, DCS...) is
// removed with its parameters.
func SGROnly(s string) string {
	if isClean(s) {
		return s
	}
	s = strings.ToValidUTF8(s, "�")

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == esc {
			n, sgr := escapeSequence(s[i:])
			if sgr {
				b.WriteString(s[i : i+n])
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if keep(r) {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// escapeSequence returns the length of the escape sequence at the start of s
// (which starts with ESC) and whether it's an SGR (color) sequence
func escapeSequence(s string) (int, bool) {
	if len(s) < 2 {
		return len(s), false
	}
	switch s[1] {
	case '[': // CSI: parameters, intermediates, final byte 0x40-0x7e
		i := 2
		for i < len(s) && s[i] >= 0x30 && s[i] <= 0x3f {
			i++
		}
		params := s[2:i]
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i >= len(s) {
			return len(s), false
		}
		if s[i] < 0x40 || s[i] > 0x7e {
			return i, false // Malformed: drop what was parsed
		}
		sgr := s[i] == 'm' && strings.Trim(params, "0123456789;:") == ""
		return i + 1, sgr
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: until BEL or ST
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1, false
			}
			if s[i] == esc && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2, false
			}
		}
		return len(s), false
	default: // Two-byte sequence (ESC 7, ESC c...)
		_, size := utf8.DecodeRuneInString(s[1:])
		return 1 + size, false
	}
}

// keep reports whether r is allowed in cleaned text
func keep(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return true
	case r < 0x20 || r == 0x7f: // C0 (includes ESC and CR), DEL
		return false
	case r >= 0x80 && r <= 0x9f: // C1 (e.g. U+009B is CSI)
		return false
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069: // Bidi overrides/isolates
		return false
	}
	return true
}

// isClean reports whether s needs no changes (fast path for plain text)
func isClean(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\n' && c != '\t' || c == 0x7f || c >= 0x80 {
			// Non-ASCII needs a closer look (C1, bidi, invalid UTF-8)
			if c >= 0x80 {
				return !strings.ContainsFunc(s[i:], func(r rune) bool { return !keep(r) || r == utf8.RuneError }) &&
					utf8.ValidString(s[i:])
			}
			return false
		}
	}
	return true
}
