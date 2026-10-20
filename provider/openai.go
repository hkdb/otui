package provider

import (
	"context"
	"fmt"
	"otui/mcp"
	"otui/model"
	"otui/ollama"
	"sync"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIProvider implements the Provider interface using OpenAI's official API.
// It uses the official OpenAI Go SDK for direct OpenAI API access.
// The same implementation also backs the "openai-compatible" provider
// (llama.cpp, vLLM, LM Studio, etc.), which only differs in provider ID,
// endpoint and an optional API key.
type OpenAIProvider struct {
	client     openai.Client
	model      string
	modelMu    sync.RWMutex // SetModel (Update) vs requests (Cmd goroutines)
	baseURL    string
	apiKey     string
	providerID string // "openai" or "openai-compatible"

	// Discovers context windows on OpenAI compatible servers (nil for OpenAI)
	prober *contextProber
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
		client:     client,
		model:      model,
		baseURL:    baseURL,
		apiKey:     apiKey,
		providerID: string(ProviderTypeOpenAI),
	}, nil
}

// NewOpenAICompatibleProvider creates a provider for any server that implements
// the OpenAI chat completions API (llama.cpp, vLLM, LM Studio, etc.).
//
// Parameters:
//   - baseURL: API base URL including the version path (e.g. "http://localhost:8080/v1")
//   - apiKey: API key (optional - many local servers don't need one)
//   - model: Initial model to use (optional)
//
// Returns an error if the base URL is missing.
func NewOpenAICompatibleProvider(baseURL, apiKey, model string) (*OpenAIProvider, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("OpenAI compatible provider requires a base URL")
	}

	// Always set an explicit key so the SDK doesn't pick up OPENAI_API_KEY
	// from the environment and send it to a third-party server
	key := apiKey
	if key == "" {
		key = "none"
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(key),
	)

	return &OpenAIProvider{
		client:     client,
		model:      model,
		baseURL:    baseURL,
		apiKey:     apiKey,
		providerID: string(ProviderTypeOpenAICompatible),
		prober:     newContextProber(baseURL, apiKey),
	}, nil
}

// displayName returns the user-facing provider name for error messages
func (p *OpenAIProvider) displayName() string {
	if p.providerID == string(ProviderTypeOpenAICompatible) {
		return "OpenAI compatible server"
	}
	return "OpenAI"
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
			Content: buildToolInstructions(tools),
		}
		messagesWithInstructions = append([]model.Message{toolInstruction}, messages...)
	}

	// Convert OTUI messages to OpenAI format
	openaiMessages := ConvertToOpenAIMessages(messagesWithInstructions, len(tools) > 0)

	// Build request parameters
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(p.GetModel()),
	}

	// Add tools if provided
	if len(tools) > 0 {
		openaiTools := mcp.ConvertMCPToolsToOpenAIFormat(tools)
		params.Tools = openaiTools
	}

	// Stream the response (tool calls are reported once the stream ends)
	content, toolCalls, err := streamOpenAIChat(ctx, p.client, params, p.displayName(), callback)
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

// ListModels implements Provider.ListModels.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	// Fetch models from OpenAI
	modelsPage, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s models: %w", p.displayName(), err)
	}

	// Convert to ModelInfo
	result := make([]ollama.ModelInfo, 0, len(modelsPage.Data))
	for _, m := range modelsPage.Data {
		result = append(result, ollama.ModelInfo{
			Name:         m.ID,         // OpenAI models don't have vendor prefixes
			InternalName: m.ID,         // Same as display name
			Size:         0,            // OpenAI doesn't provide size info
			Provider:     p.providerID, // CRITICAL: Must match provider ID
		})
	}

	return result, nil
}

// GetModel implements Provider.GetModel.
// Returns the full model name for API calls.
func (p *OpenAIProvider) GetModel() string {
	p.modelMu.RLock()
	defer p.modelMu.RUnlock()
	return p.model
}

// GetDisplayName implements Provider.GetDisplayName.
// Returns the model name for UI display (same as GetModel for OpenAI).
func (p *OpenAIProvider) GetDisplayName() string {
	return p.GetModel()
}

// SetModel implements Provider.SetModel.
func (p *OpenAIProvider) SetModel(model string) {
	p.modelMu.Lock()
	defer p.modelMu.Unlock()
	p.model = model
}

// Ping implements Provider.Ping by attempting to list models.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	_, err := p.client.Models.List(ctx)
	if err != nil {
		return fmt.Errorf("%s ping failed: %w", p.displayName(), err)
	}
	return nil
}

// GetModelMetadata returns metadata for the specified model.
//
// OpenAI's API doesn't expose context windows, so OpenAI models use the
// hardcoded table. OpenAI compatible servers are probed for the context
// they actually run the model with (see contextProber).
func (p *OpenAIProvider) GetModelMetadata(ctx context.Context, modelName string) (model.ModelMetadata, error) {
	meta := p.FallbackModelMetadata(modelName)
	if p.prober == nil {
		return meta, nil
	}

	if contextWindow, source := p.prober.contextWindow(ctx, modelName); contextWindow > 0 {
		meta.ContextWindow = contextWindow
		meta.Source = source
		if meta.MaxOutput > contextWindow {
			meta.MaxOutput = contextWindow
		}
	}
	return meta, nil
}

// FallbackModelMetadata returns metadata from the hardcoded OpenAI table
func (p *OpenAIProvider) FallbackModelMetadata(modelName string) model.ModelMetadata {
	meta := toModelMetadata(GetFallbackMetadata(modelName, OpenAIFallbackMetadata), model.MetadataSourceFallback)

	// Models on compatible servers are arbitrary, so the fallback table can't
	// tell whether they support tools. Assume they do and let the server decide.
	// Context window can be set with [model_context_overrides].
	if p.providerID == string(ProviderTypeOpenAICompatible) {
		meta.SupportsTools = true
	}
	return meta
}
