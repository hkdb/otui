package mcp

import (
	"encoding/json"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
)

// ConvertMCPToolsToOllama converts MCP tools to Ollama API tool format
func ConvertMCPToolsToOllama(mcpTools []mcptypes.Tool) []api.Tool {
	ollamaTools := make([]api.Tool, 0, len(mcpTools))

	for _, mcpTool := range mcpTools {
		ollamaTool := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				Parameters:  convertInputSchemaToParameters(mcpTool.InputSchema),
			},
		}
		ollamaTools = append(ollamaTools, ollamaTool)
	}

	return ollamaTools
}

// convertInputSchemaToParameters converts MCP InputSchema to Ollama ToolFunctionParameters
func convertInputSchemaToParameters(inputSchema mcptypes.ToolInputSchema) api.ToolFunctionParameters {
	params := api.ToolFunctionParameters{
		Type:       inputSchema.Type,
		Required:   inputSchema.Required,
		Properties: make(map[string]api.ToolProperty),
	}

	// Handle $defs if present
	if inputSchema.Defs != nil {
		params.Defs = inputSchema.Defs
	}

	// Convert properties from map[string]any to map[string]api.ToolProperty
	for propName, propValue := range inputSchema.Properties {
		params.Properties[propName] = convertPropertyValue(propValue)
	}

	return params
}

// convertPropertyValue converts a property value from MCP format to Ollama ToolProperty
func convertPropertyValue(propValue any) api.ToolProperty {
	toolProp := api.ToolProperty{}

	// Handle the property as a map
	propMap, ok := propValue.(map[string]any)
	if !ok {
		// If it's not a map, try to marshal and unmarshal it
		bytes, err := json.Marshal(propValue)
		if err != nil {
			return toolProp
		}
		var m map[string]any
		if err := json.Unmarshal(bytes, &m); err != nil {
			return toolProp
		}
		propMap = m
	}

	// Extract type (can be string or []string)
	if typeVal, ok := propMap["type"]; ok {
		switch t := typeVal.(type) {
		case string:
			toolProp.Type = api.PropertyType{t}
		case []string:
			toolProp.Type = api.PropertyType(t)
		case []any:
			// Convert []any to []string
			types := make([]string, 0, len(t))
			for _, v := range t {
				if s, ok := v.(string); ok {
					types = append(types, s)
				}
			}
			toolProp.Type = api.PropertyType(types)
		}
	}

	// Extract description
	if desc, ok := propMap["description"].(string); ok {
		toolProp.Description = desc
	}

	// Extract enum
	if enumVal, ok := propMap["enum"]; ok {
		if enumSlice, ok := enumVal.([]any); ok {
			toolProp.Enum = enumSlice
		}
	}

	// Extract items (for array types)
	if items, ok := propMap["items"]; ok {
		toolProp.Items = items
	}

	// Extract anyOf (for union types)
	if anyOfVal, ok := propMap["anyOf"]; ok {
		if anyOfSlice, ok := anyOfVal.([]any); ok {
			anyOfProps := make([]api.ToolProperty, 0, len(anyOfSlice))
			for _, item := range anyOfSlice {
				anyOfProps = append(anyOfProps, convertPropertyValue(item))
			}
			toolProp.AnyOf = anyOfProps
		}
	}

	return toolProp
}

// ConvertOllamaToolCallToMCP converts an Ollama tool call to MCP CallToolRequest format
func ConvertOllamaToolCallToMCP(toolCall api.ToolCall) (string, map[string]any, error) {
	toolName := toolCall.Function.Name

	// Arguments is already a map[string]any in Ollama API
	args := map[string]any(toolCall.Function.Arguments)

	return toolName, args, nil
}
