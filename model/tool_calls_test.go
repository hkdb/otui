package model

import (
	"context"
	"testing"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/config"
	"otui/storage"
)

// toolCallProvider reports tool calls the way OpenAI-style providers do:
// one callback per call
type toolCallProvider struct {
	metaProvider
	text  string
	calls []ToolCall
}

func (p *toolCallProvider) ChatWithTools(_ context.Context, _ []Message, _ []mcptypes.Tool, callback StreamCallback) error {
	if p.text != "" {
		callback(p.text, nil)
	}
	for _, call := range p.calls {
		callback("", []ToolCall{call})
	}
	return nil
}

func toolCallModel(provider Provider) *Model {
	return &Model{
		Config:         &config.Config{RequireApproval: true},
		CurrentSession: &storage.Session{ID: "s1", Model: "m", Provider: "test"},
		Providers:      map[string]Provider{"test": provider},
		Provider:       provider,
		Messages:       []Message{{Role: "user", Content: "read both files"}},
	}
}

func TestSendToOllamaKeepsAllToolCalls(t *testing.T) {
	provider := &toolCallProvider{
		text: "Reading both.",
		calls: []ToolCall{
			{Name: "fs.read_file", Arguments: map[string]any{"path": "a"}},
			{Name: "fs.read_file", Arguments: map[string]any{"path": "b"}},
			{Name: "fs.read_file", Arguments: map[string]any{"path": "a"}}, // reported twice
		},
	}
	m := toolCallModel(provider)

	msg, ok := m.SendToOllama()().(ToolCallsDetectedMsg)
	if !ok {
		t.Fatalf("expected ToolCallsDetectedMsg")
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2 (a and b, duplicate dropped): %+v", len(msg.ToolCalls), msg.ToolCalls)
	}
	if msg.ToolCalls[0].Arguments["path"] != "a" || msg.ToolCalls[1].Arguments["path"] != "b" {
		t.Errorf("tool calls out of order: %+v", msg.ToolCalls)
	}
	if msg.InitialResponse != "Reading both." {
		t.Errorf("InitialResponse = %q", msg.InitialResponse)
	}
}

func TestPermissionRequestCarriesWholeBatch(t *testing.T) {
	m := toolCallModel(&toolCallProvider{})
	batch := []ToolCall{
		{Name: "fs.read_file", Arguments: map[string]any{"path": "a"}},
		{Name: "web.fetch", Arguments: map[string]any{"url": "x"}},
	}

	msg, ok := m.ExecuteToolsAndContinue(ToolCallsDetectedMsg{
		ToolCalls:       batch,
		InitialResponse: "Let me check.",
		ContextMessages: m.Messages,
	})().(ToolPermissionRequestMsg)
	if !ok {
		t.Fatal("expected ToolPermissionRequestMsg")
	}
	if msg.ToolName != "fs.read_file" {
		t.Errorf("asked about %q first, want fs.read_file", msg.ToolName)
	}
	if len(msg.ToolCalls) != 2 || msg.InitialResponse != "Let me check." {
		t.Errorf("permission request lost the batch: calls=%d initial=%q", len(msg.ToolCalls), msg.InitialResponse)
	}

	// After approving the first tool, the second one is asked about next
	m.CurrentSession.AllowedTools = []string{"fs.read_file"}
	next, ok := m.ExecuteToolsAndContinue(ToolCallsDetectedMsg{
		ToolCalls: msg.ToolCalls, InitialResponse: msg.InitialResponse, ContextMessages: msg.ContextMessages,
	})().(ToolPermissionRequestMsg)
	if !ok || next.ToolName != "web.fetch" {
		t.Errorf("expected permission request for web.fetch next, got %+v", next)
	}
}

func TestAppendToolCalls(t *testing.T) {
	a := ToolCall{Name: "t.x", Arguments: map[string]any{"n": 1.0}}
	b := ToolCall{Name: "t.x", Arguments: map[string]any{"n": 2.0}}
	c := ToolCall{Name: "t.y", Arguments: map[string]any{"n": 1.0}}

	got := appendToolCalls(nil, []ToolCall{a})
	got = appendToolCalls(got, []ToolCall{b, a, c})
	if len(got) != 3 {
		t.Errorf("got %d calls, want 3 (a, b, c): %+v", len(got), got)
	}
}

func TestStepPurposeNamesAllTools(t *testing.T) {
	m := toolCallModel(&toolCallProvider{})
	calls := []ToolCall{{Name: "fake-echo.echo_a"}, {Name: "fake-echo.echo_b"}}

	if got := m.StepPurpose(nil, "", calls); got != "Execute echo_a, echo_b" {
		t.Errorf("StepPurpose = %q", got)
	}
	// The LLM's text for this step is its reasoning
	if got := m.StepPurpose(nil, "Checking both. Then answering", calls); got != "Checking both" {
		t.Errorf("StepPurpose with reasoning = %q", got)
	}
}

func TestEnsureToolCallIDs(t *testing.T) {
	calls := ensureToolCallIDs([]ToolCall{{Name: "a"}, {ID: "keep", Name: "b"}, {Name: "c"}})
	if calls[1].ID != "keep" {
		t.Errorf("existing ID replaced: %q", calls[1].ID)
	}
	if calls[0].ID == "" || calls[2].ID == "" || calls[0].ID == calls[2].ID {
		t.Errorf("generated IDs = %q, %q", calls[0].ID, calls[2].ID)
	}
}

func TestBuildToolStepMessages(t *testing.T) {
	context := []Message{{Role: "user", Content: "q"}}
	calls := []ToolCall{{ID: "c1", Name: "t.a"}, {ID: "c2", Name: "t.b"}}

	msgs := buildToolStepMessages(context, "Checking.", calls, []string{"ra", "rb"})

	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want user, assistant, tool, tool", len(msgs))
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Checking." || len(msgs[1].ToolCalls) != 2 {
		t.Errorf("assistant turn = %+v", msgs[1])
	}
	if msgs[2].ToolCallID != "c1" || msgs[2].Content != "ra" || msgs[3].ToolCallID != "c2" || msgs[3].ToolName != "t.b" {
		t.Errorf("results = %+v %+v", msgs[2], msgs[3])
	}
	if len(context) != 1 {
		t.Error("context was modified")
	}

	// A follow-up step adds exactly one assistant turn (no duplicate)
	next := buildToolStepMessages(msgs, "Now B.", []ToolCall{{ID: "c3", Name: "t.c"}}, []string{"rc"})
	assistants := 0
	for _, m := range next {
		if m.Role == "assistant" {
			assistants++
		}
	}
	if assistants != 2 || next[4].Content != "Now B." {
		t.Errorf("follow-up step: %d assistant turns, step 2 turn = %+v", assistants, next[4])
	}
}
