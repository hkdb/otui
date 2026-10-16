package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/model"
)

// sseServer replays chat completion chunks as a server-sent event stream
func sseServer(t *testing.T, chunks []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func streamChunk(id string, delta map[string]any, finish any) map[string]any {
	return map[string]any{"id": id, "object": "chat.completion.chunk", "model": "m",
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
}

// toolDelta is a tool call delta; name is only sent with the first delta
func toolDelta(index int, name, args string) map[string]any {
	fn := map[string]any{"arguments": args}
	call := map[string]any{"index": index, "function": fn}
	if name != "" {
		call["id"] = fmt.Sprintf("call_%d", index)
		call["type"] = "function"
		fn["name"] = name
	}
	return call
}

func calls(deltas ...map[string]any) map[string]any {
	list := make([]any, len(deltas))
	for i, d := range deltas {
		list[i] = d
	}
	return map[string]any{"tool_calls": list}
}

func with(delta map[string]any, extra map[string]any) map[string]any {
	for k, v := range extra {
		delta[k] = v
	}
	return delta
}

type streamResult struct {
	content string
	calls   []string
}

func runStream(t *testing.T, providerID string, chunks []map[string]any) streamResult {
	t.Helper()
	srv := sseServer(t, chunks)
	defer srv.Close()

	var p model.Provider
	var err error
	switch providerID {
	case "openai-compatible":
		p, err = NewOpenAICompatibleProvider(srv.URL+"/v1", "", "m")
	case "openrouter":
		p, err = NewOpenRouterProvider(srv.URL+"/v1", "key", "m")
	}
	if err != nil {
		t.Fatal(err)
	}

	var res streamResult
	var content strings.Builder
	err = p.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "hi"}},
		[]mcptypes.Tool{{Name: "fake-echo.echo_a"}}, func(chunk string, toolCalls []model.ToolCall) error {
			content.WriteString(chunk)
			for _, c := range toolCalls {
				args, _ := json.Marshal(c.Arguments)
				res.calls = append(res.calls, c.Name+string(args))
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	res.content = content.String()
	return res
}

func TestOpenAIStreamToolCalls(t *testing.T) {
	both := []string{`fake-echo.echo_a{"text":"one"}`, `fake-echo.echo_b{"text":"two"}`}
	empty := map[string]any{"content": ""}

	tests := []struct {
		name   string
		chunks []map[string]any
		want   []string
	}{
		{
			name: "parallel calls, finish in separate chunk",
			chunks: []map[string]any{
				streamChunk("c", with(calls(toolDelta(0, "fake-echo__echo_a", "")), map[string]any{"role": "assistant"}), nil),
				streamChunk("c", calls(toolDelta(0, "", `{"text":"one"}`)), nil),
				streamChunk("c", calls(toolDelta(1, "fake-echo__echo_b", "")), nil),
				streamChunk("c", calls(toolDelta(1, "", `{"text":"two"}`)), nil),
				streamChunk("c", map[string]any{}, "tool_calls"),
			},
			want: both,
		},
		{
			name: `"content": "" alongside tool calls`,
			chunks: []map[string]any{
				streamChunk("c", with(calls(toolDelta(0, "fake-echo__echo_a", "")), map[string]any{"role": "assistant", "content": ""}), nil),
				streamChunk("c", with(calls(toolDelta(0, "", `{"text":"one"}`)), empty), nil),
				streamChunk("c", with(calls(toolDelta(1, "fake-echo__echo_b", "")), empty), nil),
				streamChunk("c", with(calls(toolDelta(1, "", `{"text":"two"}`)), empty), nil),
				streamChunk("c", map[string]any{}, "tool_calls"),
			},
			want: both,
		},
		{
			name: "finish_reason on the last tool delta",
			chunks: []map[string]any{
				streamChunk("c", calls(toolDelta(0, "fake-echo__echo_a", `{"text":"one"}`)), nil),
				streamChunk("c", calls(toolDelta(1, "fake-echo__echo_b", "")), nil),
				streamChunk("c", calls(toolDelta(1, "", `{"text":"two"}`)), "tool_calls"),
			},
			want: both,
		},
		{
			name: "usage-only chunk after the calls",
			chunks: []map[string]any{
				streamChunk("c", calls(toolDelta(0, "fake-echo__echo_a", `{"text":"one"}`), toolDelta(1, "fake-echo__echo_b", `{"text":"two"}`)), "tool_calls"),
				{"id": "c", "object": "chat.completion.chunk", "model": "m", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}},
			},
			want: both,
		},
		{
			name: "chunk id changes mid-stream",
			chunks: []map[string]any{
				streamChunk("c1", calls(toolDelta(0, "fake-echo__echo_a", "")), nil),
				streamChunk("c2", calls(toolDelta(0, "", `{"text":"one"}`)), nil),
				streamChunk("c2", calls(toolDelta(1, "fake-echo__echo_b", `{"text":"two"}`)), "tool_calls"),
			},
			want: both,
		},
		{
			name: "arguments split over many deltas",
			chunks: []map[string]any{
				streamChunk("c", calls(toolDelta(0, "fake-echo__echo_a", `{"te`)), nil),
				streamChunk("c", calls(toolDelta(0, "", `xt":"o`)), nil),
				streamChunk("c", calls(toolDelta(0, "", `ne"}`)), "tool_calls"),
			},
			want: []string{`fake-echo.echo_a{"text":"one"}`},
		},
	}

	for _, providerID := range []string{"openai-compatible", "openrouter"} {
		for _, tt := range tests {
			t.Run(providerID+"/"+tt.name, func(t *testing.T) {
				got := runStream(t, providerID, tt.chunks).calls
				if strings.Join(got, "|") != strings.Join(tt.want, "|") {
					t.Errorf("tool calls = %v, want %v", got, tt.want)
				}
			})
		}
	}
}

func TestOpenAIStreamContent(t *testing.T) {
	res := runStream(t, "openai-compatible", []map[string]any{
		streamChunk("c", map[string]any{"role": "assistant", "content": "Hello"}, nil),
		streamChunk("c", map[string]any{"content": " world"}, nil),
		streamChunk("c", map[string]any{}, "stop"),
	})
	if res.content != "Hello world" || len(res.calls) != 0 {
		t.Errorf("content=%q calls=%v", res.content, res.calls)
	}
}

func TestOpenAIStreamLeakedToolCallFallback(t *testing.T) {
	res := runStream(t, "openai-compatible", []map[string]any{
		streamChunk("c", map[string]any{"content": `{"name": "fake-echo.echo_a", "arguments": {"text": "one"}}`}, nil),
		streamChunk("c", map[string]any{}, "stop"),
	})
	if len(res.calls) == 0 || !strings.HasPrefix(res.calls[0], "fake-echo.echo_a") {
		t.Errorf("leaked tool call not detected: %v", res.calls)
	}
}
