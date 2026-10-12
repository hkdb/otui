package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

// authTransport injects an Authorization: Bearer header on every request.
type authTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	return t.base.RoundTrip(req)
}

func NewClient(baseURL, model, apiKey string) (*Client, error) {
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

	httpClient := http.DefaultClient
	if apiKey != "" {
		httpClient = &http.Client{
			Transport: &authTransport{
				base:   http.DefaultTransport,
				apiKey: apiKey,
			},
		}
	}

	client := api.NewClient(parsedURL, httpClient)

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

// ModelContext describes a model's context window as reported by Ollama
type ModelContext struct {
	Loaded        int  // Context the model is loaded with right now (/api/ps), 0 if not loaded
	Modelfile     int  // num_ctx set in the model's Modelfile (/api/show parameters), 0 if unset
	Trained       int  // Model's trained context length (/api/show model_info), 0 if unknown
	SupportsTools bool // Model reports the "tools" capability
	HasShowInfo   bool // /api/show succeeded (Modelfile/Trained/SupportsTools are valid)
}

// Effective returns the context window Ollama will actually use for the
// model: the loaded context, else the Modelfile num_ctx, else the trained
// length. Ollama never runs a model with more than its trained length.
// Returns 0 if unknown.
func (mc ModelContext) Effective() (int, string) {
	ctxLen, source := 0, ""
	switch {
	case mc.Loaded > 0:
		ctxLen, source = mc.Loaded, "ollama:/api/ps"
	case mc.Modelfile > 0:
		ctxLen, source = mc.Modelfile, "ollama:modelfile num_ctx"
	case mc.Trained > 0:
		ctxLen, source = mc.Trained, "ollama:model_info"
	}
	if mc.Trained > 0 && ctxLen > mc.Trained {
		ctxLen = mc.Trained
	}
	return ctxLen, source
}

// GetModelContext queries Ollama for a model's context window.
// /api/ps reports the context of loaded models (Ollama 0.10+); /api/show
// reports the trained length and any Modelfile num_ctx.
func (c *Client) GetModelContext(ctx context.Context, modelName string) (ModelContext, error) {
	var mc ModelContext

	// Context of the loaded model (absent on older servers or if not loaded)
	if running, err := c.client.ListRunning(ctx); err == nil {
		for _, m := range running.Models {
			if sameModelName(m.Name, modelName) || sameModelName(m.Model, modelName) {
				mc.Loaded = m.ContextLength
				break
			}
		}
	}

	show, err := c.client.Show(ctx, &api.ShowRequest{Model: modelName})
	if err != nil {
		if mc.Loaded > 0 {
			return mc, nil
		}
		return mc, fmt.Errorf("failed to show model %s: %w", modelName, err)
	}

	mc.HasShowInfo = true
	mc.Trained = trainedContextLength(show.ModelInfo)
	mc.Modelfile = parseNumCtx(show.Parameters)
	for _, capability := range show.Capabilities {
		if string(capability) == "tools" {
			mc.SupportsTools = true
		}
	}

	return mc, nil
}

// sameModelName compares Ollama model names, treating a missing tag as ":latest"
func sameModelName(a, b string) bool {
	normalize := func(name string) string {
		if name != "" && !strings.Contains(name[strings.LastIndex(name, "/")+1:], ":") {
			return name + ":latest"
		}
		return name
	}
	return normalize(a) == normalize(b)
}

// trainedContextLength reads "<general.architecture>.context_length" from
// /api/show model_info (e.g. "qwen3.context_length")
func trainedContextLength(modelInfo map[string]any) int {
	arch, _ := modelInfo["general.architecture"].(string)
	if arch != "" {
		if n := anyToInt(modelInfo[arch+".context_length"]); n > 0 {
			return n
		}
	}

	// Architecture key missing - fall back to any "*.context_length" key
	for key, value := range modelInfo {
		if strings.HasSuffix(key, ".context_length") {
			if n := anyToInt(value); n > 0 {
				return n
			}
		}
	}
	return 0
}

// parseNumCtx extracts num_ctx from /api/show "parameters", which lists
// Modelfile parameters one per line (e.g. "num_ctx                        32768")
func parseNumCtx(parameters string) int {
	for _, line := range strings.Split(parameters, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "num_ctx" {
			if n, err := strconv.Atoi(fields[1]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// anyToInt converts a JSON number decoded into an interface to int
func anyToInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
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
