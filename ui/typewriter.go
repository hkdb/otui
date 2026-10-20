package ui

import (
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// The typewriter shows streamed text at an even pace, however the provider
// delivers it: text is queued as it arrives and revealed on a ticker.
//
// The rate follows the queue: it's sized to show the queued text within
// typewriterLag (so a slow model's bursts are spread out until the next one
// arrives), capped at typewriterMaxRate (so a fast model types at a steady
// speed instead of appearing at once).
const (
	typewriterTick    = 25 * time.Millisecond
	typewriterLag     = 500 * time.Millisecond
	typewriterMaxRate = 130.0 // Characters per second (about the old typewriter's speed)
	typewriterMinRate = 20.0  // Characters per second (so short tails don't crawl)
)

// typewriterTickMsg reveals the next part of the queued text
type typewriterTickMsg struct {
	RequestID uint64
}

func (a *AppView) typewriterTickCmd() tea.Cmd {
	requestID := a.dataModel.RequestID()
	return tea.Tick(typewriterTick, func(time.Time) tea.Msg {
		return typewriterTickMsg{RequestID: requestID}
	})
}

// queueText adds streamed text to the typewriter. Returns the tick Cmd if
// the typewriter isn't running yet.
func (a *AppView) queueText(text string) tea.Cmd {
	a.typeBacklog += text
	if a.typing {
		return nil
	}
	a.typing = true
	return a.typewriterTickCmd()
}

// revealText shows the next part of the queued text. Returns true when
// everything queued has been shown.
func (a *AppView) revealText() bool {
	queued := utf8.RuneCountInString(a.typeBacklog)
	if queued == 0 {
		return true
	}

	rate := min(max(float64(queued)/typewriterLag.Seconds(), typewriterMinRate), typewriterMaxRate)
	a.typeCarry += rate * typewriterTick.Seconds()
	n := int(a.typeCarry)
	if n == 0 {
		return false
	}
	a.typeCarry -= float64(n)

	// Cut after n characters (whole runes)
	cut := len(a.typeBacklog)
	if n < queued {
		cut = 0
		for i := 0; i < n; i++ {
			_, size := utf8.DecodeRuneInString(a.typeBacklog[cut:])
			cut += size
		}
	}

	a.showText(a.typeBacklog[:cut])
	a.typeBacklog = a.typeBacklog[cut:]
	return a.typeBacklog == ""
}

// showText adds text to the response shown so far
func (a *AppView) showText(text string) {
	if text == "" {
		return
	}
	// Text replaces the "Waiting for response..." / "Analyzing results..."
	// message (persistent step messages stay)
	if a.currentResp.Len() == 0 {
		a.removeLastNonPersistentSystemMessage()
	}
	a.currentResp.WriteString(text)
	a.updateViewportContent(!a.userScrolledUp)
}

// deferUntilTyped holds the message that ends a response until the
// typewriter has shown all of its text (it's handled by the last tick).
// Returns false if there's nothing left to show.
func (a *AppView) deferUntilTyped(msg tea.Msg) bool {
	if a.typeBacklog == "" {
		return false
	}
	a.typeFinal = msg
	return true
}

// flushTypewriter shows all queued text at once and stops the typewriter
// (cancelled or failed response)
func (a *AppView) flushTypewriter() {
	backlog := a.typeBacklog
	a.resetTypewriter()
	if backlog != "" {
		a.currentResp.WriteString(backlog)
	}
}

// resetTypewriter drops queued text and stops the typewriter
func (a *AppView) resetTypewriter() {
	a.typeBacklog = ""
	a.typeCarry = 0
	a.typing = false
	a.typeFinal = nil
}
