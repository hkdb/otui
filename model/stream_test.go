package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// drainStream runs a request Cmd and follows its stream (as Update does)
// until the final message. Returns the final message and the streamed text.
func drainStream(t *testing.T, cmd tea.Cmd) (tea.Msg, string) {
	t.Helper()
	var text strings.Builder
	msg := cmd()
	for i := 0; i < 10000; i++ {
		switch m := msg.(type) {
		case StreamChunkMsg:
			text.WriteString(m.Chunk)
			msg = WaitForStream(m.Stream)()
		case ToolResultsReadyMsg:
			msg = WaitForStream(m.Stream)()
		default:
			return msg, text.String()
		}
	}
	t.Fatal("stream didn't end")
	return nil, ""
}

// scriptProvider streams chunks with a delay before each, then optionally
// stalls until the request is cancelled
type scriptProvider struct {
	metaProvider
	chunks []string
	delay  time.Duration
	stall  bool
}

func (p *scriptProvider) ChatWithTools(ctx context.Context, _ []Message, _ []mcptypes.Tool, callback StreamCallback) error {
	for _, c := range p.chunks {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := callback(c, nil); err != nil {
			return err
		}
	}
	if p.stall {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type finished struct {
	text string
	err  error
}

func finishWith(text string, _ []ToolCall, err error) tea.Msg { return finished{text, err} }

func streamFor(ctx context.Context, p Provider, timeouts StreamTimeouts) tea.Cmd {
	s := startChatStream(ctx, 1, p, nil, nil, timeouts, finishWith)
	return WaitForStream(s)
}

func TestStreamDeliversChunksThenFinal(t *testing.T) {
	p := &scriptProvider{chunks: []string{"Hel", "lo ", "world"}, delay: time.Millisecond}
	msg, text := drainStream(t, streamFor(context.Background(), p, StreamTimeouts{time.Second, time.Second}))
	f, ok := msg.(finished)
	if !ok || f.err != nil || f.text != "Hello world" || text != "Hello world" {
		t.Fatalf("final %#v, streamed %q", msg, text)
	}
}

func TestStreamFirstTokenTimeout(t *testing.T) {
	p := &scriptProvider{stall: true}
	msg, _ := drainStream(t, streamFor(context.Background(), p, StreamTimeouts{30 * time.Millisecond, time.Second}))
	var timeout *StreamTimeoutError
	if f := msg.(finished); !errors.As(f.err, &timeout) || !timeout.FirstToken {
		t.Fatalf("err = %v, want first token timeout", f.err)
	}
}

func TestStreamIdleTimeout(t *testing.T) {
	p := &scriptProvider{chunks: []string{"partial"}, stall: true}
	msg, text := drainStream(t, streamFor(context.Background(), p, StreamTimeouts{time.Second, 30 * time.Millisecond}))
	var timeout *StreamTimeoutError
	if f := msg.(finished); !errors.As(f.err, &timeout) || timeout.FirstToken || text != "partial" {
		t.Fatalf("err = %v, streamed %q, want idle timeout after the partial text", f.err, text)
	}
}

func TestStreamActivityResetsIdleTimeout(t *testing.T) {
	// 20 chunks 10ms apart (200ms) with a 50ms idle limit: never idle for long
	chunks := make([]string, 20)
	for i := range chunks {
		chunks[i] = "x"
	}
	p := &scriptProvider{chunks: chunks, delay: 10 * time.Millisecond}
	msg, text := drainStream(t, streamFor(context.Background(), p, StreamTimeouts{50 * time.Millisecond, 50 * time.Millisecond}))
	if f := msg.(finished); f.err != nil || len(text) != 20 {
		t.Fatalf("err = %v, streamed %d chars; a long but active response must not time out", f.err, len(text))
	}
}

func TestStreamCancelledEndsWithoutFinal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &scriptProvider{chunks: []string{"a"}, stall: true}
	cmd := streamFor(ctx, p, StreamTimeouts{time.Second, time.Second})

	first := cmd().(StreamChunkMsg)
	cancel()
	done := make(chan tea.Msg, 1)
	go func() { done <- WaitForStream(first.Stream)() }()
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("cancelled stream returned %#v, want nil", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled stream didn't close")
	}
}
