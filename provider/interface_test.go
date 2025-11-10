package provider_test

import (
	"context"
	"otui/model"
	"otui/provider/testutil"
	"testing"
	"time"
)

// TestProviderContract defines the contract ALL providers must satisfy.
// This test suite will be run against Ollama, OpenAI, and Anthropic.
func TestProviderContract(t *testing.T) {
	tests := []struct {
		name     string
		provider model.Provider
	}{
		{"Mock", testutil.NewMockProvider("test-model")},
		// Future: {"Ollama", newOllamaProvider()},
		// Future: {"OpenAI", newOpenAIProvider()},
		// Future: {"Anthropic", newAnthropicProvider()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("BasicChat", func(t *testing.T) {
				testProviderBasicChat(t, tt.provider)
			})
			t.Run("ChatWithTools", func(t *testing.T) {
				testProviderChatWithTools(t, tt.provider)
			})
			t.Run("ModelManagement", func(t *testing.T) {
				testProviderModelManagement(t, tt.provider)
			})
			t.Run("HealthCheck", func(t *testing.T) {
				testProviderHealthCheck(t, tt.provider)
			})
		})
	}
}

func testProviderBasicChat(t *testing.T, p model.Provider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	messages := testutil.SingleUserMessage("Hello")
	var receivedChunk string

	err := p.Chat(ctx, messages, func(chunk string, toolCalls []model.ToolCall) error {
		receivedChunk = chunk
		return nil
	})

	if err != nil {
		t.Errorf("Chat() error = %v", err)
	}

	if receivedChunk == "" {
		t.Error("Chat() did not receive any chunks")
	}
}

func testProviderChatWithTools(t *testing.T, p model.Provider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	messages := testutil.SingleUserMessage("What's the weather?")
	tools := testutil.TestMCPTools()
	var receivedChunk string

	err := p.ChatWithTools(ctx, messages, tools, func(chunk string, toolCalls []model.ToolCall) error {
		receivedChunk = chunk
		return nil
	})

	if err != nil {
		t.Errorf("ChatWithTools() error = %v", err)
	}

	if receivedChunk == "" {
		t.Error("ChatWithTools() did not receive any chunks")
	}
}

func testProviderModelManagement(t *testing.T, p model.Provider) {
	// Test GetModel
	initialModel := p.GetModel()
	if initialModel == "" {
		t.Error("GetModel() returned empty string")
	}

	// Test SetModel
	newModel := "new-test-model"
	p.SetModel(newModel)

	// Verify model was changed
	if got := p.GetModel(); got != newModel {
		t.Errorf("After SetModel(%s), GetModel() = %s, want %s", newModel, got, newModel)
	}
}

func testProviderHealthCheck(t *testing.T, p model.Provider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// TestMockProviderImplementsInterface ensures mock provider implements the interface
func TestMockProviderImplementsInterface(t *testing.T) {
	var _ model.Provider = (*testutil.MockProvider)(nil)
}
