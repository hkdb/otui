package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	appmodel "otui/model"
)

// handleStreamingMessage handles all streaming-related messages
func (a AppView) handleStreamingMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case streamChunkMsg:
		// Ignore if user cancelled (or a newer request started)
		if a.staleRequest(msg.RequestID) || !a.dataModel.Streaming {
			return a, nil
		}

		// The typewriter shows the text at an even pace
		var tick tea.Cmd
		if msg.Chunk != "" {
			tick = a.queueText(msg.Chunk)
		}

		return a, tea.Batch(appmodel.WaitForStream(msg.Stream), tick)

	case typewriterTickMsg:
		// Ticks of a cancelled request stop (a new request runs its own)
		if a.staleRequest(msg.RequestID) {
			return a, nil
		}
		if !a.dataModel.Streaming {
			a.typing = false
			return a, nil
		}

		if !a.revealText() {
			return a, a.typewriterTickCmd()
		}

		// Everything shown: handle the end of the response if it arrived
		a.typing = false
		if final := a.typeFinal; final != nil {
			a.typeFinal = nil
			m, cmd := a.Update(final)
			return m.(AppView), cmd
		}
		return a, nil

	case streamDoneMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("streamDoneMsg received - response length: %d", len(msg.FullResponse))
		}

		if a.staleRequest(msg.RequestID) || !a.dataModel.Streaming {
			return a, nil
		}

		// Finish once the typewriter has shown the whole response
		if a.deferUntilTyped(msg) {
			return a, nil
		}

		return a, a.finishResponse(msg.FullResponse, nil)

	case streamErrorMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("streamErrorMsg received: %v", msg.Err)
		}

		// Errors of a cancelled request (e.g. "context canceled") aren't shown
		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		a.dataModel.Streaming = false
		a.userScrolledUp = false

		// Remove loading message (but not persistent step messages)
		a.removeLastNonPersistentSystemMessage()

		// Keep what arrived before the error
		a.flushTypewriter()
		a.keepPartialResponse("⚠️ Response incomplete")

		// Check if error is about tool support
		errorMsg := msg.Err.Error()

		// Default: show the raw error
		displayMsg := fmt.Sprintf("❌ Error: %v", msg.Err)

		// Override with friendly message if it's a tool support error
		if strings.Contains(errorMsg, "does not support tools") {
			currentModel := a.dataModel.Provider.GetModel()
			displayMsg = fmt.Sprintf("❌ Error: %s does not support tool calling.\n\n"+
				"Your session has enabled plugins that require tool support.\n"+
				"Switch to a tool-capable model marked with [🔧] next to it.\n\n"+
				"Press %s to change model.", currentModel, a.formatKeyDisplay("primary", "M"))
		}

		// Wrap error message to fit viewport width
		maxWidth := a.width - 10 // Leave padding for margins
		if maxWidth > 0 {
			displayMsg = lipgloss.NewStyle().Width(maxWidth).Render(displayMsg)
		}

		// Show error message
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "system",
			Content:   displayMsg,
			Rendered:  displayMsg,
			Timestamp: time.Now(),
		})
		a.updateViewportContent(true)
		return a, nil

	case notifyCompleteMsg:
		// No-op — notification already emitted
		return a, nil
	}

	return a, nil
}

// finishResponse ends a response: the text becomes an assistant message
// (followed by the multi-step summary, if any), and the session is saved
func (a *AppView) finishResponse(fullResp string, summary *IterationSummaryMsg) tea.Cmd {
	a.dataModel.Streaming = false
	a.currentResp.Reset()
	a.resetTypewriter()

	if config.DebugLog != nil {
		config.DebugLog.Printf("Response complete - finalizing message (len=%d)", len(fullResp))
	}

	// Remove "Waiting for response..." / "Analyzing results..." message
	a.removeLastNonPersistentSystemMessage()

	if fullResp != "" {
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "assistant",
			Content:   fullResp,
			Rendered:  fullResp,
			Timestamp: time.Now(),
		})
	}
	messageIndex := len(a.dataModel.Messages) - 1

	// Add summary after assistant message (Phase 2)
	if summary != nil && summary.TotalSteps > 0 {
		summaryContent := buildIterationSummary(*summary)
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:       "system",
			Content:    summaryContent,
			Rendered:   summaryContent,
			Timestamp:  time.Now(),
			Persistent: true,
		})
	}

	a.updateViewportContent(!a.userScrolledUp)
	a.dataModel.SessionDirty = true

	var cmds []tea.Cmd
	if fullResp != "" {
		cmds = append(cmds, a.renderMarkdownAsync(messageIndex, fullResp))
	}
	cmds = append(cmds,
		a.dataModel.AutoSaveSession(),
		a.dataModel.CheckAutoCompactionCmd(), // Check if auto-compaction should trigger
		// The model is loaded now, so local servers (e.g. Ollama) can
		// report the context it actually runs with
		a.dataModel.RefreshModelMetadataCmd(),
	)
	if a.dataModel.Config.NotifyOnComplete {
		cmds = append(cmds, notifyResponseComplete())
	}
	return tea.Batch(cmds...)
}

// keepPartialResponse turns text streamed so far into an assistant message
// marked with note (response cancelled or failed)
func (a *AppView) keepPartialResponse(note string) {
	partial := a.currentResp.String()
	a.currentResp.Reset()
	if partial == "" {
		return
	}
	content := partial + "\n\n" + note
	a.dataModel.Messages = append(a.dataModel.Messages, Message{
		Role:      "assistant",
		Content:   content,
		Rendered:  content,
		Timestamp: time.Now(),
	})
}
