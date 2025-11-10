package testutil

import (
	"otui/model"
	"time"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// TestMessages returns a sample conversation for testing
func TestMessages() []model.Message {
	return []model.Message{
		{
			Role:      "user",
			Content:   "Hello, how are you?",
			Timestamp: time.Now(),
		},
		{
			Role:      "assistant",
			Content:   "I'm doing well, thank you!",
			Timestamp: time.Now(),
		},
		{
			Role:      "user",
			Content:   "Can you help me with a task?",
			Timestamp: time.Now(),
		},
	}
}

// SingleUserMessage returns a single user message for simple tests
func SingleUserMessage(content string) []model.Message {
	return []model.Message{
		{
			Role:      "user",
			Content:   content,
			Timestamp: time.Now(),
		},
	}
}

// TestMCPTools returns sample MCP tools for testing
func TestMCPTools() []mcptypes.Tool {
	return []mcptypes.Tool{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "The city and state, e.g. San Francisco, CA",
					},
				},
				Required: []string{"location"},
			},
		},
		{
			Name:        "calculate",
			Description: "Perform a mathematical calculation",
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"expression": map[string]any{
						"type":        "string",
						"description": "The mathematical expression to evaluate",
					},
				},
				Required: []string{"expression"},
			},
		},
	}
}

// EmptyMessages returns an empty message slice for edge case testing
func EmptyMessages() []model.Message {
	return []model.Message{}
}

// SystemMessage returns a system message for testing
func SystemMessage(content string) model.Message {
	return model.Message{
		Role:      "system",
		Content:   content,
		Timestamp: time.Now(),
	}
}
