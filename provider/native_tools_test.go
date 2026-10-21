package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"otui/model"
)

// toolStep is a conversation with one tool step as the tool loop builds it
func toolStep() []model.Message {
	return []model.Message{
		{Role: "user", Content: "read a and b"},
		{Role: "assistant", Content: "", ToolCalls: []model.ToolCall{
			{ID: "call_1", Name: "fs.read_file", Arguments: map[string]any{"path": "a"}},
			{ID: "call_2", Name: "fs.read_file", Arguments: map[string]any{"path": "b"}},
		}},
		{Role: "tool", Content: "A", ToolCallID: "call_1", ToolName: "fs.read_file"},
		{Role: "tool", Content: "B", ToolCallID: "call_2", ToolName: "fs.read_file"},
	}
}

func TestConvertToOpenAIMessagesNative(t *testing.T) {
	data, err := json.Marshal(ConvertToOpenAIMessages(toolStep(), true))
	if err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	json.Unmarshal(data, &msgs)

	if len(msgs) != 4 {
		t.Fatalf("got %d messages: %s", len(msgs), data)
	}
	assistant := msgs[1]
	if _, hasContent := assistant["content"]; hasContent {
		t.Errorf("empty assistant content should be omitted: %v", assistant)
	}
	calls := assistant["tool_calls"].([]any)
	first := calls[0].(map[string]any)
	fn := first["function"].(map[string]any)
	if len(calls) != 2 || first["id"] != "call_1" || fn["name"] != "fs__read_file" || fn["arguments"] != `{"path":"a"}` {
		t.Errorf("assistant tool_calls = %v", calls)
	}
	for i, id := range []string{"call_1", "call_2"} {
		if msgs[2+i]["role"] != "tool" || msgs[2+i]["tool_call_id"] != id {
			t.Errorf("message %d = %v, want tool result for %s", 2+i, msgs[2+i], id)
		}
	}
}

func TestConvertToOpenAIMessagesWithoutTools(t *testing.T) {
	data, _ := json.Marshal(ConvertToOpenAIMessages(toolStep(), false))
	if strings.Contains(string(data), "tool_call") {
		t.Errorf("tool calls sent although the request has no tools: %s", data)
	}
	var msgs []map[string]any
	json.Unmarshal(data, &msgs)
	// user, (empty assistant dropped), results as user text
	if len(msgs) != 3 || msgs[1]["role"] != "user" || msgs[2]["role"] != "user" {
		t.Errorf("fallback messages = %s", data)
	}
}

func TestConvertToAnthropicMessagesNative(t *testing.T) {
	msgs, _ := convertToAnthropicMessages(append(toolStep(), model.Message{Role: "user", Content: "thanks"}), true)
	if len(msgs) != 3 {
		t.Fatalf("got %d turns, want user, assistant(tool_use), user(tool_result...)", len(msgs))
	}

	assistant := msgs[1].Content
	if len(assistant) != 2 || assistant[0].OfToolUse == nil || assistant[0].OfToolUse.ID != "call_1" ||
		assistant[0].OfToolUse.Name != "fs__read_file" || assistant[1].OfToolUse.ID != "call_2" {
		t.Errorf("assistant blocks = %+v", assistant)
	}

	results := msgs[2].Content
	if len(results) != 3 || results[0].OfToolResult == nil || results[0].OfToolResult.ToolUseID != "call_1" ||
		results[1].OfToolResult.ToolUseID != "call_2" || results[2].OfText == nil {
		t.Errorf("result turn should be tool_result, tool_result, then text: %+v", results)
	}
}

func TestConvertToOllamaMessagesNative(t *testing.T) {
	msgs := ConvertToOllamaMessages(toolStep())
	if len(msgs[1].ToolCalls) != 2 || msgs[1].ToolCalls[1].Function.Name != "fs.read_file" ||
		msgs[1].ToolCalls[1].Function.Arguments["path"] != "b" {
		t.Errorf("assistant tool calls = %+v", msgs[1].ToolCalls)
	}
	if msgs[2].Role != "tool" || msgs[2].ToolName != "fs.read_file" {
		t.Errorf("tool result = %+v", msgs[2])
	}
}

func TestStreamedToolCallIDs(t *testing.T) {
	srv := sseServer(t, []map[string]any{
		streamChunk("c", calls(toolDelta(0, "fake-echo__echo_a", `{}`)), "tool_calls"),
	})
	defer srv.Close()
	p, _ := NewOpenAICompatibleProvider(srv.URL+"/v1", "", "m")
	var got []model.ToolCall
	p.ChatWithTools(t.Context(), []model.Message{{Role: "user", Content: "hi"}}, nil, func(_ string, c []model.ToolCall) error {
		got = append(got, c...)
		return nil
	})
	if len(got) != 1 || got[0].ID != "call_0" {
		t.Errorf("tool call ID = %+v, want call_0", got)
	}
}
