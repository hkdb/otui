package model

import (
	"context"
	"testing"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/config"
	"otui/ollama"
	"otui/storage"
)

// metaProvider is a Provider stub that reports a fixed context window
type metaProvider struct {
	fetched  int
	fallback int
}

func (p *metaProvider) Chat(context.Context, []Message, StreamCallback) error { return nil }
func (p *metaProvider) ChatWithTools(context.Context, []Message, []mcptypes.Tool, StreamCallback) error {
	return nil
}
func (p *metaProvider) ListModels(context.Context) ([]ollama.ModelInfo, error) { return nil, nil }
func (p *metaProvider) GetModel() string                                       { return "" }
func (p *metaProvider) GetDisplayName() string                                 { return "" }
func (p *metaProvider) SetModel(string)                                        {}
func (p *metaProvider) Ping(context.Context) error                             { return nil }
func (p *metaProvider) GetModelMetadata(context.Context, string) (ModelMetadata, error) {
	return ModelMetadata{ContextWindow: p.fetched, Source: "server"}, nil
}
func (p *metaProvider) FallbackModelMetadata(string) ModelMetadata {
	return ModelMetadata{ContextWindow: p.fallback, Source: MetadataSourceFallback}
}

func newMetadataTestModel() (*Model, *metaProvider, *metaProvider) {
	ollamaProv := &metaProvider{fetched: 4096, fallback: 8192}
	compatProv := &metaProvider{fetched: 65536, fallback: 8192}
	m := &Model{
		Config: &config.Config{},
		CurrentSession: &storage.Session{
			Model:    "qwen3.6-27b",
			Provider: "openai-compatible",
		},
		// m.Provider deliberately points at a different provider than the session's
		Provider:  ollamaProv,
		Providers: map[string]Provider{"ollama": ollamaProv, "openai-compatible": compatProv},
	}
	return m, ollamaProv, compatProv
}

func runRefresh(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.RefreshModelMetadataCmd()
	if cmd == nil {
		t.Fatal("RefreshModelMetadataCmd returned nil")
	}
	msg, ok := cmd().(ModelMetadataMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}
	m.SetModelMetadata(msg)
}

func TestGetModelMetadataUsesSessionProvider(t *testing.T) {
	m, _, _ := newMetadataTestModel()

	// Before the fetch completes: provider fallback, no network
	if got := m.GetModelMetadata(); got.ContextWindow != 8192 || got.Source != MetadataSourceFallback {
		t.Errorf("before refresh = %+v, want fallback 8192", got)
	}

	runRefresh(t, m)

	// Fetched from the session's provider, not m.Provider
	if got := m.GetModelMetadata().ContextWindow; got != 65536 {
		t.Errorf("after refresh ContextWindow = %d, want 65536 from session provider", got)
	}

	// Same model name on another provider has its own cache entry
	m.CurrentSession.Provider = "ollama"
	if got := m.GetModelMetadata().ContextWindow; got != 8192 {
		t.Errorf("other provider before refresh = %d, want fallback 8192", got)
	}
}

func TestGetModelMetadataOverrideWins(t *testing.T) {
	m, _, _ := newMetadataTestModel()
	m.Config.ModelContextOverrides = map[string]int{"qwen3.6-27b": 131072}

	if cmd := m.RefreshModelMetadataCmd(); cmd != nil {
		t.Error("RefreshModelMetadataCmd should be nil when an override is set")
	}
	if got := m.GetModelMetadata().ContextWindow; got != 131072 {
		t.Errorf("ContextWindow = %d, want override 131072", got)
	}
}

func TestContextUsageUpdatesWithContextWindow(t *testing.T) {
	m, _, _ := newMetadataTestModel()
	m.Messages = []Message{
		{Role: "user", Content: string(make([]byte, 4096*4))}, // 4096 tokens
	}

	before := m.GetContextUsagePercentage() // 4096 / 8192 fallback
	if before != 0.5 {
		t.Fatalf("usage before refresh = %.3f, want 0.5", before)
	}

	runRefresh(t, m)

	// Same messages, but the real context window is known now
	after := m.GetContextUsagePercentage() // 4096 / 65536
	if after != 0.0625 {
		t.Errorf("usage after refresh = %.4f, want 0.0625", after)
	}
}

func TestClearModelMetadata(t *testing.T) {
	m, _, _ := newMetadataTestModel()
	runRefresh(t, m)
	m.ClearModelMetadata()
	if got := m.GetModelMetadata().ContextWindow; got != 8192 {
		t.Errorf("after clear ContextWindow = %d, want fallback 8192", got)
	}
}
