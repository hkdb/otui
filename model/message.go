package model

import "time"

// Message represents a chat message in the conversation
type Message struct {
	Role       string
	Content    string // Raw content from Ollama
	Rendered   string // Cached rendered markdown (optimize if storage becomes a concern)
	Timestamp  time.Time
	Persistent bool // If true, don't auto-remove (e.g., step messages)
}

// ToolCall represents a provider-agnostic tool call request.
// This allows us to abstract away provider-specific tool call formats
// (Ollama's api.ToolCall, OpenAI's function calls, etc.).
type ToolCall struct {
	Name      string
	Arguments map[string]any
}
