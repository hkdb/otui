package provider

import (
	"context"
	"fmt"
	"otui/config"
	"otui/mcp"
	"otui/model"
	"otui/ollama"
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenRouterProvider implements the Provider interface using OpenAI's official Go SDK.
// It connects to OpenRouter's API which is 100% OpenAI-compatible.
type OpenRouterProvider struct {
	client  openai.Client
	model   string
	baseURL string
	apiKey  string
}

// NewOpenRouterProvider creates a new OpenRouter provider instance.
//
// Parameters:
//   - baseURL: OpenRouter API base URL ("https://openrouter.ai/api/v1")
//   - apiKey: OpenRouter API key
//   - model: Initial model to use (can be changed with SetModel)
//
// Returns an error if the client cannot be created.
func NewOpenRouterProvider(baseURL, apiKey, model string) (*OpenRouterProvider, error) {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key is required")
	}
	if model == "" {
		model = "meta-llama/llama-3.2-90b-instruct" // Default model
	}

	// Create OpenAI client with custom base URL for OpenRouter
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &OpenRouterProvider{
		client:  client,
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

// shouldSkipToolInstructions checks if a model BREAKS with explicit tool instructions.
// Most models work well with instructions, but some models (like qwen) understand
// tools natively and get confused by explicit prompting, causing XML leakage.
func shouldSkipToolInstructions(modelName string) bool {
	modelLower := strings.ToLower(modelName)

	// Blacklist: Models that BREAK with explicit instructions
	skipInstructions := []string{
		"qwen", // Leaks XML with instructions, works natively without them
	}

	for _, prefix := range skipInstructions {
		if strings.Contains(modelLower, prefix) {
			return true
		}
	}

	// Default: send instructions (most models benefit from them)
	return false
}

// convertToolNamesForOpenRouter converts tool names from dotted notation to underscore notation.
// OpenRouter API requires tool names matching ^[a-zA-Z0-9_-]{1,64}$ (no dots allowed).
// Example: "server-filesystem.read_file" → "server-filesystem__read_file"
func convertToolNamesForOpenRouter(tools []mcptypes.Tool) []mcptypes.Tool {
	converted := make([]mcptypes.Tool, len(tools))
	for i, tool := range tools {
		converted[i] = tool
		converted[i].Name = strings.ReplaceAll(tool.Name, ".", "__")
	}
	return converted
}

// convertToolNameFromOpenRouter converts a tool name from underscore notation back to dotted notation.
// This reverses the conversion applied by convertToolNamesForOpenRouter.
// Example: "server-filesystem__read_file" → "server-filesystem.read_file"
func convertToolNameFromOpenRouter(toolName string) string {
	return strings.ReplaceAll(toolName, "__", ".")
}

// Chat implements Provider.Chat by delegating to ChatWithTools with no tools.
func (p *OpenRouterProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with streaming support.
func (p *OpenRouterProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Prepend tool instructions if tools present (unless model is blacklisted)
	messagesWithInstructions := messages
	if len(tools) > 0 && !shouldSkipToolInstructions(p.model) {
		toolInstruction := model.Message{
			Role:    "system",
			Content: buildOpenRouterToolInstructions(tools),
		}
		messagesWithInstructions = append([]model.Message{toolInstruction}, messages...)
	}

	// Debug logging for tool instruction decisions (no else statements)
	if config.DebugLog != nil && len(tools) > 0 && shouldSkipToolInstructions(p.model) {
		config.DebugLog.Printf("[OpenRouter] Model '%s': Skipping tool instructions (blacklisted - uses native understanding)", p.model)
	}

	if config.DebugLog != nil && len(tools) > 0 && !shouldSkipToolInstructions(p.model) {
		config.DebugLog.Printf("[OpenRouter] Model '%s': Adding tool instructions", p.model)
	}

	// Convert OTUI messages to OpenAI format
	openaiMessages := ConvertToOpenAIMessages(messagesWithInstructions)

	// Build request parameters
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(p.model),
	}

	// Add tools if provided (convert dots to underscores for OpenRouter API)
	if len(tools) > 0 {
		convertedTools := convertToolNamesForOpenRouter(tools)
		openaiTools := mcp.ConvertMCPToolsToOpenAIFormat(convertedTools)
		params.Tools = openaiTools
	}

	// Create streaming request
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	// Track if we got tool calls via API
	var apiToolCallsDetected bool
	var contentBuilder strings.Builder

	// Process stream
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		// Handle finished tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			apiToolCallsDetected = true
			if callback != nil {
				// Convert to provider tool call (convert underscores back to dots)
				args := ParseToolArguments(tool.Arguments)
				toolCall := model.ToolCall{
					Name:      convertToolNameFromOpenRouter(tool.Name),
					Arguments: args,
				}
				callback("", []model.ToolCall{toolCall})
			}
		}

		// Send content delta and accumulate for leak detection
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			content := chunk.Choices[0].Delta.Content
			contentBuilder.WriteString(content)
			if callback != nil {
				callback(content, nil)
			}
		}
	}

	// Check for errors
	if err := stream.Err(); err != nil {
		return fmt.Errorf("OpenRouter streaming error: %w", err)
	}

	// Safety check: detect leaked tool calls if none were detected via API
	if !apiToolCallsDetected && callback != nil {
		fullContent := contentBuilder.String()

		// Check for JSON leaked tool calls
		if leakedCalls := ParseLeakedJSONToolCalls(fullContent); len(leakedCalls) > 0 {
			// Convert tool names from OpenRouter format
			for i := range leakedCalls {
				leakedCalls[i].Name = convertToolNameFromOpenRouter(leakedCalls[i].Name)
			}
			callback("", leakedCalls)

			// Note: Content was already streamed, but we could clean it in future
			// by tracking and re-sending cleaned content
		}

		// Check for XML leaked tool calls
		if leakedCalls := ParseLeakedXMLToolCalls(fullContent); len(leakedCalls) > 0 {
			// Convert tool names from OpenRouter format
			for i := range leakedCalls {
				leakedCalls[i].Name = convertToolNameFromOpenRouter(leakedCalls[i].Name)
			}
			callback("", leakedCalls)
		}
	}

	return nil
}

// ListModels implements Provider.ListModels with prefix stripping.
func (p *OpenRouterProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	// Fetch models from OpenRouter
	modelsPage, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list OpenRouter models: %w", err)
	}

	// Convert to ModelInfo with prefix stripping
	result := make([]ollama.ModelInfo, 0, len(modelsPage.Data))
	for _, m := range modelsPage.Data {
		result = append(result, ollama.ModelInfo{
			Name:         stripProviderPrefix(m.ID), // Display: "llama-3.2-90b-instruct"
			InternalName: m.ID,                      // API: "meta-llama/llama-3.2-90b-instruct"
			Size:         0,                         // OpenRouter doesn't provide size
			Provider:     "openrouter",
		})
	}

	return result, nil
}

// GetModel implements Provider.GetModel.
// Returns the full model name with vendor prefix for API calls.
// Example: "qwen/qwen3-coder:free"
func (p *OpenRouterProvider) GetModel() string {
	return p.model
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name with vendor prefix stripped for UI display.
// Example: "qwen/qwen3-coder:free" → "qwen3-coder:free"
func (p *OpenRouterProvider) GetDisplayName() string {
	return stripProviderPrefix(p.model)
}

// SetModel implements Provider.SetModel.
func (p *OpenRouterProvider) SetModel(model string) {
	p.model = model
}

// Ping implements Provider.Ping by attempting to list models.
func (p *OpenRouterProvider) Ping(ctx context.Context) error {
	_, err := p.client.Models.List(ctx)
	if err != nil {
		return fmt.Errorf("OpenRouter ping failed: %w", err)
	}
	return nil
}

// stripProviderPrefix removes vendor prefixes from OpenRouter model names.
// "meta-llama/llama-3.2-90b-instruct" → "llama-3.2-90b-instruct"
// "anthropic/claude-sonnet-4" → "claude-sonnet-4"
func stripProviderPrefix(modelName string) string {
	if idx := strings.Index(modelName, "/"); idx != -1 {
		return modelName[idx+1:]
	}
	return modelName
}

// ConvertToOpenAIMessages converts OTUI messages to OpenAI format.
func ConvertToOpenAIMessages(messages []model.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, len(messages))

	for i, msg := range messages {
		switch msg.Role {
		case "system":
			result[i] = openai.SystemMessage(msg.Content)
		case "user":
			result[i] = openai.UserMessage(msg.Content)
		case "assistant":
			result[i] = openai.AssistantMessage(msg.Content)
		case "tool":
			// Tool messages are sent as user messages for now
			// TODO: Add ToolCallID to Message struct to properly handle tool responses
			result[i] = openai.UserMessage(msg.Content)
		default:
			// Default to user message
			result[i] = openai.UserMessage(msg.Content)
		}
	}

	return result
}
