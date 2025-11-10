package provider_test

import (
	"context"
	"fmt"
	"log"
	"otui/model"
	"otui/provider"
)

// ExampleNewProvider demonstrates creating an Ollama provider using the factory.
func ExampleNewProvider() {
	cfg := provider.Config{
		Type:    provider.ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
		Model:   "llama3.1",
	}

	p, err := provider.NewProvider(cfg)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Provider created: %T\n", p)
	// Output: Provider created: *provider.OllamaProvider
}

// ExampleNewOllamaProvider demonstrates creating an Ollama provider directly.
func ExampleNewOllamaProvider() {
	p, err := provider.NewOllamaProvider("http://localhost:11434", "llama3.1")
	if err != nil {
		log.Fatal(err)
	}

	// Check current model
	currentModel := p.GetModel()
	fmt.Printf("Current model: %s\n", currentModel)

	// Change model
	p.SetModel("llama3.2:latest")
	fmt.Printf("New model: %s\n", p.GetModel())

	// Output:
	// Current model: llama3.1
	// New model: llama3.2:latest
}

// ExampleOllamaProvider_Chat demonstrates basic chat without tools.
//
// Note: This example doesn't actually run because it requires a live Ollama server.
// It's provided for documentation purposes.
func ExampleOllamaProvider_Chat() {
	// Create provider
	p, err := provider.NewOllamaProvider("http://localhost:11434", "llama3.1")
	if err != nil {
		log.Fatal(err)
	}

	// Prepare messages
	messages := []model.Message{
		{Role: "user", Content: "Hello! How are you?"},
	}

	// Chat with streaming callback
	ctx := context.Background()
	err = p.Chat(ctx, messages, func(chunk string, toolCalls []model.ToolCall) error {
		// Print each chunk as it arrives
		fmt.Print(chunk)
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

// ExampleOllamaProvider_ChatWithTools demonstrates chat with tool calling.
//
// Note: This example doesn't actually run because it requires a live Ollama server
// and MCP tools setup. It's provided for documentation purposes.
func ExampleOllamaProvider_ChatWithTools() {
	// Create provider
	p, err := provider.NewOllamaProvider("http://localhost:11434", "llama3.1")
	if err != nil {
		log.Fatal(err)
	}

	// Prepare messages
	messages := []model.Message{
		{Role: "user", Content: "What's the weather in San Francisco?"},
	}

	// Example tool definition (would come from MCP in real usage)
	// tools := []mcptypes.Tool{weatherTool}

	// Chat with tools and streaming callback
	ctx := context.Background()
	err = p.ChatWithTools(ctx, messages, nil, func(chunk string, toolCalls []model.ToolCall) error {
		// Handle tool calls
		if len(toolCalls) > 0 {
			for _, call := range toolCalls {
				fmt.Printf("\nTool called: %s\n", call.Name)
				fmt.Printf("Arguments: %v\n", call.Arguments)
				// In real usage, you'd execute the tool and send results back
			}
			return nil
		}

		// Print text chunks
		fmt.Print(chunk)
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

// ExampleOllamaProvider_ListModels demonstrates listing available models.
//
// Note: This example doesn't actually run because it requires a live Ollama server.
// It's provided for documentation purposes.
func ExampleOllamaProvider_ListModels() {
	// Create provider
	p, err := provider.NewOllamaProvider("http://localhost:11434", "llama3.1")
	if err != nil {
		log.Fatal(err)
	}

	// List available models
	ctx := context.Background()
	models, err := p.ListModels(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Print model information
	for _, model := range models {
		fmt.Printf("%s (%d bytes)\n", model.Name, model.Size)
	}
}

// ExampleOllamaProvider_Ping demonstrates checking server connectivity.
//
// Note: This example doesn't actually run because it requires a live Ollama server.
// It's provided for documentation purposes.
func ExampleOllamaProvider_Ping() {
	// Create provider
	p, err := provider.NewOllamaProvider("http://localhost:11434", "llama3.1")
	if err != nil {
		log.Fatal(err)
	}

	// Check if server is reachable
	ctx := context.Background()
	err = p.Ping(ctx)
	if err != nil {
		fmt.Println("Ollama server is not available:", err)
		return
	}

	fmt.Println("Ollama server is reachable")
}

// ExampleConfig demonstrates different provider configurations.
func ExampleConfig() {
	// Ollama configuration (local server)
	ollamaCfg := provider.Config{
		Type:    provider.ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
		Model:   "llama3.1",
		// APIKey is not used for Ollama
	}

	// OpenAI configuration (not yet implemented)
	openaiCfg := provider.Config{
		Type:    provider.ProviderTypeOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4",
		APIKey:  "sk-...", // Your OpenAI API key
	}

	// Anthropic configuration (not yet implemented)
	anthropicCfg := provider.Config{
		Type:    provider.ProviderTypeAnthropic,
		BaseURL: "https://api.anthropic.com/v1",
		Model:   "claude-3-opus",
		APIKey:  "sk-ant-...", // Your Anthropic API key
	}

	fmt.Printf("Ollama: %s\n", ollamaCfg.Type)
	fmt.Printf("OpenAI: %s\n", openaiCfg.Type)
	fmt.Printf("Anthropic: %s\n", anthropicCfg.Type)

	// Output:
	// Ollama: ollama
	// OpenAI: openai
	// Anthropic: anthropic
}
