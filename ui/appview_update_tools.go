package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// handleToolMessage handles tool execution messages
func (a AppView) handleToolMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case toolCallsDetectedMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool calls detected: %d calls", len(msg.ToolCalls))
		}

		// Tool calls of a cancelled request must not run
		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		// Sync iteration state
		a.iterationCount = a.dataModel.CurrentIteration
		a.maxIterations = a.dataModel.MaxIterations

		if len(msg.ToolCalls) == 0 {
			return a, nil
		}

		// Remove "Waiting for response..." message
		a.removeLastNonPersistentSystemMessage()

		// Extract purpose (covering every tool of this step) and create step message
		purpose := a.dataModel.StepPurpose(msg.ContextMessages, msg.InitialResponse, msg.ToolCalls)
		a.createStepMessage(purpose, a.iterationCount+1)

		// Start tool execution
		a.startToolExecution(msg.ToolCalls[0].Name)

		return a, tea.Batch(
			a.toolExecutionSpinner.Tick,
			a.loadingSpinner.Tick,
			a.dataModel.ExecuteToolsAndContinue(msg),
		)

	case toolExecutionCompleteMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool execution complete - %d chunks", len(msg.Chunks))
		}

		// Cancelled (Esc already reset the tool state)
		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		// Clear tool execution state
		a.executingTool = ""

		// CLEANUP: Remove temporarily allowed tools
		a = a.cleanupTemporaryTools()

		// Ignore if user cancelled
		if !a.dataModel.Streaming {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Ignoring toolExecutionCompleteMsg - cancelled")
			}
			return a, nil
		}

		// Complete the step (checkmark + persistent)
		a.completeStepMessage()

		// Show analyzing state
		a.showAnalyzingResults()

		// Store summary to add after typewriter completes
		if msg.IterationSummary.TotalSteps > 0 {
			a.pendingSummary = &msg.IterationSummary
		}

		// Store pending step info if iteration continues
		if msg.HasMoreSteps {
			a.pendingNextStep = true
			a.pendingToolCalls = msg.NextToolCalls
			a.pendingToolContext = msg.NextContext
			a.pendingToolResponse = msg.NextInitialResponse
		}
		if !msg.HasMoreSteps {
			a.pendingNextStep = false
			a.iterationCount = 0
			a.dataModel.CurrentIteration = 0
		}

		// Start typewriter
		return a, a.startTypewriter(msg.Chunks)

	case toolExecutionErrorMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool execution error: %v", msg.Err)
		}

		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		// Clear state
		a.executingTool = ""
		a.dataModel.Streaming = false
		a.currentResp.Reset()
		a.iterationCount = 0
		a.dataModel.CurrentIteration = 0
		a.pendingNextStep = false

		// CLEANUP: Remove temporarily allowed tools
		a = a.cleanupTemporaryTools()

		// Remove step message
		if len(a.dataModel.Messages) > 0 &&
			a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" &&
			strings.HasPrefix(a.dataModel.Messages[len(a.dataModel.Messages)-1].Content, "🔧 ") {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

		// Show error
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "system",
			Content:   fmt.Sprintf("❌ Tool execution error: %v", msg.Err),
			Rendered:  fmt.Sprintf("❌ Tool execution error: %v", msg.Err),
			Timestamp: time.Now(),
		})

		a.updateViewportContent(true)
		return a, nil

	case toolPermissionRequestMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Permission request for tool: %s", msg.ToolName)
		}

		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		// Clear tool execution state (we're paused for permission)
		a.executingTool = ""

		// Build permission content
		details := a.dataModel.BuildToolDetails(msg.ToolCall)
		content := buildPermissionContent(msg.ToolName, msg.Purpose, details)

		// Add permission message to chat
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "system",
			Content:   content,
			Rendered:  content,
			Timestamp: time.Now(),
		})

		// Set waiting state
		a.waitingForPermission = true
		a.pendingPermission = &msg

		a.updateViewportContent(true)
		return a, nil

	case toolPermissionResponseMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Permission response - Approved: %v, AlwaysAllow: %v", msg.Approved, msg.AlwaysAllow)
		}

		if a.staleRequest(msg.RequestID) {
			return a, nil
		}

		// Remove permission message from chat
		a = a.removeLastSystemMessage()

		// Clear waiting state
		a.waitingForPermission = false
		a.pendingPermission = nil

		// EARLY RETURN: Handle denial
		if !msg.Approved {
			a.dataModel.Streaming = false
			a.dataModel.CancelRequest()

			// Tools approved "once" earlier in this batch didn't run - drop them
			a = a.cleanupTemporaryTools()

			// Remove "Waiting for response..." loading message (but not persistent step messages)
			if len(a.dataModel.Messages) > 0 &&
				a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" &&
				!a.dataModel.Messages[len(a.dataModel.Messages)-1].Persistent {
				a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
			}

			// Show error message
			a.dataModel.Messages = append(a.dataModel.Messages, Message{
				Role:      "system",
				Content:   fmt.Sprintf("❌ Permission denied for tool: %s", msg.ToolName),
				Rendered:  fmt.Sprintf("❌ Permission denied for tool: %s", msg.ToolName),
				Timestamp: time.Now(),
			})
			a.updateViewportContent(true)
			return a, nil
		}

		// EARLY RETURN: No session to work with
		if a.dataModel.CurrentSession == nil {
			return a, nil
		}

		// Tool approved - add to allowed list (both "once" and "always")
		a.dataModel.CurrentSession.AllowedTools = append(
			a.dataModel.CurrentSession.AllowedTools,
			msg.ToolName,
		)

		// Handle persistence based on AlwaysAllow flag
		if msg.AlwaysAllow {
			// ALWAYS ALLOW: Mark session dirty for disk persistence
			a.dataModel.SessionDirty = true
			if config.DebugLog != nil {
				config.DebugLog.Printf("Added %s to session allowed tools (PERSISTENT)", msg.ToolName)
			}
		}

		// NOT AlwaysAllow: Track for removal after execution
		if !msg.AlwaysAllow {
			a.temporarilyAllowedTools = append(a.temporarilyAllowedTools, msg.ToolName)
			// DON'T mark session dirty (no disk persistence)
			if config.DebugLog != nil {
				config.DebugLog.Printf("Added %s to session allowed tools (TEMPORARY)", msg.ToolName)
			}
		}

		// Re-trigger execution of the whole batch with the approved tool now
		// in the allowed list (any other unapproved tool is asked about next)
		toolCalls := msg.ToolCalls
		if len(toolCalls) == 0 {
			toolCalls = []ToolCall{msg.ToolCall}
		}
		toolMsg := toolCallsDetectedMsg{
			ToolCalls:       toolCalls,
			InitialResponse: msg.InitialResponse,
			ContextMessages: msg.ContextMessages,
			RequestID:       msg.RequestID,
		}

		// Start tool execution with proper state
		a.startToolExecution(msg.ToolCall.Name)

		return a, tea.Batch(
			a.toolExecutionSpinner.Tick,
			a.dataModel.ExecuteToolsAndContinue(toolMsg),
		)
	}

	return a, nil
}

// removeLastSystemMessage removes the last system message from the chat (used to clean up permission prompts)
func (a AppView) removeLastSystemMessage() AppView {
	if len(a.dataModel.Messages) == 0 {
		return a
	}

	lastIdx := len(a.dataModel.Messages) - 1
	if a.dataModel.Messages[lastIdx].Role == "system" {
		a.dataModel.Messages = a.dataModel.Messages[:lastIdx]
		a.updateViewportContent(false) // Don't auto-scroll when removing
	}

	return a
}

// cleanupTemporaryTools removes temporarily allowed tools from the session
func (a AppView) cleanupTemporaryTools() AppView {
	// EARLY RETURN: Nothing to clean up
	if len(a.temporarilyAllowedTools) == 0 {
		return a
	}

	// EARLY RETURN: No session to clean
	if a.dataModel.CurrentSession == nil {
		a.temporarilyAllowedTools = []string{}
		return a
	}

	// Remove each temporary tool from session.AllowedTools
	for _, tempTool := range a.temporarilyAllowedTools {
		filtered := []string{}
		for _, tool := range a.dataModel.CurrentSession.AllowedTools {
			if tool != tempTool {
				filtered = append(filtered, tool)
			}
		}
		a.dataModel.CurrentSession.AllowedTools = filtered

		if config.DebugLog != nil {
			config.DebugLog.Printf("Removed temporary tool from allowed list: %s", tempTool)
		}
	}

	// Clear temporary list
	a.temporarilyAllowedTools = []string{}

	return a
}
