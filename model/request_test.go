package model

import (
	"testing"
)

func TestBeginRequestCancelsPrevious(t *testing.T) {
	m := &Model{}
	ctx1, id1 := m.beginRequest()
	ctx2, id2 := m.beginRequest()

	if ctx1.Err() == nil {
		t.Error("previous request context not cancelled")
	}
	if ctx2.Err() != nil {
		t.Error("new request context cancelled")
	}
	if id1 == id2 || m.RequestID() != id2 {
		t.Errorf("ids: first %d, second %d, current %d", id1, id2, m.RequestID())
	}

	m.CancelRequest()
	if ctx2.Err() == nil {
		t.Error("CancelRequest didn't cancel the context")
	}
	if m.RequestID() == id2 {
		t.Error("CancelRequest didn't make the request stale")
	}
}

func TestCancelledRequestRunsNoTools(t *testing.T) {
	m := toolCallModel(&toolCallProvider{})
	m.Config.RequireApproval = false // would execute the tools (nil MCP manager panics)
	_, id := m.beginRequest()
	msg := ToolCallsDetectedMsg{
		ToolCalls: []ToolCall{{Name: "fs.read_file", Arguments: map[string]any{"path": "a"}}},
		RequestID: id,
	}

	// Cancelled before the step starts: no command at all
	cmd := m.ExecuteToolsAndContinue(msg)
	m.CancelRequest()
	if m.ExecuteToolsAndContinue(msg) != nil {
		t.Error("cancelled request still returned a tool command")
	}

	// Cancelled while the step was queued: the command stops before any tool
	result, ok := cmd().(ToolExecutionErrorMsg)
	if !ok || result.RequestID != id {
		t.Fatalf("got %#v, want ToolExecutionErrorMsg for request %d", result, id)
	}
}

func TestSendToOllamaTagsRequest(t *testing.T) {
	m := toolCallModel(&toolCallProvider{text: "hi"})
	cmd := m.SendToOllama()
	msg, ok := cmd().(StreamChunksCollectedMsg)
	if !ok || msg.RequestID != m.RequestID() {
		t.Fatalf("got %#v, want StreamChunksCollectedMsg for request %d", msg, m.RequestID())
	}
}
