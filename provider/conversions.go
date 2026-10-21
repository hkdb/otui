package provider

import (
	"encoding/json"
	"otui/config"
	"otui/mcp"
	"otui/model"
	"regexp"
	"strings"

	"github.com/ollama/ollama/api"
)

// ConvertToOllamaMessages converts OTUI model.Message to Ollama api.Message.
//
// This conversion is used when sending messages to the Ollama provider. It performs
// a simple field mapping since both types have compatible Role and Content fields.
//
// Note: The Timestamp and Rendered fields from model.Message are not preserved, as
// the Ollama API does not support these fields. Timestamps should be managed at the
// OTUI layer, not the provider layer.
//
// Example:
//
//	otuiMessages := []model.Message{
//	    {Role: "user", Content: "Hello"},
//	    {Role: "assistant", Content: "Hi there!"},
//	}
//	ollamaMessages := ConvertToOllamaMessages(otuiMessages)
//	// ollamaMessages now contains Ollama-compatible messages
//
// Tool calls of assistant turns and the tool name of tool results are passed
// natively (Ollama has no tool call IDs; results are matched by order/name).
func ConvertToOllamaMessages(messages []model.Message) []api.Message {
	result := make([]api.Message, len(messages))
	for i, msg := range messages {
		result[i] = api.Message{
			Role:     msg.Role,
			Content:  msg.Content,
			ToolName: msg.ToolName,
		}
		for j, call := range msg.ToolCalls {
			result[i].ToolCalls = append(result[i].ToolCalls, api.ToolCall{
				Function: api.ToolCallFunction{
					Index:     j,
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			})
		}
	}
	return result
}

// ParseToolArguments parses JSON arguments string into a map.
// Used by OpenAI and OpenRouter providers for tool call parsing.
func ParseToolArguments(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		// If parsing fails, return empty map
		return make(map[string]any)
	}
	return args
}

// ParseLeakedJSONToolCalls detects and parses tool calls leaked as JSON text in response content.
// Some models output tool calls as JSON text instead of using the proper API format.
// This function extracts those calls so they can be executed normally.
//
// Supported formats:
//   - Array: [{"name": "tool", "arguments": {...}}]
//   - Single object: {"name": "tool", "arguments": {...}}
//   - Variations: "param", "parameters", "input" instead of "arguments"
//
// Returns nil if no valid tool calls are found.
func ParseLeakedJSONToolCalls(content string) []model.ToolCall {
	content = strings.TrimSpace(content)

	// Try to find JSON array of tool calls
	// Look for patterns like: [{"name": "...", "arguments"|"param"|"parameters"|"input": ...}]
	jsonArrayRegex := regexp.MustCompile(`\[\s*\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}\s*\]`)
	if match := jsonArrayRegex.FindString(content); match != "" {
		var calls []struct {
			Name       string         `json:"name"`
			Arguments  map[string]any `json:"arguments"`
			Param      map[string]any `json:"param"`
			Parameters map[string]any `json:"parameters"`
			Input      map[string]any `json:"input"`
		}
		if err := json.Unmarshal([]byte(match), &calls); err == nil && len(calls) > 0 {
			result := make([]model.ToolCall, len(calls))
			for i, c := range calls {
				// Get arguments from whichever field is populated
				args := c.Arguments
				if args == nil {
					args = c.Param
				}
				if args == nil {
					args = c.Parameters
				}
				if args == nil {
					args = c.Input
				}
				result[i] = model.ToolCall{
					Name:      c.Name,
					Arguments: args,
				}
			}
			config.Debugf("[Provider] Parsed %d leaked JSON tool calls from content", len(result))
			return result
		}
	}

	// Try single object format: {"name": "...", "arguments"|"param"|"parameters"|"input": ...}
	jsonObjRegex := regexp.MustCompile(`\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}`)
	if match := jsonObjRegex.FindString(content); match != "" {
		var call struct {
			Name       string         `json:"name"`
			Arguments  map[string]any `json:"arguments"`
			Param      map[string]any `json:"param"`
			Parameters map[string]any `json:"parameters"`
			Input      map[string]any `json:"input"`
		}
		if err := json.Unmarshal([]byte(match), &call); err == nil && call.Name != "" {
			// Get arguments from whichever field is populated
			args := call.Arguments
			if args == nil {
				args = call.Param
			}
			if args == nil {
				args = call.Parameters
			}
			if args == nil {
				args = call.Input
			}
			config.Debugf("[Provider] Parsed 1 leaked JSON tool call from content: %s", call.Name)
			return []model.ToolCall{{
				Name:      call.Name,
				Arguments: args,
			}}
		}
	}

	return nil
}

// ParseLeakedXMLToolCalls detects and parses tool calls leaked as XML text in response content.
// Some models (like qwen) output tool calls as XML instead of using the proper API format.
//
// Supported formats:
//   - <tool_call><name>tool</name><arguments>{"key": "value"}</arguments></tool_call>
//   - <function_call>...</function_call>
//   - <function=TOOL_NAME><parameter=PARAM_NAME>VALUE</parameter></function> (qwen3-coder)
//
// Returns nil if no valid tool calls are found.
func ParseLeakedXMLToolCalls(content string) []model.ToolCall {
	var result []model.ToolCall

	// Pattern for <tool_call> or <function_call> tags
	toolCallRegex := regexp.MustCompile(`<(?:tool_call|function_call)>\s*<name>([^<]+)</name>\s*<arguments>([^<]*)</arguments>\s*</(?:tool_call|function_call)>`)
	matches := toolCallRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			name := strings.TrimSpace(match[1])
			argsStr := strings.TrimSpace(match[2])

			var args map[string]any
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = make(map[string]any)
			}

			result = append(result, model.ToolCall{
				Name:      name,
				Arguments: args,
			})
		}
	}

	// Pattern for qwen3-coder style: <function=TOOL_NAME><parameter=PARAM_NAME>VALUE</parameter></function>
	qwenRegex := regexp.MustCompile(`<function=([^>]+)><parameter=([^>]+)>\s*([^<]*)</parameter></function>`)
	qwenMatches := qwenRegex.FindAllStringSubmatch(content, -1)

	for _, match := range qwenMatches {
		if len(match) >= 4 {
			name := strings.TrimSpace(match[1])
			paramName := strings.TrimSpace(match[2])
			paramValue := strings.TrimSpace(match[3])

			args := map[string]any{
				paramName: paramValue,
			}

			result = append(result, model.ToolCall{
				Name:      name,
				Arguments: args,
			})
		}
	}

	if len(result) > 0 {
		config.Debugf("[Provider] Parsed %d leaked XML tool calls from content", len(result))
	}

	return result
}

// ConvertToProviderToolCalls converts Ollama api.ToolCall to provider-agnostic model.ToolCall.
//
// This conversion abstracts away Ollama-specific tool call structures, allowing the
// OTUI model layer to work with tool calls in a provider-agnostic way. This is
// particularly useful when adding support for other providers (OpenAI, Anthropic)
// that have different tool call formats.
//
// Returns nil if the input is nil or empty, maintaining the same nil semantics as
// the Ollama API.
//
// Example:
//
//	ollamaCalls := []api.ToolCall{
//	    {Function: api.ToolCallFunction{
//	        Name: "get_weather",
//	        Arguments: map[string]any{"city": "San Francisco"},
//	    }},
//	}
//	providerCalls := ConvertToProviderToolCalls(ollamaCalls)
//	// providerCalls[0].Name == "get_weather"
func ConvertToProviderToolCalls(ollamaCalls []api.ToolCall) []model.ToolCall {
	if len(ollamaCalls) == 0 {
		return nil
	}

	result := make([]model.ToolCall, len(ollamaCalls))
	for i, call := range ollamaCalls {
		result[i] = model.ToolCall{
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		}
	}
	return result
}

// decodeToolCallNames converts API safe tool names (see mcp.EncodeToolName)
// back to the namespaced "plugin.tool" form used by the MCP manager
func decodeToolCallNames(calls []model.ToolCall) []model.ToolCall {
	for i := range calls {
		calls[i].Name = mcp.DecodeToolName(calls[i].Name)
	}
	return calls
}
