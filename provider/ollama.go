package provider

import (
	"context"
	"fmt"
	"otui/mcp"
	"otui/model"
	"otui/ollama"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
)

// OllamaProvider wraps the existing ollama.Client to implement the Provider interface.
//
// This provider handles all type conversions between OTUI's provider-agnostic types
// and Ollama's specific API types. It converts model.Message to api.Message,
// mcptypes.Tool to api.Tool, and api.ToolCall to provider.ToolCall.
type OllamaProvider struct {
	client *ollama.Client
}

// NewOllamaProvider creates a new Ollama provider instance.
//
// Parameters:
//   - baseURL: The Ollama server URL (e.g., "http://localhost:11434").
//     If empty, defaults to "http://localhost:11434".
//   - model: The model name to use (e.g., "llama3.1:latest").
//     If empty, defaults to "llama3.1:latest".
//
// Returns an error if the baseURL is invalid or the Ollama client cannot be created.
//
// Example:
//
//	provider, err := NewOllamaProvider("http://localhost:11434", "llama3.1")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Use provider for chat operations
func NewOllamaProvider(baseURL, model string) (*OllamaProvider, error) {
	client, err := ollama.NewClient(baseURL, model)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}

	return &OllamaProvider{
		client: client,
	}, nil
}

// Chat implements Provider.Chat by converting messages and wrapping the callback.
//
// This method converts OTUI model.Message to Ollama api.Message, sends the chat
// request to Ollama, and streams responses back through the callback. It internally
// calls ChatWithTools with no tools.
//
// The callback is invoked for each chunk of the streamed response. Tool calls will
// be nil since this method doesn't support tools.
//
// Example:
//
//	messages := []model.Message{
//	    {Role: "user", Content: "Hello!"},
//	}
//	err := provider.Chat(ctx, messages, func(chunk string, tools []ToolCall) error {
//	    fmt.Print(chunk)
//	    return nil
//	})
func (p *OllamaProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return p.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools implements Provider.ChatWithTools with type conversions.
//
// This method handles all necessary type conversions:
//   - Converts model.Message to api.Message (OTUI → Ollama messages)
//   - Converts mcptypes.Tool to api.Tool (MCP → Ollama tools)
//   - Converts api.ToolCall to provider.ToolCall (Ollama → provider tool calls)
//
// The response is streamed back through the callback, which receives both text chunks
// and any tool calls requested by the model. Tool calls are converted to provider-agnostic
// format before being passed to the callback.
//
// Example:
//
//	messages := []model.Message{{Role: "user", Content: "What's the weather?"}}
//	tools := []mcptypes.Tool{weatherTool} // MCP tool definition
//	err := provider.ChatWithTools(ctx, messages, tools, func(chunk string, toolCalls []ToolCall) error {
//	    if len(toolCalls) > 0 {
//	        // Handle tool calls
//	        for _, call := range toolCalls {
//	            fmt.Printf("Tool: %s, Args: %v\n", call.Name, call.Arguments)
//	        }
//	    }
//	    fmt.Print(chunk)
//	    return nil
//	})
func (p *OllamaProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Convert OTUI messages to Ollama messages
	ollamaMessages := ConvertToOllamaMessages(messages)

	// Convert MCP tools to Ollama tools (if any)
	var ollamaTools []api.Tool
	if len(tools) > 0 {
		ollamaTools = mcp.ConvertMCPToolsToOllama(tools)
	}

	// Wrap the provider callback to convert Ollama tool calls
	ollamaCallback := func(chunk string, ollamaCalls []api.ToolCall) error {
		if callback == nil {
			return nil
		}

		// Convert Ollama tool calls to provider-agnostic tool calls
		providerCalls := ConvertToProviderToolCalls(ollamaCalls)
		return callback(chunk, providerCalls)
	}

	return p.client.ChatWithTools(ctx, ollamaMessages, ollamaTools, ollamaCallback)
}

// ListModels implements Provider.ListModels (direct passthrough).
//
// Returns a list of all models available on the Ollama server. This is a direct
// passthrough to the underlying ollama.Client with no type conversions needed.
//
// Example:
//
//	models, err := provider.ListModels(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, model := range models {
//	    fmt.Printf("%s (%d bytes)\n", model.Name, model.Size)
//	}
func (p *OllamaProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	return p.client.ListModels(ctx)
}

// GetModel implements Provider.GetModel (direct passthrough).
//
// Returns the name of the currently selected model. This is a direct passthrough
// to the underlying ollama.Client.
func (p *OllamaProvider) GetModel() string {
	return p.client.GetModel()
}

// GetDisplayName implements Provider.GetDisplayName.
//
// For Ollama, the display name is the same as the model name (no vendor prefix).
// This is a direct passthrough to GetModel().
func (p *OllamaProvider) GetDisplayName() string {
	return p.client.GetModel()
}

// SetModel implements Provider.SetModel (direct passthrough).
//
// Changes the active model for subsequent chat operations. This is a direct
// passthrough to the underlying ollama.Client.
//
// Example:
//
//	provider.SetModel("llama3.2:latest")
//	// Future chat calls will use llama3.2
func (p *OllamaProvider) SetModel(model string) {
	p.client.SetModel(model)
}

// Ping implements Provider.Ping (direct passthrough).
//
// Checks if the Ollama server is reachable by making a lightweight API call.
// This is useful for connection testing and health checks. This is a direct
// passthrough to the underlying ollama.Client.
//
// Returns an error if the server is not reachable or times out.
//
// Example:
//
//	err := provider.Ping(ctx)
//	if err != nil {
//	    log.Println("Ollama server is not available:", err)
//	}
func (p *OllamaProvider) Ping(ctx context.Context) error {
	return p.client.Ping(ctx)
}
