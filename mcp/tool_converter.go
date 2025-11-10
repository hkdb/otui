package mcp

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
	"github.com/openai/openai-go/v3"
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

// ConvertMCPToolsToOpenAIFormat converts MCP tools to OpenAI/OpenRouter format.
// This format is shared between OpenAI and OpenRouter since they use the same API.
//
// MCP Tool structure:
//
//	{
//	  "name": "get_weather",
//	  "description": "Get weather data",
//	  "inputSchema": {
//	    "type": "object",
//	    "properties": {...},
//	    "required": [...]
//	  }
//	}
//
// OpenAI Tool structure:
//
//	{
//	  "type": "function",
//	  "function": {
//	    "name": "get_weather",
//	    "description": "Get weather data",
//	    "parameters": {...}
//	  }
//	}
func ConvertMCPToolsToOpenAIFormat(mcpTools []mcptypes.Tool) []openai.ChatCompletionToolUnionParam {
	if len(mcpTools) == 0 {
		return nil
	}

	result := make([]openai.ChatCompletionToolUnionParam, len(mcpTools))

	for i, tool := range mcpTools {
		// Convert MCP InputSchema to OpenAI FunctionParameters
		// Both are JSON Schema format, just need to convert the struct to map
		params := openai.FunctionParameters{
			"type":       tool.InputSchema.Type,
			"properties": tool.InputSchema.Properties,
		}

		if len(tool.InputSchema.Required) > 0 {
			params["required"] = tool.InputSchema.Required
		}

		if tool.InputSchema.Defs != nil {
			params["$defs"] = tool.InputSchema.Defs
		}

		// Build the function tool
		result[i] = openai.ChatCompletionFunctionTool(
			openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters:  params,
			},
		)
	}

	return result
}

// ConvertMCPToolsToAnthropicFormat converts MCP tools to Anthropic format.
// This format is used by Anthropic's Claude API for tool use.
//
// MCP Tool structure:
//
//	{
//	  "name": "get_weather",
//	  "description": "Get weather data",
//	  "inputSchema": {
//	    "type": "object",
//	    "properties": {...},
//	    "required": [...]
//	  }
//	}
//
// Anthropic Tool structure uses ToolUnionParam with input_schema
func ConvertMCPToolsToAnthropicFormat(mcpTools []mcptypes.Tool) []anthropic.ToolUnionParam {
	if len(mcpTools) == 0 {
		return nil
	}

	result := make([]anthropic.ToolUnionParam, len(mcpTools))

	for i, tool := range mcpTools {
		// Convert InputSchema to Anthropic's ToolInputSchemaParam
		inputSchema := anthropic.ToolInputSchemaParam{
			// Type defaults to "object" when omitted
			Properties: tool.InputSchema.Properties,
		}

		if len(tool.InputSchema.Required) > 0 {
			inputSchema.Required = tool.InputSchema.Required
		}

		if tool.InputSchema.Defs != nil {
			// Use ExtraFields for $defs
			inputSchema.ExtraFields = map[string]any{
				"$defs": tool.InputSchema.Defs,
			}
		}

		// Create ToolUnionParam using the helper function
		result[i] = anthropic.ToolUnionParamOfTool(inputSchema, tool.Name)

		// Set description if provided
		if tool.Description != "" {
			result[i].OfTool.Description = anthropic.String(tool.Description)
		}
	}

	return result
}
