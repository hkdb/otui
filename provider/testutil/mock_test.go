package testutil

import (
	"context"
	"errors"
	"testing"

	"otui/model"
)

func TestNewMockProvider_Defaults(t *testing.T) {
	mock := NewMockProvider("test-model")

	t.Run("GetModel", func(t *testing.T) {
		if mock.GetModel() != "test-model" {
			t.Errorf("expected 'test-model', got %q", mock.GetModel())
		}
	})

	t.Run("GetDisplayName", func(t *testing.T) {
		if mock.GetDisplayName() != "test-model" {
			t.Errorf("expected 'test-model', got %q", mock.GetDisplayName())
		}
	})

	t.Run("Ping", func(t *testing.T) {
		err := mock.Ping(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("ListModels", func(t *testing.T) {
		models, err := mock.ListModels(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 {
			t.Errorf("expected 2 models, got %d", len(models))
		}
	})

	t.Run("SetModel", func(t *testing.T) {
		mock.SetModel("new-model")
		if mock.GetModel() != "new-model" {
			t.Errorf("expected 'new-model', got %q", mock.GetModel())
		}
	})

	t.Run("GetModelMetadata", func(t *testing.T) {
		meta, err := mock.GetModelMetadata(context.Background(), "any")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if meta.ContextWindow != 4096 {
			t.Errorf("expected ContextWindow 4096, got %d", meta.ContextWindow)
		}
	})
}

func TestMockProvider_CustomFunc(t *testing.T) {
	mock := NewMockProvider("test")
	called := false
	expectedErr := errors.New("custom error")

	mock.ChatFunc = func(ctx context.Context, messages []model.Message, callback model.StreamCallback) error {
		called = true
		return expectedErr
	}

	err := mock.Chat(context.Background(), []model.Message{{Content: "hi"}}, nil)
	if !called {
		t.Error("custom ChatFunc was not called")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected custom error, got %v", err)
	}
}

func TestMockProvider_DefaultChat(t *testing.T) {
	mock := NewMockProvider("test")
	var received string

	callback := func(chunk string, toolCalls []model.ToolCall) error {
		received = chunk
		return nil
	}

	messages := []model.Message{{Role: "user", Content: "hello"}}
	err := mock.Chat(context.Background(), messages, callback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "Mock response" {
		t.Errorf("expected 'Mock response', got %q", received)
	}
}
