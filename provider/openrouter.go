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

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenRouterProvider implements the Provider interface using OpenAI's official Go SDK.
// It connects to OpenRouter's API which is 100% OpenAI-compatible.
type OpenRouterProvider struct {
	client  openai.Client
	model   string
	modelMu sync.RWMutex // SetModel (Update) vs requests (Cmd goroutines)
	baseURL string
	apiKey  string

	// Model metadata from the model list (nil until fetched)
	metadataMu    sync.Mutex
	metadataCache map[string]model.ModelMetadata
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

// Chat implements Provider.Chat by delegating to ChatWithTools with no tools.
func (p *OpenRouterProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with streaming support.
func (p *OpenRouterProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	modelName := p.GetModel()

	// Prepend tool instructions if tools present (unless model is blacklisted)
	messagesWithInstructions := messages
	if len(tools) > 0 && !shouldSkipToolInstructions(modelName) {
		toolInstruction := model.Message{
			Role:    "system",
			Content: buildOpenRouterToolInstructions(tools),
		}
		messagesWithInstructions = append([]model.Message{toolInstruction}, messages...)
	}

	// Debug logging for tool instruction decisions (no else statements)
	if config.DebugLog != nil && len(tools) > 0 && shouldSkipToolInstructions(modelName) {
		config.DebugLog.Printf("[OpenRouter] Model '%s': Skipping tool instructions (blacklisted - uses native understanding)", modelName)
	}

	if config.DebugLog != nil && len(tools) > 0 && !shouldSkipToolInstructions(modelName) {
		config.DebugLog.Printf("[OpenRouter] Model '%s': Adding tool instructions", modelName)
	}

	// Convert OTUI messages to OpenAI format
	openaiMessages := ConvertToOpenAIMessages(messagesWithInstructions, len(tools) > 0)

	// Build request parameters
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(modelName),
	}

	// Add tools if provided (tool names are encoded to the API safe format)
	if len(tools) > 0 {
		openaiTools := mcp.ConvertMCPToolsToOpenAIFormat(tools)
		params.Tools = openaiTools
	}

	// Stream the response (tool calls are reported once the stream ends)
	content, toolCalls, err := streamOpenAIChat(ctx, p.client, params, "OpenRouter", callback)
	if err != nil {
		return err
	}

	// Safety check: detect leaked tool calls if none were reported via the API
	if len(toolCalls) == 0 && callback != nil {
		// Check for JSON leaked tool calls
		if leakedCalls := ParseLeakedJSONToolCalls(content); len(leakedCalls) > 0 {
			callback("", decodeToolCallNames(leakedCalls))
		}

		// Check for XML leaked tool calls
		if leakedCalls := ParseLeakedXMLToolCalls(content); len(leakedCalls) > 0 {
			callback("", decodeToolCallNames(leakedCalls))
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

	// The list includes context windows - cache them for GetModelMetadata
	p.cacheModelMetadata(modelsPage.Data)

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
	p.modelMu.RLock()
	defer p.modelMu.RUnlock()
	return p.model
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name with vendor prefix stripped for UI display.
// Example: "qwen/qwen3-coder:free" → "qwen3-coder:free"
func (p *OpenRouterProvider) GetDisplayName() string {
	return stripProviderPrefix(p.GetModel())
}

// SetModel implements Provider.SetModel.
func (p *OpenRouterProvider) SetModel(model string) {
	p.modelMu.Lock()
	defer p.modelMu.Unlock()
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

// GetModelMetadata returns metadata for the specified model from OpenRouter's
// model list (context_length, top_provider), falling back to name based
// defaults. The list is cached after the first fetch (ListModels fills it too).
func (p *OpenRouterProvider) GetModelMetadata(ctx context.Context, modelName string) (model.ModelMetadata, error) {
	p.metadataMu.Lock()
	meta, ok := p.metadataCache[modelName]
	loaded := p.metadataCache != nil
	p.metadataMu.Unlock()
	if ok {
		return meta, nil
	}

	if !loaded {
		if modelsPage, err := p.client.Models.List(ctx); err == nil {
			p.cacheModelMetadata(modelsPage.Data)
			p.metadataMu.Lock()
			meta, ok = p.metadataCache[modelName]
			p.metadataMu.Unlock()
			if ok {
				return meta, nil
			}
		}
	}

	return p.FallbackModelMetadata(modelName), nil
}

// cacheModelMetadata extracts context window info from OpenRouter's model list.
//
// OpenRouter reports the model's max context_length and top_provider.context_length
// for the provider requests are routed to by default; they can differ (e.g. a
// 1M model whose default provider serves 262K). top_provider is preferred as
// it's what requests actually get unless routed elsewhere.
func (p *OpenRouterProvider) cacheModelMetadata(models []openai.Model) {
	cache := make(map[string]model.ModelMetadata, len(models))
	for _, m := range models {
		var topProvider struct {
			ContextLength       int `json:"context_length"`
			MaxCompletionTokens int `json:"max_completion_tokens"`
		}
		if raw := m.JSON.ExtraFields["top_provider"].Raw(); raw != "" && raw != "null" {
			_ = json.Unmarshal([]byte(raw), &topProvider)
		}

		contextWindow, source := topProvider.ContextLength, "openrouter:top_provider.context_length"
		if contextWindow <= 0 {
			contextWindow = rawJSONInt(m.JSON.ExtraFields["context_length"].Raw())
			source = "openrouter:context_length"
		}
		if contextWindow <= 0 {
			continue
		}

		meta := p.FallbackModelMetadata(m.ID)
		meta.ContextWindow = contextWindow
		meta.SupportsTools = true // Most OpenRouter models support tools
		meta.Source = source
		if topProvider.MaxCompletionTokens > 0 {
			meta.MaxOutput = topProvider.MaxCompletionTokens
		}
		cache[m.ID] = meta
	}

	p.metadataMu.Lock()
	p.metadataCache = cache
	p.metadataMu.Unlock()
}

// FallbackModelMetadata returns defaults based on the model name, trying the
// Anthropic, OpenAI and Ollama tables since OpenRouter serves models from all
func (p *OpenRouterProvider) FallbackModelMetadata(modelName string) model.ModelMetadata {
	// Strip provider prefix for lookup (e.g., "anthropic/claude-sonnet-4" → "claude-sonnet-4")
	lookupName := stripProviderPrefix(modelName)

	for _, table := range []map[string]ModelMetadata{AnthropicFallbackMetadata, OpenAIFallbackMetadata, OllamaFallbackMetadata} {
		if meta := GetFallbackMetadata(lookupName, table); meta.ContextWindow > 8192 {
			return toModelMetadata(meta, model.MetadataSourceFallback)
		}
	}

	// Ultimate fallback
	return model.ModelMetadata{
		ContextWindow: 8192,
		MaxOutput:     2048,
		SupportsTools: false,
		Source:        model.MetadataSourceFallback,
	}
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
func ConvertToOpenAIMessages(messages []model.Message, nativeTools bool) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))

		case "assistant":
			// Native: the assistant turn records the tool calls it made (the
			// API requires tools in the request whenever history has them)
			if nativeTools && len(msg.ToolCalls) > 0 {
				assistant := openai.ChatCompletionAssistantMessageParam{}
				if msg.Content != "" {
					assistant.Content.OfString = openai.String(msg.Content)
				}
				for _, call := range msg.ToolCalls {
					args, err := json.Marshal(call.Arguments)
					if err != nil || call.Arguments == nil {
						args = []byte("{}")
					}
					assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: call.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      mcp.EncodeToolName(call.Name),
								Arguments: string(args),
							},
						},
					})
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
				continue
			}
			if msg.Content == "" {
				continue // A tool-only turn without native tool calls carries nothing
			}
			result = append(result, openai.AssistantMessage(msg.Content))

		case "tool":
			// Native: the result answers a specific tool call
			if nativeTools && msg.ToolCallID != "" {
				result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
				continue
			}
			result = append(result, openai.UserMessage(msg.Content))

		default: // "user" and anything unknown
			result = append(result, openai.UserMessage(msg.Content))
		}
	}

	return result
}
