package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/safetext"
)

// StreamTimeouts limit how long a response may go without any data from the
// provider. There's no limit on the total time (Esc cancels).
type StreamTimeouts struct {
	FirstToken time.Duration // Until the first chunk (model loading, prompt processing)
	Idle       time.Duration // Between chunks
}

// streamTimeouts returns the configured response timeouts. Call from Update.
func (m *Model) streamTimeouts() StreamTimeouts {
	return StreamTimeouts{FirstToken: m.Config.FirstTokenTimeout(), Idle: m.Config.StreamIdleTimeout()}
}

// StreamTimeoutError is returned when the provider sent nothing for too long
type StreamTimeoutError struct {
	FirstToken bool // No chunk at all (otherwise the stream went silent)
	After      time.Duration
}

func (e *StreamTimeoutError) Error() string {
	if e.FirstToken {
		return fmt.Sprintf("no response from the model after %s (first token timeout, see Settings)", e.After)
	}
	return fmt.Sprintf("the model stopped responding for %s (stream idle timeout, see Settings)", e.After)
}

// ChatStream delivers a response as it's generated. Started by
// startChatStream, read with WaitForStream: each chunk arrives in Update as a
// StreamChunkMsg, the end as the final message given to startChatStream.
type ChatStream struct {
	requestID uint64
	events    chan streamEvent
	held      *streamEvent // Read ahead while coalescing chunks (only used by the one waiting Cmd)
}

type streamEvent struct {
	text  string
	final tea.Msg // Set on the last event
}

// streamFinish builds the final message from the complete response
type streamFinish func(text string, toolCalls []ToolCall, err error) tea.Msg

// startChatStream sends a chat request and streams the response. Chunks are
// queued for WaitForStream; when the response ends, finish builds the last
// message. Cancelling ctx (Esc, new request) stops the request and closes
// the stream without a final message.
func startChatStream(ctx context.Context, requestID uint64, client Provider, messages []Message,
	tools []mcptypes.Tool, timeouts StreamTimeouts, finish streamFinish) *ChatStream {

	s := &ChatStream{requestID: requestID, events: make(chan streamEvent, 64)}

	go func() {
		defer close(s.events)

		streamCtx, cancel := context.WithCancelCause(ctx)
		defer cancel(nil)

		// Any data from the provider (also empty or "thinking" chunks) counts
		// as activity; the timer cancels the request when there's none
		var active atomic.Bool
		timer := time.AfterFunc(timeouts.FirstToken, func() {
			if active.Load() {
				cancel(&StreamTimeoutError{After: timeouts.Idle})
			} else {
				cancel(&StreamTimeoutError{FirstToken: true, After: timeouts.FirstToken})
			}
		})
		defer timer.Stop()

		var full strings.Builder
		var toolCalls []ToolCall
		err := client.ChatWithTools(streamCtx, messages, tools, func(chunk string, calls []ToolCall) error {
			active.Store(true)
			timer.Reset(timeouts.Idle)

			// Model output is shown in the terminal: no control sequences
			chunk = safetext.Clean(chunk)

			full.WriteString(chunk)
			// Providers report tool calls per callback (OpenAI-style APIs
			// send one per call), so keep every call, not just the first batch
			toolCalls = appendToolCalls(toolCalls, calls)

			if chunk == "" {
				return nil
			}
			select {
			case s.events <- streamEvent{text: chunk}:
				return nil
			case <-streamCtx.Done():
				return streamCtx.Err()
			}
		})
		timer.Stop()

		if ctx.Err() != nil {
			return // Cancelled: the request's results are ignored
		}
		var timeoutErr *StreamTimeoutError
		if cause := context.Cause(streamCtx); err != nil && errors.As(cause, &timeoutErr) {
			err = timeoutErr
		}

		select {
		case s.events <- streamEvent{final: finish(full.String(), toolCalls, err)}:
		case <-ctx.Done():
		}
	}()

	return s
}

// WaitForStream returns a Cmd that delivers the stream's next chunks (all
// that are queued, as one StreamChunkMsg) or its final message. Call it again
// for each StreamChunkMsg. Returns nil once a cancelled stream is closed.
func WaitForStream(s *ChatStream) tea.Cmd {
	return func() tea.Msg {
		ev, ok := s.next()
		if !ok {
			return nil
		}
		if ev.final != nil {
			return ev.final
		}

		// Coalesce chunks that are already queued (fast models, busy UI)
		var text strings.Builder
		text.WriteString(ev.text)
		for {
			select {
			case more, ok := <-s.events:
				if !ok {
					return StreamChunkMsg{Chunk: text.String(), RequestID: s.requestID, Stream: s}
				}
				if more.final != nil {
					s.held = &more
					return StreamChunkMsg{Chunk: text.String(), RequestID: s.requestID, Stream: s}
				}
				text.WriteString(more.text)
			default:
				return StreamChunkMsg{Chunk: text.String(), RequestID: s.requestID, Stream: s}
			}
		}
	}
}

func (s *ChatStream) next() (streamEvent, bool) {
	if s.held != nil {
		ev := *s.held
		s.held = nil
		return ev, true
	}
	ev, ok := <-s.events
	return ev, ok
}
