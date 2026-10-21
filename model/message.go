package model

import (
	"time"
	"unicode/utf8"
)

// Message represents a chat message in the conversation
type Message struct {
	Role       string
	Content    string // Raw content from Ollama
	Rendered   string // Cached rendered markdown (optimize if storage becomes a concern)
	Timestamp  time.Time
	Persistent bool // If true, don't auto-remove (e.g., step messages)

	// Tool calling (only used in API requests, never saved in sessions)
	ToolCalls  []ToolCall // Tools an assistant turn called
	ToolCallID string     // For role "tool": the call this result answers
	ToolName   string     // For role "tool": the tool that produced it
}

// ToolCall represents a provider-agnostic tool call request.
// This allows us to abstract away provider-specific tool call formats
// (Ollama's api.ToolCall, OpenAI's function calls, etc.).
type ToolCall struct {
	ID        string // Provider's call ID (generated if the provider has none)
	Name      string
	Arguments map[string]any
}

// truncateRunes shortens s to at most n characters, appending "..." if it was
// cut. Unlike byte slicing it never splits a multi-byte character.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "..."
}
