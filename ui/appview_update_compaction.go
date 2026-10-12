package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
	appmodel "otui/model"
)

// handleCompactionMessage handles all compaction-related messages
func (a AppView) handleCompactionMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case appmodel.CompactionRequestMsg:
		return a.handleCompactionRequest(msg)

	case appmodel.CompactionResponseMsg:
		return a.handleCompactionResponse(msg)

	case appmodel.CompactionCompleteMsg:
		return a.handleCompactionComplete(msg)

	case appmodel.TokenUsageUpdatedMsg:
		return a.handleTokenUsageUpdated(msg)

	default:
		return a, nil
	}
}

// compactingMessage is shown while a compaction runs
const compactingMessage = "⏳ Compacting session..."

// removeCompactingMessage removes the "Compacting..." notice, if present.
// Auto-compaction doesn't show one, so nothing else may be removed here
// (e.g. an iteration summary or a pending permission prompt).
func (a AppView) removeCompactingMessage() AppView {
	for i := len(a.dataModel.Messages) - 1; i >= 0; i-- {
		msg := a.dataModel.Messages[i]
		if msg.Role == "system" && !msg.Persistent && msg.Content == compactingMessage {
			a.dataModel.Messages = append(a.dataModel.Messages[:i], a.dataModel.Messages[i+1:]...)
			break
		}
	}
	return a
}

// handleCompactionComplete handles compaction completion
func (a AppView) handleCompactionComplete(msg appmodel.CompactionCompleteMsg) (AppView, tea.Cmd) {
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[ui] handleCompactionComplete: success=%v", msg.Success)
	}

	// Store the result on the session (in Update, not in the Cmd goroutine)
	err := a.dataModel.ApplyCompaction(msg)
	if errors.Is(err, appmodel.ErrStaleCompaction) {
		// Session switched or compacted further while the summary was generated
		if config.DebugLog != nil {
			config.DebugLog.Printf("[ui] Ignoring stale compaction result (session=%s marker=%d)", msg.SessionID, msg.Marker)
		}
		a = a.removeCompactingMessage()
		a.updateViewportContent(!a.userScrolledUp)
		return a, nil
	}
	if err != nil {
		msg.Success = false
		msg.Err = err
	}

	// Remove "Compacting..." message
	a = a.removeCompactingMessage()

	if !msg.Success {
		// Show error message inline
		a.dataModel.Messages = append(a.dataModel.Messages, appmodel.Message{
			Role:      "system",
			Content:   fmt.Sprintf("❌ Compaction failed: %v", msg.Err),
			Rendered:  fmt.Sprintf("❌ Compaction failed: %v", msg.Err),
			Timestamp: time.Now(),
		})
		a.updateViewportContent(!a.userScrolledUp)
		return a, nil
	}

	// Success - insert permanent compaction marker as a system message
	maxWidth := a.viewport.Width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	wrappedSummary := wrapText(msg.Summary, maxWidth)
	markerMsg := fmt.Sprintf("📦 Earlier Context Compacted\n\n%s\n\nContext now at %.0f%%",
		strings.Join(wrappedSummary, "\n"), a.dataModel.GetContextUsagePercentage()*100)

	// Insert marker message - this stays permanently in history
	a.dataModel.Messages = append(a.dataModel.Messages, appmodel.Message{
		Role:      "system",
		Content:   markerMsg,
		Rendered:  markerMsg,
		Timestamp: time.Now(),
		Persistent: true, // Mark as persistent so it gets saved
	})

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[ui] Session compacted successfully: %s", msg.Summary)
	}

	// Add LLM summary as persistent system message for display
	// (Actual LLM context injection happens via system prompt in SendToOllama)
	if msg.LLMSummary != "" {
		a.dataModel.Messages = append(a.dataModel.Messages, appmodel.Message{
			Role:       "system",
			Content:    msg.LLMSummary,
			Rendered:   msg.LLMSummary,
			Timestamp:  time.Now(),
			Persistent: true,
		})

		a.updateViewportContent(!a.userScrolledUp)

		// Trigger async markdown rendering for the summary message and save
		summaryIdx := len(a.dataModel.Messages) - 1
		return a, tea.Batch(
			a.renderMarkdownAsync(summaryIdx, msg.LLMSummary),
			a.dataModel.AutoSaveSession(),
		)
	}

	a.updateViewportContent(!a.userScrolledUp)

	// Save session after successful compaction
	return a, a.dataModel.AutoSaveSession()
}

// handleCompactionRequest shows inline approval message (like tool permissions)
func (a AppView) handleCompactionRequest(msg appmodel.CompactionRequestMsg) (AppView, tea.Cmd) {
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[ui] Compaction request: marker=%d, messages=%d", msg.MarkerIndex, msg.MessageCount)
	}

	// Build the approval message content
	percentage := float64(msg.CurrentTokens) / float64(msg.ContextWindow) * 100

	// Wrap the explanatory text to fit viewport
	maxWidth := a.viewport.Width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	line1 := "Compacted messages stay in history but won't be sent to LLM."
	line2 := "A summary will be generated to preserve context."

	wrappedLine1 := wrapText(line1, maxWidth)
	wrappedLine2 := wrapText(line2, maxWidth)

	content := fmt.Sprintf(`⚙️  Context Compaction Recommended

╰── Current: %.0f%% (%d / %d tokens)
╰── Messages: %d to compact (%d user, %d assistant)
╰── Result: ~0%% after compaction

%s
%s

[Y] Compact Now    [N] Cancel`,
		percentage, msg.CurrentTokens, msg.ContextWindow,
		msg.MessageCount, msg.UserMessageCount, msg.AssistantCount,
		strings.Join(wrappedLine1, "\n"),
		strings.Join(wrappedLine2, "\n"))

	// Add permission message to chat (like tool permissions)
	a.dataModel.Messages = append(a.dataModel.Messages, appmodel.Message{
		Role:      "system",
		Content:   content,
		Rendered:  content,
		Timestamp: time.Now(),
	})

	// Set waiting state
	a.waitingForCompaction = true
	a.pendingCompaction = &msg

	a.updateViewportContent(true)
	return a, nil
}

// handleCompactionResponse processes user's Y/N response
func (a AppView) handleCompactionResponse(msg appmodel.CompactionResponseMsg) (AppView, tea.Cmd) {
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[ui] Compaction response: approved=%v", msg.Approved)
	}

	// Remove approval message from chat
	a = a.removeLastSystemMessage()

	// Clear waiting state
	a.waitingForCompaction = false
	a.pendingCompaction = nil

	// If denied, just return
	if !msg.Approved {
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[ui] Compaction cancelled by user")
		}
		a.updateViewportContent(false)
		return a, nil
	}

	// Approved - show "Compacting..." message
	a.dataModel.Messages = append(a.dataModel.Messages, appmodel.Message{
		Role:      "system",
		Content:   compactingMessage,
		Rendered:  compactingMessage,
		Timestamp: time.Now(),
	})

	a.updateViewportContent(!a.userScrolledUp)

	// Trigger compaction in background
	return a, a.dataModel.CompactSessionMarkerCmd(msg.MarkerIndex)
}

// handleTokenUsageUpdated handles token usage updates
func (a AppView) handleTokenUsageUpdated(msg appmodel.TokenUsageUpdatedMsg) (AppView, tea.Cmd) {
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[ui] Token usage updated: %d active / %d total tokens",
			msg.Usage.ActiveTokens, msg.Usage.TotalTokens)
	}

	// Token usage is already in session - just trigger a re-render
	return a, nil
}

