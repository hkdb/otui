package testutil

import (
	"context"
	"otui/model"
	"otui/ollama"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// MockProvider implements provider.Provider interface for testing
type MockProvider struct {
	// Configurable responses
	ChatFunc          func(ctx context.Context, messages []model.Message, callback model.StreamCallback) error
	ChatWithToolsFunc func(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error
	ListModelsFunc    func(ctx context.Context) ([]ollama.ModelInfo, error)
	PingFunc          func(ctx context.Context) error

	// State
	currentModel string
}

// NewMockProvider creates a mock provider with default implementations
func NewMockProvider(modelName string) *MockProvider {
	mock := &MockProvider{
		currentModel: modelName,
	}
	mock.ChatFunc = mock.defaultChat
	mock.ChatWithToolsFunc = mock.defaultChatWithTools
	mock.ListModelsFunc = mock.defaultListModels
	mock.PingFunc = mock.defaultPing
	return mock
}

func (m *MockProvider) defaultChat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	// Default: echo back a mock response
	if len(messages) > 0 {
		return callback("Mock response", nil)
	}
	return nil
}

func (m *MockProvider) defaultChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	// Default: mock response with tools available
	return callback("Mock response with tools", nil)
}

func (m *MockProvider) defaultListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	return []ollama.ModelInfo{
		{Name: "mock-model-1", Size: 1000},
		{Name: "mock-model-2", Size: 2000},
	}, nil
}

func (m *MockProvider) defaultPing(ctx context.Context) error {
	return nil
}

func (m *MockProvider) Chat(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
	return m.ChatFunc(ctx, messages, callback)
}

func (m *MockProvider) ChatWithTools(ctx context.Context, messages []model.Message, tools []mcptypes.Tool, callback model.StreamCallback) error {
	return m.ChatWithToolsFunc(ctx, messages, tools, callback)
}

func (m *MockProvider) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	return m.ListModelsFunc(ctx)
}

func (m *MockProvider) GetModel() string {
	return m.currentModel
}

func (m *MockProvider) GetDisplayName() string {
	// Mock provider returns same value as GetModel (no prefix stripping)
	return m.currentModel
}

func (m *MockProvider) SetModel(model string) {
	m.currentModel = model
}

func (m *MockProvider) Ping(ctx context.Context) error {
	return m.PingFunc(ctx)
}
