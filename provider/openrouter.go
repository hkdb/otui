package provider

import (
	"context"
	"fmt"
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

// Chat implements Provider.Chat by delegating to ChatWithTools with no tools.
func (p *OpenRouterProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with streaming support.
func (p *OpenRouterProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Convert OTUI messages to OpenAI format
	openaiMessages := ConvertToOpenAIMessages(messages)

	// Build request parameters
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(p.model),
	}

	// Add tools if provided
	if len(tools) > 0 {
		openaiTools := mcp.ConvertMCPToolsToOpenAIFormat(tools)
		params.Tools = openaiTools
	}

	// Create streaming request
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	// Process stream
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		// Handle finished tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			if callback != nil {
				// Convert to provider tool call
				args := ParseToolArguments(tool.Arguments)
				toolCall := model.ToolCall{
					Name:      tool.Name,
					Arguments: args,
				}
				callback("", []model.ToolCall{toolCall})
			}
		}

		// Send content delta
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if callback != nil {
				callback(chunk.Choices[0].Delta.Content, nil)
			}
		}
	}

	// Check for errors
	if err := stream.Err(); err != nil {
		return fmt.Errorf("OpenRouter streaming error: %w", err)
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
