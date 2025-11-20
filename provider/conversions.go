package provider

import (
	"encoding/json"
	"otui/config"
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
func ConvertToOllamaMessages(messages []model.Message) []api.Message {
	result := make([]api.Message, len(messages))
	for i, msg := range messages {
		result[i] = api.Message{
			Role:    msg.Role,
			Content: msg.Content,
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
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Provider] Parsed %d leaked JSON tool calls from content", len(result))
			}
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
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Provider] Parsed 1 leaked JSON tool call from content: %s", call.Name)
			}
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

	if len(result) > 0 && config.DebugLog != nil {
		config.DebugLog.Printf("[Provider] Parsed %d leaked XML tool calls from content", len(result))
	}

	return result
}

// CleanLeakedToolCalls removes leaked JSON/XML tool calls from content.
// Returns cleaned content suitable for display to the user.
func CleanLeakedToolCalls(content string) string {
	// Remove JSON array tool calls (with argument variations)
	jsonArrayRegex := regexp.MustCompile(`\[\s*\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}\s*\]`)
	content = jsonArrayRegex.ReplaceAllString(content, "")

	// Remove single JSON object tool calls (with argument variations)
	jsonObjRegex := regexp.MustCompile(`\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}`)
	content = jsonObjRegex.ReplaceAllString(content, "")

	// Remove XML tool calls
	xmlRegex := regexp.MustCompile(`<(?:tool_call|function_call)>\s*<name>[^<]+</name>\s*<arguments>[^<]*</arguments>\s*</(?:tool_call|function_call)>`)
	content = xmlRegex.ReplaceAllString(content, "")

	// Remove qwen3-coder style XML tool calls (with multiline support)
	// Pattern: <function=TOOL_NAME><parameter=PARAM_NAME>VALUE</parameter></function>
	// (?s) enables dot to match newlines
	qwenXmlRegex := regexp.MustCompile(`(?s)<function=[^>]+><parameter=[^>]+>.*?</parameter></function>(?:</tool_call>)?`)
	content = qwenXmlRegex.ReplaceAllString(content, "")

	// Remove system-reminder tags that may leak into content
	sysReminderRegex := regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	content = sysReminderRegex.ReplaceAllString(content, "")

	// Clean up extra whitespace
	content = strings.TrimSpace(content)

	return content
}

// ConvertFromOllamaMessages converts Ollama api.Message to OTUI model.Message.
//
// This conversion is used when receiving messages from the Ollama API. It creates
// OTUI messages with Role and Content fields populated from the Ollama message.
//
// Note: The Timestamp field is not set (will be zero value) because Ollama API
// messages don't include timestamp information. The Rendered field is also not set
// and should be populated by the UI layer when needed.
//
// Example:
//
//	ollamaMessages := []api.Message{
//	    {Role: "assistant", Content: "Response text"},
//	}
//	otuiMessages := ConvertFromOllamaMessages(ollamaMessages)
//	// otuiMessages[0].Timestamp will be zero, set it manually if needed
func ConvertFromOllamaMessages(messages []api.Message) []model.Message {
	result := make([]model.Message, len(messages))
	for i, msg := range messages {
		result[i] = model.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
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

// ConvertFromProviderToolCalls converts provider-agnostic model.ToolCall to Ollama api.ToolCall.
//
// This conversion is primarily used for testing and mocking purposes, allowing tests
// to create provider-agnostic tool calls and convert them to Ollama format for
// validation.
//
// Returns nil if the input is nil or empty, maintaining the same nil semantics.
//
// Example:
//
//	providerCalls := []model.ToolCall{
//	    {Name: "search", Arguments: map[string]any{"query": "golang"}},
//	}
//	ollamaCalls := ConvertFromProviderToolCalls(providerCalls)
//	// ollamaCalls[0].Function.Name == "search"
func ConvertFromProviderToolCalls(providerCalls []model.ToolCall) []api.ToolCall {
	if len(providerCalls) == 0 {
		return nil
	}

	result := make([]api.ToolCall, len(providerCalls))
	for i, call := range providerCalls {
		result[i] = api.ToolCall{
			Function: api.ToolCallFunction{
				Name:      call.Name,
				Arguments: call.Arguments,
			},
		}
	}
	return result
}
