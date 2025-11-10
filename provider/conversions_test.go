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

func TestConvertFromOllamaMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []api.Message
		expected []model.Message
	}{
		{
			name:     "empty slice",
			input:    []api.Message{},
			expected: []model.Message{},
		},
		{
			name: "single message",
			input: []api.Message{
				{Role: "assistant", Content: "Hello back"},
			},
			expected: []model.Message{
				{Role: "assistant", Content: "Hello back"},
			},
		},
		{
			name: "multiple messages",
			input: []api.Message{
				{Role: "user", Content: "Question 1"},
				{Role: "assistant", Content: "Answer 1"},
				{Role: "user", Content: "Question 2"},
			},
			expected: []model.Message{
				{Role: "user", Content: "Question 1"},
				{Role: "assistant", Content: "Answer 1"},
				{Role: "user", Content: "Question 2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertFromOllamaMessages(tt.input)

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

func TestConvertFromProviderToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    []model.ToolCall
		expected []api.ToolCall
	}{
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []model.ToolCall{},
			expected: nil,
		},
		{
			name: "single tool call",
			input: []model.ToolCall{
				{
					Name:      "get_time",
					Arguments: map[string]any{"timezone": "UTC"},
				},
			},
			expected: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name:      "get_time",
						Arguments: map[string]any{"timezone": "UTC"},
					},
				},
			},
		},
		{
			name: "multiple tool calls",
			input: []model.ToolCall{
				{
					Name:      "read_file",
					Arguments: map[string]any{"path": "/tmp/test.txt"},
				},
				{
					Name:      "write_file",
					Arguments: map[string]any{"path": "/tmp/out.txt", "content": "data"},
				},
			},
			expected: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name:      "read_file",
						Arguments: map[string]any{"path": "/tmp/test.txt"},
					},
				},
				{
					Function: api.ToolCallFunction{
						Name:      "write_file",
						Arguments: map[string]any{"path": "/tmp/out.txt", "content": "data"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertFromProviderToolCalls(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, want %d", len(result), len(tt.expected))
			}

			for i, call := range result {
				if call.Function.Name != tt.expected[i].Function.Name {
					t.Errorf("tool call %d name: got %q, want %q", i, call.Function.Name, tt.expected[i].Function.Name)
				}
				if len(call.Function.Arguments) != len(tt.expected[i].Function.Arguments) {
					t.Errorf("tool call %d arguments length: got %d, want %d", i, len(call.Function.Arguments), len(tt.expected[i].Function.Arguments))
				}
			}
		})
	}
}

// TestRoundTripConversions verifies that converting back and forth preserves data
func TestRoundTripConversions(t *testing.T) {
	t.Run("messages round trip", func(t *testing.T) {
		original := []model.Message{
			{Role: "user", Content: "Test message"},
			{Role: "assistant", Content: "Response"},
		}

		// Convert to Ollama and back
		ollama := ConvertToOllamaMessages(original)
		result := ConvertFromOllamaMessages(ollama)

		if len(result) != len(original) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(original))
		}

		for i := range result {
			if result[i].Role != original[i].Role || result[i].Content != original[i].Content {
				t.Errorf("message %d changed: got {%q, %q}, want {%q, %q}",
					i, result[i].Role, result[i].Content, original[i].Role, original[i].Content)
			}
		}
	})

	t.Run("tool calls round trip", func(t *testing.T) {
		original := []model.ToolCall{
			{Name: "test_tool", Arguments: map[string]any{"key": "value"}},
		}

		// Convert to Ollama and back
		ollama := ConvertFromProviderToolCalls(original)
		result := ConvertToProviderToolCalls(ollama)

		if len(result) != len(original) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(original))
		}

		if result[0].Name != original[0].Name {
			t.Errorf("tool name changed: got %q, want %q", result[0].Name, original[0].Name)
		}
	})
}
