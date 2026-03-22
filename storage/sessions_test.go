package storage

import (
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"slashes", "path/to\\file", "path-to-file"},
		{"special_chars", "file:name*with?bad\"chars", "file-name-with-bad-chars"},
		{"angle_brackets", "<input>|output", "input--output"},
		{"spaces", "hello world", "hello-world"},
		{"newlines", "line1\nline2\rline3", "line1-line2-line3"},
		{"leading_dots", "...hidden", "hidden"},
		{"trailing_dots", "file...", "file"},
		{"leading_hyphens", "---name", "name"},
		{"empty_after_sanitize", "...", "session"},
		{"long_name", strings.Repeat("a", 100), strings.Repeat("a", 50)},
		{"normal", "my-session-name", "my-session-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateSessionName(t *testing.T) {
	t.Run("EmptyInput", func(t *testing.T) {
		name := GenerateSessionName("")
		if !strings.HasPrefix(name, "Session ") {
			t.Errorf("expected fallback name starting with 'Session ', got %q", name)
		}
	})

	t.Run("ShortInput", func(t *testing.T) {
		name := GenerateSessionName("Hello world")
		if name != "Hello world" {
			t.Errorf("expected 'Hello world', got %q", name)
		}
	})

	t.Run("LongInput", func(t *testing.T) {
		long := strings.Repeat("x", 50)
		name := GenerateSessionName(long)
		if len(name) != 33 { // 30 + "..."
			t.Errorf("expected length 33, got %d (%q)", len(name), name)
		}
		if !strings.HasSuffix(name, "...") {
			t.Errorf("expected '...' suffix, got %q", name)
		}
	})

	t.Run("NewlinesReplaced", func(t *testing.T) {
		name := GenerateSessionName("line1\nline2")
		if strings.Contains(name, "\n") {
			t.Errorf("expected newlines to be removed, got %q", name)
		}
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		name := GenerateSessionName("   \n  ")
		if !strings.HasPrefix(name, "Session ") {
			t.Errorf("expected fallback name for whitespace-only input, got %q", name)
		}
	})
}

func TestGenerateExportPath(t *testing.T) {
	path := GenerateExportPath("My Session")
	if !strings.Contains(path, "otui-session-") {
		t.Errorf("expected 'otui-session-' in path, got %q", path)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("expected '.json' suffix, got %q", path)
	}
	if !strings.Contains(path, "My-Session") {
		t.Errorf("expected sanitized name 'My-Session' in path, got %q", path)
	}
}

func TestSearchMessages(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helper"},
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there, world!"},
		{Role: "user", Content: "Goodbye"},
	}

	t.Run("EmptyQuery", func(t *testing.T) {
		matches := SearchMessages(messages, "")
		if len(matches) != 0 {
			t.Errorf("expected 0 matches for empty query, got %d", len(matches))
		}
	})

	t.Run("MatchesFound", func(t *testing.T) {
		matches := SearchMessages(messages, "world")
		if len(matches) != 2 {
			t.Errorf("expected 2 matches for 'world', got %d", len(matches))
		}
	})

	t.Run("SkipsSystem", func(t *testing.T) {
		matches := SearchMessages(messages, "helper")
		if len(matches) != 0 {
			t.Errorf("expected 0 matches (system skipped), got %d", len(matches))
		}
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		matches := SearchMessages(messages, "HELLO")
		if len(matches) != 1 {
			t.Errorf("expected 1 case-insensitive match, got %d", len(matches))
		}
	})
}

func TestSessionPlugins(t *testing.T) {
	s := &Session{}

	t.Run("EnablePlugin", func(t *testing.T) {
		s.EnablePlugin("mcp-fs")
		if !s.IsPluginEnabled("mcp-fs") {
			t.Error("expected plugin to be enabled")
		}
	})

	t.Run("EnableDuplicate", func(t *testing.T) {
		s.EnablePlugin("mcp-fs")
		if len(s.EnabledPlugins) != 1 {
			t.Errorf("expected 1 plugin after duplicate enable, got %d", len(s.EnabledPlugins))
		}
	})

	t.Run("DisablePlugin", func(t *testing.T) {
		s.DisablePlugin("mcp-fs")
		if s.IsPluginEnabled("mcp-fs") {
			t.Error("expected plugin to be disabled")
		}
	})

	t.Run("IsPluginEnabled_nil", func(t *testing.T) {
		empty := &Session{}
		if empty.IsPluginEnabled("anything") {
			t.Error("expected false for nil EnabledPlugins")
		}
	})

	t.Run("GetEnabledPlugins_nil", func(t *testing.T) {
		empty := &Session{}
		plugins := empty.GetEnabledPlugins()
		if plugins == nil {
			t.Error("expected non-nil slice")
		}
		if len(plugins) != 0 {
			t.Error("expected empty slice")
		}
	})
}
