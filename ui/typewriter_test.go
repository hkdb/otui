package ui

import (
	"strings"
	"testing"

	"otui/config"
	appmodel "otui/model"
)

func typingAppView() AppView {
	a := AppView{currentResp: &strings.Builder{}, dataModel: &appmodel.Model{
		Config: &config.Config{Keybindings: config.DefaultKeybindings()},
	}}
	a.dataModel.Streaming = true
	return a
}

func TestTypewriterPaceIsCapped(t *testing.T) {
	// A fast model delivers a long answer at once: it types at the max rate
	a := typingAppView()
	a.queueText(strings.Repeat("x", 2000))
	a.revealText()
	want := int(typewriterMaxRate * typewriterTick.Seconds())
	if got := a.currentResp.Len(); got != want {
		t.Errorf("first tick showed %d chars, want %d", got, want)
	}
}

func TestTypewriterSpreadsSmallBursts(t *testing.T) {
	// A slow model's burst is spread over about typewriterLag, not shown at once
	a := typingAppView()
	a.queueText("Hello world, this is a slow model.") // 34 chars
	ticks := 0
	for !a.revealText() {
		ticks++
		if a.currentResp.Len() == 34 {
			t.Fatal("burst shown at once")
		}
	}
	if ticks < 5 {
		t.Errorf("burst shown in %d ticks, want it spread out", ticks)
	}
	if a.currentResp.String() != "Hello world, this is a slow model." {
		t.Errorf("shown %q", a.currentResp.String())
	}
}

func TestTypewriterKeepsRunesWhole(t *testing.T) {
	a := typingAppView()
	a.queueText("héllo wörld ✓ 日本語")
	for !a.revealText() {
		if !strings.HasPrefix("héllo wörld ✓ 日本語", a.currentResp.String()) {
			t.Fatalf("split a character: %q", a.currentResp.String())
		}
	}
}

func TestResponseFinishesAfterTyping(t *testing.T) {
	a := typingAppView()
	id := a.dataModel.RequestID()
	a.queueText("The answer.")

	// The response ends while text is still queued: held until it's shown
	b, _ := a.handleStreamingMessage(streamDoneMsg{FullResponse: "The answer.", RequestID: id})
	if len(b.dataModel.Messages) != 0 || !b.dataModel.Streaming || b.typeFinal == nil {
		t.Fatal("response finished before its text was shown")
	}

	for i := 0; i < 1000 && b.dataModel.Streaming; i++ {
		b, _ = b.handleStreamingMessage(typewriterTickMsg{RequestID: id})
	}
	if b.dataModel.Streaming || len(b.dataModel.Messages) != 1 || b.dataModel.Messages[0].Content != "The answer." {
		t.Fatalf("after typing: streaming=%v messages=%+v", b.dataModel.Streaming, b.dataModel.Messages)
	}
}

func TestEscShowsQueuedText(t *testing.T) {
	a := typingAppView()
	a.queueText("partial answer")
	a.revealText()
	a.cancelRequest()
	if a.currentResp.String() != "partial answer" || a.typeBacklog != "" || a.typing {
		t.Errorf("after Esc: shown %q, queued %q, typing %v", a.currentResp.String(), a.typeBacklog, a.typing)
	}
}
