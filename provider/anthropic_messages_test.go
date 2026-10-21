package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/model"
)

// texts returns the text blocks of each converted message
func texts(msgs []anthropic.MessageParam) []string {
	var out []string
	for _, m := range msgs {
		var parts []string
		for _, b := range m.Content {
			if b.OfText != nil {
				parts = append(parts, b.OfText.Text)
			}
		}
		out = append(out, string(m.Role)+":"+strings.Join(parts, "|"))
	}
	return out
}

func TestConvertToAnthropicMessages(t *testing.T) {
	tests := []struct {
		name       string
		messages   []model.Message
		want       []string
		wantSystem int
	}{
		{
			name: "tool-only assistant turn and several results",
			messages: []model.Message{
				{Role: "user", Content: "read a and b"},
				{Role: "assistant", Content: ""},
				{Role: "tool", Content: "r1"},
				{Role: "tool", Content: "r2"},
			},
			want: []string{"user:read a and b|r1|r2"},
		},
		{
			name: "whitespace-only content dropped",
			messages: []model.Message{
				{Role: "system", Content: "  \n"},
				{Role: "system", Content: "be brief"},
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: " \t"},
				{Role: "user", Content: "again"},
			},
			want:       []string{"user:hi|again"},
			wantSystem: 1,
		},
		{
			name: "alternating turns unchanged",
			messages: []model.Message{
				{Role: "user", Content: "q1"},
				{Role: "assistant", Content: "a1"},
				{Role: "user", Content: "q2"},
			},
			want: []string{"user:q1", "assistant:a1", "user:q2"},
		},
		{
			name: "final assistant turn trimmed",
			messages: []model.Message{
				{Role: "user", Content: "q"},
				{Role: "assistant", Content: "partial answer  \n"},
			},
			want: []string{"user:q", "assistant:partial answer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs, system := convertToAnthropicMessages(tt.messages, false)
			if got := texts(msgs); strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("messages = %q, want %q", got, tt.want)
			}
			if len(system) != tt.wantSystem {
				t.Errorf("system blocks = %d, want %d", len(system), tt.wantSystem)
			}
		})
	}
}

// fakeAnthropic answers like the Messages API: 400 for empty text blocks, a
// streamed tool_use reply when tools are offered, text otherwise
func fakeAnthropic(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // Models API: not known here (fallback limits)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"type":"error","error":{"type":"not_found_error","message":"model not found"}}`)
			return
		}
		var req struct {
			Messages []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
			Tools []any `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			for _, b := range m.Content {
				if b.Type == "text" && strings.TrimSpace(b.Text) == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"messages: text content blocks must be non-empty"}}`)
					return
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		ev := func(name string, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		ev("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		lastMsg := req.Messages[len(req.Messages)-1].Content
		lastText := lastMsg[len(lastMsg)-1].Text
		if len(req.Tools) > 0 && strings.Contains(lastText, "please use both tools") {
			ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"fake-echo__echo_a","input":{}}}`)
			ev("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"text\": \"one\"}"}}`)
			ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
			ev("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
		} else {
			ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			ev("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"All done."}}`)
			ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
			ev("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
		}
		ev("message_stop", `{"type":"message_stop"}`)
	}))
}

func TestAnthropicToolOnlyReplyFollowUp(t *testing.T) {
	srv := fakeAnthropic(t)
	defer srv.Close()

	p, err := NewAnthropicProvider(srv.URL, "key", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	tools := []mcptypes.Tool{{Name: "fake-echo.echo_a"}}
	question := []model.Message{{Role: "user", Content: "please use both tools"}}

	// 1. Tool-only reply: the tool call is reported, no text
	var calls []model.ToolCall
	var text strings.Builder
	err = p.ChatWithTools(context.Background(), question, tools, func(chunk string, tc []model.ToolCall) error {
		text.WriteString(chunk)
		calls = append(calls, tc...)
		return nil
	})
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "fake-echo.echo_a" || calls[0].Arguments["text"] != "one" {
		t.Fatalf("tool calls = %+v", calls)
	}

	// 2. Follow-up as the tool loop builds it: empty assistant turn + result
	followUp := append(question,
		model.Message{Role: "assistant", Content: text.String()},
		model.Message{Role: "tool", Content: `[{"type":"text","text":"echo_a says one"}]`},
	)
	var answer strings.Builder
	err = p.ChatWithTools(context.Background(), followUp, tools, func(chunk string, _ []model.ToolCall) error {
		answer.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("follow-up rejected: %v", err)
	}
	if answer.String() != "All done." {
		t.Errorf("answer = %q", answer.String())
	}
}
