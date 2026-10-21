package provider

import (
	"otui/model"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
)

func TestConvertToOllamaMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []model.Message
		expected []api.Message
	}{
		{
			name:     "empty slice",
			input:    []model.Message{},
			expected: []api.Message{},
		},
		{
			name: "single message",
			input: []model.Message{
				{Role: "user", Content: "Hello"},
			},
			expected: []api.Message{
				{Role: "user", Content: "Hello"},
			},
		},
		{
			name: "multiple messages",
			input: []model.Message{
				{Role: "user", Content: "Hello", Timestamp: time.Now()},
				{Role: "assistant", Content: "Hi there", Timestamp: time.Now()},
				{Role: "user", Content: "How are you?", Timestamp: time.Now()},
			},
			expected: []api.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
				{Role: "user", Content: "How are you?"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToOllamaMessages(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, want %d", len(result), len(tt.expected))
			}

			for i, msg := range result {
				if msg.Role != tt.expected[i].Role {
					t.Errorf("message %d role: got %q, want %q", i, msg.Role, tt.expected[i].Role)
				}
				if msg.Content != tt.expected[i].Content {
					t.Errorf("message %d content: got %q, want %q", i, msg.Content, tt.expected[i].Content)
				}
			}
		})
	}
}

func TestConvertToProviderToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    []api.ToolCall
		expected []model.ToolCall
	}{
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []api.ToolCall{},
			expected: nil,
		},
		{
			name: "single tool call",
			input: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name:      "get_weather",
						Arguments: map[string]any{"city": "San Francisco"},
					},
				},
			},
			expected: []model.ToolCall{
				{
					Name:      "get_weather",
					Arguments: map[string]any{"city": "San Francisco"},
				},
			},
		},
		{
			name: "multiple tool calls",
			input: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name:      "search",
						Arguments: map[string]any{"query": "golang"},
					},
				},
				{
					Function: api.ToolCallFunction{
						Name:      "calculate",
						Arguments: map[string]any{"expr": "2+2"},
					},
				},
			},
			expected: []model.ToolCall{
				{
					Name:      "search",
					Arguments: map[string]any{"query": "golang"},
				},
				{
					Name:      "calculate",
					Arguments: map[string]any{"expr": "2+2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToProviderToolCalls(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, want %d", len(result), len(tt.expected))
			}

			for i, call := range result {
				if call.Name != tt.expected[i].Name {
					t.Errorf("tool call %d name: got %q, want %q", i, call.Name, tt.expected[i].Name)
				}
				// Simple check for arguments (deep comparison would require reflection)
				if len(call.Arguments) != len(tt.expected[i].Arguments) {
					t.Errorf("tool call %d arguments length: got %d, want %d", i, len(call.Arguments), len(tt.expected[i].Arguments))
				}
			}
		})
	}
}
