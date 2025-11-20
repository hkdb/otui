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

// OpenAIProvider implements the Provider interface using OpenAI's official API.
// It uses the official OpenAI Go SDK for direct OpenAI API access.
type OpenAIProvider struct {
	client  openai.Client
	model   string
	baseURL string
	apiKey  string
}

// NewOpenAIProvider creates a new OpenAI provider instance.
//
// Parameters:
//   - baseURL: OpenAI API base URL (default: "https://api.openai.com/v1")
//   - apiKey: OpenAI API key (required)
//   - model: Initial model to use (default: "gpt-4o-mini")
//
// Returns an error if the API key is missing.
func NewOpenAIProvider(baseURL, apiKey, model string) (*OpenAIProvider, error) {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	if model == "" {
		model = "gpt-4o-mini" // Default to affordable model
	}

	// Create OpenAI client
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &OpenAIProvider{
		client:  client,
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

// Chat implements Provider.Chat by delegating to ChatWithTools with no tools.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with streaming support.
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Prepend tool instructions if tools present
	messagesWithInstructions := messages
	if len(tools) > 0 {
		toolInstruction := model.Message{
			Role:    "system",
			Content: buildOpenAIToolInstructions(tools),
		}
		messagesWithInstructions = append([]model.Message{toolInstruction}, messages...)
	}

	// Convert OTUI messages to OpenAI format
	openaiMessages := ConvertToOpenAIMessages(messagesWithInstructions)

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
				// Convert to provider tool call
				args := ParseToolArguments(tool.Arguments)
				toolCall := model.ToolCall{
					Name:      tool.Name,
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
		return fmt.Errorf("OpenAI streaming error: %w", err)
	}

	// Safety check: detect leaked tool calls if none were detected via API
	if !apiToolCallsDetected && callback != nil {
		fullContent := contentBuilder.String()

		// Check for JSON leaked tool calls
		if leakedCalls := ParseLeakedJSONToolCalls(fullContent); len(leakedCalls) > 0 {
			callback("", leakedCalls)
		}

		// Check for XML leaked tool calls
		if leakedCalls := ParseLeakedXMLToolCalls(fullContent); len(leakedCalls) > 0 {
			callback("", leakedCalls)
		}
	}

	return nil
}

// ListModels implements Provider.ListModels.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	// Fetch models from OpenAI
	modelsPage, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list OpenAI models: %w", err)
	}

	// Convert to ModelInfo
	result := make([]ollama.ModelInfo, 0, len(modelsPage.Data))
	for _, m := range modelsPage.Data {
		result = append(result, ollama.ModelInfo{
			Name:         m.ID,     // OpenAI models don't have vendor prefixes
			InternalName: m.ID,     // Same as display name
			Size:         0,        // OpenAI doesn't provide size info
			Provider:     "openai", // CRITICAL: Must match provider ID
		})
	}

	return result, nil
}

// GetModel implements Provider.GetModel.
// Returns the full model name for API calls.
func (p *OpenAIProvider) GetModel() string {
	return p.model
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name for UI display (same as GetModel for OpenAI).
func (p *OpenAIProvider) GetDisplayName() string {
	return p.model
}

// SetModel implements Provider.SetModel.
func (p *OpenAIProvider) SetModel(model string) {
	p.model = model
}

// Ping implements Provider.Ping by attempting to list models.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	_, err := p.client.Models.List(ctx)
	if err != nil {
		return fmt.Errorf("OpenAI ping failed: %w", err)
	}
	return nil
}
