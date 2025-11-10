package provider

import (
	"encoding/json"
	"otui/model"

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
