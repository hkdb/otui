package model

import "time"

// Message represents a chat message in the conversation
type Message struct {
	Role      string
	Content   string // Raw content from Ollama
	Rendered  string // Cached rendered markdown (optimize if storage becomes a concern)
	Timestamp time.Time
}
