package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3"

	"otui/mcp"
	"otui/model"
)

// streamOpenAIChat runs a streaming chat completion for the OpenAI, OpenAI
// compatible and OpenRouter providers. Content deltas go to callback as they
// arrive. Tool calls are assembled from their deltas and reported once, in
// index order, when the stream ends.
//
// openai-go's ChatCompletionAccumulator.JustFinishedToolCall isn't used: it
// misses calls when a server sends "content": "" alongside tool_calls, drops
// the last call when its final delta carries finish_reason, and stops
// accumulating if chunk IDs change mid-stream.
//
// Returns the streamed content and the API tool calls so providers can fall
// back to parsing tool calls that leaked into the text.
func streamOpenAIChat(ctx context.Context, client openai.Client, params openai.ChatCompletionNewParams,
	providerName string, callback model.StreamCallback) (string, []model.ToolCall, error) {

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var content strings.Builder
	calls := make(map[int64]*streamedToolCall)

	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				continue // Only one choice is requested
			}

			if text := choice.Delta.Content; text != "" {
				content.WriteString(text)
				if callback != nil {
					if err := callback(text, nil); err != nil {
						return content.String(), nil, err
					}
				}
			}

			for _, delta := range choice.Delta.ToolCalls {
				call, ok := calls[delta.Index]
				if !ok {
					call = &streamedToolCall{}
					calls[delta.Index] = call
				}
				call.name.WriteString(delta.Function.Name)
				call.arguments.WriteString(delta.Function.Arguments)
			}
		}
	}

	if err := stream.Err(); err != nil {
		return content.String(), nil, fmt.Errorf("%s streaming error: %w", providerName, err)
	}

	toolCalls := assembleToolCalls(calls)
	if len(toolCalls) > 0 && callback != nil {
		if err := callback("", toolCalls); err != nil {
			return content.String(), toolCalls, err
		}
	}

	return content.String(), toolCalls, nil
}

// streamedToolCall collects one tool call's deltas
type streamedToolCall struct {
	name      strings.Builder
	arguments strings.Builder
}

// assembleToolCalls converts collected tool calls to model.ToolCalls in index
// order, decoding API safe tool names
func assembleToolCalls(calls map[int64]*streamedToolCall) []model.ToolCall {
	indexes := make([]int64, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })

	var toolCalls []model.ToolCall
	for _, index := range indexes {
		call := calls[index]
		name := call.name.String()
		if name == "" {
			continue
		}
		toolCalls = append(toolCalls, model.ToolCall{
			Name:      mcp.DecodeToolName(name),
			Arguments: ParseToolArguments(call.arguments.String()),
		})
	}
	return toolCalls
}
