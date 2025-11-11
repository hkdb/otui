package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"otui/mcp"
	"otui/model"
	"otui/ollama"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// AnthropicProvider implements the Provider interface using Anthropic's official API.
// It uses the official Anthropic Go SDK for direct Claude API access.
type AnthropicProvider struct {
	client  *anthropic.Client
	model   anthropic.Model
	baseURL string
	apiKey  string
}

// NewAnthropicProvider creates a new Anthropic provider instance.
//
// Parameters:
//   - baseURL: Anthropic API base URL (default: "https://api.anthropic.com")
//   - apiKey: Anthropic API key (required)
//   - model: Initial model to use (default: "claude-sonnet-4-5-20250929")
//
// Returns an error if the API key is missing.
func NewAnthropicProvider(baseURL, apiKey, model string) (*AnthropicProvider, error) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}

	// Default to Claude 3.5 Sonnet v2
	var anthropicModel anthropic.Model
	if model == "" {
		anthropicModel = anthropic.ModelClaudeSonnet4_5_20250929
	} else {
		// Convert string to Model type
		anthropicModel = anthropic.Model(model)
	}

	// Create Anthropic client
	client := anthropic.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &AnthropicProvider{
		client:  &client, // Convert value to pointer
		model:   anthropicModel,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

// Chat implements Provider.Chat by delegating to ChatWithTools with no tools.
func (p *AnthropicProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with streaming support.
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Convert OTUI messages to Anthropic format
	anthropicMessages, systemPrompt := convertToAnthropicMessages(messages)

	// Prepend tool instructions to system blocks if tools present
	finalSystemPrompt := systemPrompt
	if len(tools) > 0 {
		toolInstructionBlock := anthropic.TextBlockParam{
			Text: buildAnthropicToolInstructions(tools),
		}
		// Tool instructions go FIRST (Layer 1), then user prompts (Layer 2)
		finalSystemPrompt = append([]anthropic.TextBlockParam{toolInstructionBlock}, systemPrompt...)
	}

	// Build request parameters
	params := anthropic.MessageNewParams{
		Model:     p.model,
		Messages:  anthropicMessages,
		MaxTokens: 4096, // Required by Anthropic API
	}

	// Add system prompt if present
	if len(finalSystemPrompt) > 0 {
		params.System = finalSystemPrompt
	}

	// Add tools if provided
	if len(tools) > 0 {
		anthropicTools := mcp.ConvertMCPToolsToAnthropicFormat(tools)
		params.Tools = anthropicTools
	}

	// Create streaming request
	stream := p.client.Messages.NewStreaming(ctx, params)

	// Accumulate message
	msg := anthropic.Message{}

	// Process stream
	for stream.Next() {
		event := stream.Current()

		// Accumulate the event into the message
		err := msg.Accumulate(event)
		if err != nil {
			return fmt.Errorf("error accumulating message: %w", err)
		}

		// Handle different event types
		switch eventVariant := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			// Handle text deltas
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if callback != nil {
					callback(deltaVariant.Text, nil)
				}
			}
		}
	}

	// Check for errors
	if err := stream.Err(); err != nil {
		return fmt.Errorf("Anthropic streaming error: %w", err)
	}

	// After stream completes, check for tool calls in the final message
	if callback != nil {
		toolCalls := extractToolCalls(msg.Content)
		if len(toolCalls) > 0 {
			callback("", toolCalls)
		}
	}

	return nil
}

// ListModels implements Provider.ListModels.
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	// Anthropic doesn't have a models list API, so we return a curated list
	// of known Claude models as of the SDK version we're using
	models := []anthropic.Model{
		anthropic.ModelClaudeSonnet4_5_20250929, // Claude 3.5 Sonnet v2 (latest)
		anthropic.ModelClaude3_5Haiku20241022,   // Claude 3.5 Haiku
		anthropic.ModelClaude_3_Opus_20240229,   // Claude 3 Opus
		anthropic.ModelClaude_3_Haiku_20240307,  // Claude 3 Haiku
	}

	result := make([]ollama.ModelInfo, 0, len(models))
	for _, m := range models {
		modelStr := string(m)
		result = append(result, ollama.ModelInfo{
			Name:         modelStr,
			InternalName: modelStr,
			Size:         0,           // Anthropic doesn't provide size info
			Provider:     "anthropic", // CRITICAL: Must match provider ID
		})
	}

	return result, nil
}

// GetModel implements Provider.GetModel.
// Returns the full model name for API calls.
func (p *AnthropicProvider) GetModel() string {
	return string(p.model)
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name for UI display (same as GetModel for Anthropic).
func (p *AnthropicProvider) GetDisplayName() string {
	return string(p.model)
}

// SetModel implements Provider.SetModel.
func (p *AnthropicProvider) SetModel(model string) {
	p.model = anthropic.Model(model)
}

// Ping implements Provider.Ping by attempting to create a minimal request.
func (p *AnthropicProvider) Ping(ctx context.Context) error {
	// Anthropic doesn't have a ping/health endpoint, so we make a minimal request
	_, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("ping")),
		},
	})

	if err != nil {
		return fmt.Errorf("Anthropic ping failed: %w", err)
	}
	return nil
}

// convertToAnthropicMessages converts OTUI messages to Anthropic format.
// Returns the message array and any system prompt found.
func convertToAnthropicMessages(messages []model.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam) {
	var systemBlocks []anthropic.TextBlockParam
	anthropicMsgs := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// Anthropic uses a separate system parameter, not in messages array
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
				Text: msg.Content,
			})

		case "user":
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)),
			)

		case "assistant":
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)),
			)

		case "tool":
			// Tool results go to user messages for now
			// TODO: Properly handle tool result format when implementing tool support
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)),
			)

		default:
			// Default to user message
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)),
			)
		}
	}

	return anthropicMsgs, systemBlocks
}

// extractToolCalls extracts tool calls from Anthropic message content.
func extractToolCalls(content []anthropic.ContentBlockUnion) []model.ToolCall {
	var toolCalls []model.ToolCall

	for _, block := range content {
		blockVariant := block.AsAny()
		if toolUse, ok := blockVariant.(anthropic.ToolUseBlock); ok {
			// Convert json.RawMessage to map[string]any
			var args map[string]any
			if err := json.Unmarshal(toolUse.Input, &args); err != nil {
				// Skip if we can't parse the arguments
				continue
			}

			toolCalls = append(toolCalls, model.ToolCall{
				Name:      toolUse.Name,
				Arguments: args,
			})
		}
	}

	return toolCalls
}
