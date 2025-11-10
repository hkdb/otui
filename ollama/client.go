package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type Client struct {
	client  *api.Client
	model   string
	baseURL string
}

type StreamCallback func(chunk string, toolCalls []api.ToolCall) error

func NewClient(baseURL, model string) (*Client, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.1:latest"
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Ollama URL: %w", err)
	}

	client := api.NewClient(parsedURL, http.DefaultClient)

	return &Client{
		client:  client,
		model:   model,
		baseURL: baseURL,
	}, nil
}

func (c *Client) Chat(ctx context.Context, messages []api.Message, callback StreamCallback) error {
	return c.ChatWithTools(ctx, messages, nil, callback)
}

// ChatWithTools sends a chat request with optional tool definitions
func (c *Client) ChatWithTools(ctx context.Context, messages []api.Message, tools []api.Tool, callback StreamCallback) error {
	req := &api.ChatRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Stream:   func(b bool) *bool { return &b }(true),
	}

	respFunc := func(resp api.ChatResponse) error {
		if callback != nil {
			// Pass tool calls if present, otherwise nil
			return callback(resp.Message.Content, resp.Message.ToolCalls)
		}
		return nil
	}

	return c.client.Chat(ctx, req, respFunc)
}

type ModelInfo struct {
	Name         string // Display name (stripped for OpenRouter)
	Size         int64
	Provider     string // Provider ID: "ollama", "openrouter", "anthropic"
	InternalName string // Full API name (e.g., "meta-llama/llama-3.2-90b" for OpenRouter)
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.client.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	models := make([]ModelInfo, len(resp.Models))
	for i, model := range resp.Models {
		models[i] = ModelInfo{
			Name:         model.Name,
			Size:         model.Size,
			Provider:     "ollama",
			InternalName: model.Name, // Ollama uses same name for display and API
		}
	}

	return models, nil
}

func (c *Client) SetModel(model string) {
	c.model = model
}

func (c *Client) GetModel() string {
	return c.model
}

func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.client.List(ctx)
	return err
}

// ModelCapabilities tracks which model families support tool calling
// This is a curated list based on Ollama documentation and community testing
var toolCallingModels = map[string]bool{
	// Known working models with full tool support
	"qwen":      true, // qwen2.5-coder, qwen3-coder
	"llama3.1":  true, // llama3.1:8b, llama3.1:latest
	"llama3.2":  true, // llama3.2:3b and above
	"mistral":   true, // mistral:latest, mistral-nemo
	"command-r": true, // Cohere models
	"nemotron":  true, // NVIDIA models
	"granite3":  true, // IBM Granite 3 models
	"llama3.3":  true, // Llama 3.3 models

	// Models with issues or no tool support
	"llama3-gradient": false,
	"llama3":          false, // Original llama3 (not 3.1/3.2/3.3)
	"phi":             false,
	"gemma":           false,
	"codellama":       false,
	"deepseek":        false, // DeepSeek v2/v3 don't support tools in Ollama
}

// orderedPrefixes defines the order to check model prefixes
// IMPORTANT: Check most specific prefixes first to avoid false matches
// (e.g., check "llama3.2" before "llama3" to avoid matching llama3.2 as generic llama3)
var orderedPrefixes = []string{
	// Specific version numbers first
	"llama3.3", "llama3.2", "llama3.1",
	// Specific variants
	"llama3-gradient",
	// Other tool-supporting models
	"command-r", "qwen", "mistral", "nemotron", "granite3",
	// Non-supporting specific models
	"codellama",
	// Generic patterns LAST
	"llama3",
	"deepseek", "phi", "gemma",
}

// SupportsToolCalling checks if the current model supports Ollama's tool calling API
// Returns true if the model is known to support tool calling, false otherwise
func (c *Client) SupportsToolCalling() bool {
	modelName := strings.ToLower(c.model)

	// Check prefixes in deterministic order (most specific first)
	for _, prefix := range orderedPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			if supported, exists := toolCallingModels[prefix]; exists {
				return supported
			}
		}
	}

	// Default: assume no support (conservative approach)
	return false
}

// ModelSupportsToolCalling is a static helper to check if a model name supports tools
// without needing a Client instance
func ModelSupportsToolCalling(modelName string) bool {
	modelName = strings.ToLower(modelName)

	// Check prefixes in deterministic order (most specific first)
	for _, prefix := range orderedPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			if supported, exists := toolCallingModels[prefix]; exists {
				return supported
			}
		}
	}

	return false
}
