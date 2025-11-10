package mcp

import (
	"testing"

	"github.com/ollama/ollama/api"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

func TestConvertMCPToolsToOllama(t *testing.T) {
	tests := []struct {
		name     string
		input    []mcptypes.Tool
		expected int // expected tool count
		validate func(t *testing.T, result []api.Tool)
	}{
		{
			name:     "empty tools",
			input:    []mcptypes.Tool{},
			expected: 0,
			validate: func(t *testing.T, result []api.Tool) {
				if len(result) != 0 {
					t.Errorf("expected empty slice, got %d tools", len(result))
				}
			},
		},
		{
			name: "single simple tool",
			input: []mcptypes.Tool{
				{
					Name:        "get_weather",
					Description: "Get current weather",
					InputSchema: mcptypes.ToolInputSchema{
						Type:       "object",
						Properties: map[string]any{},
						Required:   []string{},
					},
				},
			},
			expected: 1,
			validate: func(t *testing.T, result []api.Tool) {
				if result[0].Type != "function" {
					t.Errorf("expected type 'function', got %q", result[0].Type)
				}
				if result[0].Function.Name != "get_weather" {
					t.Errorf("expected name 'get_weather', got %q", result[0].Function.Name)
				}
				if result[0].Function.Description != "Get current weather" {
					t.Errorf("expected description 'Get current weather', got %q", result[0].Function.Description)
				}
			},
		},
		{
			name: "tool with properties",
			input: []mcptypes.Tool{
				{
					Name:        "calculate",
					Description: "Perform calculation",
					InputSchema: mcptypes.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"operation": map[string]any{
								"type":        "string",
								"description": "The operation to perform",
								"enum":        []any{"add", "subtract", "multiply", "divide"},
							},
							"a": map[string]any{
								"type":        "number",
								"description": "First operand",
							},
							"b": map[string]any{
								"type":        "number",
								"description": "Second operand",
							},
						},
						Required: []string{"operation", "a", "b"},
					},
				},
			},
			expected: 1,
			validate: func(t *testing.T, result []api.Tool) {
				params := result[0].Function.Parameters
				if params.Type != "object" {
					t.Errorf("expected type 'object', got %q", params.Type)
				}
				if len(params.Required) != 3 {
					t.Errorf("expected 3 required fields, got %d", len(params.Required))
				}
				if len(params.Properties) != 3 {
					t.Errorf("expected 3 properties, got %d", len(params.Properties))
				}

				// Check operation property
				opProp, ok := params.Properties["operation"]
				if !ok {
					t.Fatal("operation property not found")
				}
				if opProp.Description != "The operation to perform" {
					t.Errorf("operation description mismatch")
				}
				if len(opProp.Enum) != 4 {
					t.Errorf("expected 4 enum values, got %d", len(opProp.Enum))
				}
			},
		},
		{
			name: "multiple tools",
			input: []mcptypes.Tool{
				{
					Name:        "tool1",
					Description: "First tool",
					InputSchema: mcptypes.ToolInputSchema{
						Type:       "object",
						Properties: map[string]any{},
						Required:   []string{},
					},
				},
				{
					Name:        "tool2",
					Description: "Second tool",
					InputSchema: mcptypes.ToolInputSchema{
						Type:       "object",
						Properties: map[string]any{},
						Required:   []string{},
					},
				},
			},
			expected: 2,
			validate: func(t *testing.T, result []api.Tool) {
				if result[0].Function.Name != "tool1" {
					t.Errorf("first tool name mismatch")
				}
				if result[1].Function.Name != "tool2" {
					t.Errorf("second tool name mismatch")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertMCPToolsToOllama(tt.input)

			if len(result) != tt.expected {
				t.Fatalf("expected %d tools, got %d", tt.expected, len(result))
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestConvertPropertyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		validate func(t *testing.T, result api.ToolProperty)
	}{
		{
			name: "string type",
			input: map[string]any{
				"type":        "string",
				"description": "A string property",
			},
			validate: func(t *testing.T, result api.ToolProperty) {
				if len(result.Type) != 1 || result.Type[0] != "string" {
					t.Errorf("expected type [string], got %v", result.Type)
				}
				if result.Description != "A string property" {
					t.Errorf("description mismatch")
				}
			},
		},
		{
			name: "array type property",
			input: map[string]any{
				"type":        []any{"string", "number"},
				"description": "Multi-type property",
			},
			validate: func(t *testing.T, result api.ToolProperty) {
				if len(result.Type) != 2 {
					t.Errorf("expected 2 types, got %d", len(result.Type))
				}
				if result.Description != "Multi-type property" {
					t.Errorf("description mismatch")
				}
			},
		},
		{
			name: "property with enum",
			input: map[string]any{
				"type": "string",
				"enum": []any{"option1", "option2", "option3"},
			},
			validate: func(t *testing.T, result api.ToolProperty) {
				if len(result.Enum) != 3 {
					t.Errorf("expected 3 enum values, got %d", len(result.Enum))
				}
			},
		},
		{
			name: "array property with items",
			input: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			validate: func(t *testing.T, result api.ToolProperty) {
				if result.Items == nil {
					t.Error("expected items to be set")
				}
			},
		},
		{
			name: "property with anyOf",
			input: map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "number"},
				},
			},
			validate: func(t *testing.T, result api.ToolProperty) {
				if len(result.AnyOf) != 2 {
					t.Errorf("expected 2 anyOf options, got %d", len(result.AnyOf))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPropertyValue(tt.input)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestConvertOllamaToolCallToMCP(t *testing.T) {
	tests := []struct {
		name         string
		input        api.ToolCall
		expectedName string
		expectedArgs map[string]any
		expectError  bool
	}{
		{
			name: "simple tool call",
			input: api.ToolCall{
				Function: api.ToolCallFunction{
					Name: "get_weather",
					Arguments: map[string]any{
						"city": "San Francisco",
					},
				},
			},
			expectedName: "get_weather",
			expectedArgs: map[string]any{
				"city": "San Francisco",
			},
			expectError: false,
		},
		{
			name: "tool call with multiple arguments",
			input: api.ToolCall{
				Function: api.ToolCallFunction{
					Name: "calculate",
					Arguments: map[string]any{
						"operation": "add",
						"a":         float64(5),
						"b":         float64(3),
					},
				},
			},
			expectedName: "calculate",
			expectedArgs: map[string]any{
				"operation": "add",
				"a":         float64(5),
				"b":         float64(3),
			},
			expectError: false,
		},
		{
			name: "tool call with empty arguments",
			input: api.ToolCall{
				Function: api.ToolCallFunction{
					Name:      "ping",
					Arguments: map[string]any{},
				},
			},
			expectedName: "ping",
			expectedArgs: map[string]any{},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := ConvertOllamaToolCallToMCP(tt.input)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if name != tt.expectedName {
				t.Errorf("expected name %q, got %q", tt.expectedName, name)
			}

			if len(args) != len(tt.expectedArgs) {
				t.Errorf("expected %d arguments, got %d", len(tt.expectedArgs), len(args))
			}

			for key, expectedVal := range tt.expectedArgs {
				actualVal, ok := args[key]
				if !ok {
					t.Errorf("missing argument %q", key)
					continue
				}
				// Simple value comparison (works for strings and numbers)
				if actualVal != expectedVal {
					t.Errorf("argument %q: expected %v, got %v", key, expectedVal, actualVal)
				}
			}
		})
	}
}

// TestComplexSchemaConversion tests a realistic complex tool schema
func TestComplexSchemaConversion(t *testing.T) {
	mcpTool := mcptypes.Tool{
		Name:        "search_files",
		Description: "Search for files in a directory",
		InputSchema: mcptypes.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path to search",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "File pattern to match",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "Search recursively",
				},
				"max_results": map[string]any{
					"type":        "number",
					"description": "Maximum number of results",
				},
			},
			Required: []string{"path", "pattern"},
		},
	}

	result := ConvertMCPToolsToOllama([]mcptypes.Tool{mcpTool})

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	tool := result[0]

	// Validate structure
	if tool.Type != "function" {
		t.Errorf("expected type 'function', got %q", tool.Type)
	}

	if tool.Function.Name != "search_files" {
		t.Errorf("name mismatch")
	}

	params := tool.Function.Parameters

	if params.Type != "object" {
		t.Errorf("parameters type mismatch")
	}

	if len(params.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(params.Required))
	}

	if len(params.Properties) != 4 {
		t.Errorf("expected 4 properties, got %d", len(params.Properties))
	}

	// Validate each property exists and has correct type
	pathProp, ok := params.Properties["path"]
	if !ok {
		t.Error("path property not found")
	}
	if len(pathProp.Type) != 1 || pathProp.Type[0] != "string" {
		t.Errorf("path type mismatch")
	}

	recursiveProp, ok := params.Properties["recursive"]
	if !ok {
		t.Error("recursive property not found")
	}
	if len(recursiveProp.Type) != 1 || recursiveProp.Type[0] != "boolean" {
		t.Errorf("recursive type mismatch")
	}
}
