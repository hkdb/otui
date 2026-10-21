package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"otui/config"
	"otui/mcp"
	"otui/model"
	"otui/ollama"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// AnthropicProvider implements the Provider interface using Anthropic's official API.
// It uses the official Anthropic Go SDK for direct Claude API access.
type AnthropicProvider struct {
	client  *anthropic.Client
	model   anthropic.Model
	modelMu sync.RWMutex // SetModel (Update) vs requests (Cmd goroutines)
	baseURL string
	apiKey  string

	// Model metadata fetched from the Models API (static per model)
	metadataMu    sync.Mutex
	metadataCache map[string]model.ModelMetadata
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
	anthropicMessages, systemPrompt := convertToAnthropicMessages(messages, len(tools) > 0)

	// Prepend tool instructions to system blocks if tools present
	finalSystemPrompt := systemPrompt
	if len(tools) > 0 {
		toolInstructionBlock := anthropic.TextBlockParam{
			Text: buildToolInstructions(tools),
		}
		// Tool instructions go FIRST (Layer 1), then user prompts (Layer 2)
		finalSystemPrompt = append([]anthropic.TextBlockParam{toolInstructionBlock}, systemPrompt...)
	}

	// Build request parameters. max_tokens is required: the model's own
	// output limit, so long replies aren't cut off early
	modelName := p.GetModel()
	maxTokens := p.maxOutputTokens(ctx, modelName)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(modelName),
		Messages:  anthropicMessages,
		MaxTokens: int64(maxTokens),
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
	defer stream.Close()

	// Accumulate message
	msg := anthropic.Message{}

	// Track content for leak detection
	var contentBuilder strings.Builder

	// Process stream
	for stream.Next() {
		event := stream.Current()

		// Accumulate the event into the message
		err := msg.Accumulate(event)
		if err != nil {
			return fmt.Errorf("error accumulating message: %w", err)
		}

		// Report text deltas; other events (e.g. tool input deltas) are
		// reported without text, as activity
		text := ""
		if delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if textDelta, ok := delta.Delta.AsAny().(anthropic.TextDelta); ok {
				text = textDelta.Text
				contentBuilder.WriteString(text)
			}
		}
		if callback != nil {
			if err := callback(text, nil); err != nil {
				return err
			}
		}
	}

	// Check for errors
	if err := stream.Err(); err != nil {
		return fmt.Errorf("Anthropic streaming error: %w", err)
	}

	// Cut off at the output limit: the text so far is kept by the caller;
	// tool calls may be incomplete, so they're not run
	if msg.StopReason == anthropic.StopReasonMaxTokens {
		return fmt.Errorf("the reply was cut off at the model's output limit (%d tokens)", maxTokens)
	}

	// After stream completes, check for tool calls in the final message
	if callback != nil {
		toolCalls := extractToolCalls(msg.Content)
		if len(toolCalls) > 0 {
			callback("", toolCalls)
		} else {
			// Safety check: detect leaked tool calls if none were detected via API
			fullContent := contentBuilder.String()

			// Check for JSON leaked tool calls
			if leakedCalls := ParseLeakedJSONToolCalls(fullContent); len(leakedCalls) > 0 {
				callback("", decodeToolCallNames(leakedCalls))
			}

			// Check for XML leaked tool calls
			if leakedCalls := ParseLeakedXMLToolCalls(fullContent); len(leakedCalls) > 0 {
				callback("", decodeToolCallNames(leakedCalls))
			}
		}
	}

	return nil
}

// anthropicFallbackModels is listed when the Models API can't be reached
var anthropicFallbackModels = []string{
	"claude-fable-5-1",
	"claude-opus-5-5",
	"claude-sonnet-5-5",
	"claude-haiku-5-5",
	"claude-haiku-4-5",
}

// ListModels implements Provider.ListModels with Anthropic's Models API
// (falls back to a built-in list if it fails)
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	var ids []string
	pager := p.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{}, option.WithMaxRetries(0))
	for pager.Next() {
		ids = append(ids, pager.Current().ID)
	}
	if err := pager.Err(); err != nil || len(ids) == 0 {
		config.Debugf("[Anthropic] Models API unavailable (%v), using built-in model list", err)
		ids = anthropicFallbackModels
	}

	result := make([]ollama.ModelInfo, 0, len(ids))
	for _, id := range ids {
		result = append(result, ollama.ModelInfo{
			Name:         id,
			InternalName: id,
			Size:         0,           // Anthropic doesn't provide size info
			Provider:     "anthropic", // Must match the provider ID (selecting the model switches to it)
		})
	}

	return result, nil
}

// anthropicDefaultMaxOutput is used for models the Models API and the
// fallback table don't know; every current Claude model accepts it
const anthropicDefaultMaxOutput = 8192

// maxOutputTokens returns the model's output limit for max_tokens: from the
// Models API (cached), else the fallback table, else a safe default
func (p *AnthropicProvider) maxOutputTokens(ctx context.Context, modelName string) int {
	meta, _ := p.GetModelMetadata(ctx, modelName)
	if meta.MaxOutput > 0 {
		return meta.MaxOutput
	}
	return anthropicDefaultMaxOutput
}

// GetModel implements Provider.GetModel.
// Returns the full model name for API calls.
func (p *AnthropicProvider) GetModel() string {
	p.modelMu.RLock()
	defer p.modelMu.RUnlock()
	return string(p.model)
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name for UI display (same as GetModel for Anthropic).
func (p *AnthropicProvider) GetDisplayName() string {
	p.modelMu.RLock()
	defer p.modelMu.RUnlock()
	return string(p.model)
}

// SetModel implements Provider.SetModel.
func (p *AnthropicProvider) SetModel(model string) {
	p.modelMu.Lock()
	defer p.modelMu.Unlock()
	p.model = anthropic.Model(model)
}

// Ping implements Provider.Ping by attempting to create a minimal request.
func (p *AnthropicProvider) Ping(ctx context.Context) error {
	// Anthropic doesn't have a ping/health endpoint, so we make a minimal request
	_, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.GetModel()),
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

// GetModelMetadata returns metadata for the specified model.
// The Models API reports max_input_tokens and max_tokens (added 2026-03-18;
// the SDK version in use doesn't have typed fields for them yet, so they're
// read from the raw response). Falls back to the hardcoded table.
func (p *AnthropicProvider) GetModelMetadata(ctx context.Context, modelName string) (model.ModelMetadata, error) {
	p.metadataMu.Lock()
	cached, ok := p.metadataCache[modelName]
	p.metadataMu.Unlock()
	if ok {
		return cached, nil
	}

	fallback := p.FallbackModelMetadata(modelName)

	// No retries: this is metadata (the fallback works), not worth delaying a request
	info, err := p.client.Models.Get(ctx, modelName, anthropic.ModelGetParams{}, option.WithMaxRetries(0))
	if err != nil {
		return fallback, nil
	}

	contextWindow := rawJSONInt(info.JSON.ExtraFields["max_input_tokens"].Raw())
	if contextWindow <= 0 {
		return fallback, nil
	}

	meta := fallback
	meta.ContextWindow = contextWindow
	meta.SupportsTools = true // All Claude models support tools
	meta.Source = "anthropic:/v1/models max_input_tokens"
	if maxTokens := rawJSONInt(info.JSON.ExtraFields["max_tokens"].Raw()); maxTokens > 0 {
		meta.MaxOutput = maxTokens
	}

	p.metadataMu.Lock()
	if p.metadataCache == nil {
		p.metadataCache = make(map[string]model.ModelMetadata)
	}
	p.metadataCache[modelName] = meta
	p.metadataMu.Unlock()

	return meta, nil
}

// FallbackModelMetadata returns metadata from the hardcoded table
func (p *AnthropicProvider) FallbackModelMetadata(modelName string) model.ModelMetadata {
	meta, ok := lookupFallbackMetadata(modelName, AnthropicFallbackMetadata)
	if !ok {
		meta.MaxOutput = anthropicDefaultMaxOutput // The generic default (2048) is too low for Claude
	}
	return toModelMetadata(meta, model.MetadataSourceFallback)
}

// convertToAnthropicMessages converts OTUI messages to Anthropic format.
// Returns the message array and any system prompt found.
func convertToAnthropicMessages(messages []model.Message, nativeTools bool) ([]anthropic.MessageParam, []anthropic.TextBlockParam) {
	var systemBlocks []anthropic.TextBlockParam
	anthropicMsgs := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		// Native tool calls and results (the API requires tools in the request
		// whenever history has tool_use/tool_result blocks)
		nativeCall := nativeTools && msg.Role == "assistant" && len(msg.ToolCalls) > 0
		nativeResult := nativeTools && msg.Role == "tool" && msg.ToolCallID != ""

		// The API rejects empty or whitespace-only text blocks. They come from
		// e.g. a tool-only assistant reply in the tool loop and carry nothing.
		if strings.TrimSpace(msg.Content) == "" && !nativeCall && !nativeResult {
			continue
		}

		// Anthropic uses a separate system parameter, not in messages array
		if msg.Role == "system" {
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: msg.Content})
			continue
		}

		// Tool results (and unknown roles) go to user messages
		role := anthropic.MessageParamRoleUser
		if msg.Role == "assistant" {
			role = anthropic.MessageParamRoleAssistant
		}

		var blocks []anthropic.ContentBlockParamUnion
		switch {
		case nativeResult:
			blocks = append(blocks, anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false))
		case nativeCall:
			if strings.TrimSpace(msg.Content) != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				input := call.Arguments
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, input, mcp.EncodeToolName(call.Name)))
			}
		default:
			blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
		}

		// Consecutive messages with the same role become one turn (e.g. the
		// results of several tool calls, which always come right after the
		// tool_use turn and so stay first in their turn)
		if n := len(anthropicMsgs); n > 0 && anthropicMsgs[n-1].Role == role {
			anthropicMsgs[n-1].Content = append(anthropicMsgs[n-1].Content, blocks...)
			continue
		}
		anthropicMsgs = append(anthropicMsgs, anthropic.MessageParam{Role: role, Content: blocks})
	}

	// A final assistant turn (prefill) must not end with whitespace
	if n := len(anthropicMsgs); n > 0 && anthropicMsgs[n-1].Role == anthropic.MessageParamRoleAssistant {
		blocks := anthropicMsgs[n-1].Content
		if last := blocks[len(blocks)-1].OfText; last != nil {
			last.Text = strings.TrimRight(last.Text, " \t\r\n")
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
				ID:        toolUse.ID,
				Name:      mcp.DecodeToolName(toolUse.Name),
				Arguments: args,
			})
		}
	}

	return toolCalls
}
