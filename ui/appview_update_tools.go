package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
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

		// Set executing tool indicator (use first tool's plugin name)
		if len(msg.ToolCalls) > 0 {
			// Extract plugin ID from namespaced tool name (e.g., "ihor-sokoliuk-mcp-searxng.search" -> "ihor-sokoliuk-mcp-searxng")
			toolName := msg.ToolCalls[0].Name
			var pluginID string
			if idx := strings.Index(toolName, "."); idx != -1 {
				pluginID = toolName[:idx]
			} else {
				pluginID = toolName
			}

			// Get short display name from registry (e.g., "ihor-sokoliuk-mcp-searxng" -> "mcp-searxng")
			if a.dataModel.MCPManager != nil {
				shortName := a.dataModel.MCPManager.GetPluginShortName(pluginID)
				if shortName != "" {
					a.executingTool = shortName
				} else {
					a.executingTool = pluginID // Fallback
				}
			} else {
				a.executingTool = pluginID
			}

			// Initialize and start tool execution spinner
			a.toolExecutionSpinner = spinner.New()
			a.toolExecutionSpinner.Spinner = spinner.Dot

			if config.DebugLog != nil {
				config.DebugLog.Printf("Starting tool execution for: %s", a.executingTool)
			}

			return a, tea.Batch(
				a.toolExecutionSpinner.Tick,
				a.dataModel.ExecuteToolsAndContinue(msg),
			)
		}

		return a, nil

	case toolExecutionCompleteMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool execution complete - %d chunks", len(msg.Chunks))
		}

		// Clear tool execution state
		a.executingTool = ""

		// Ignore if user cancelled
		if !a.dataModel.Streaming {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Ignoring toolExecutionCompleteMsg - user cancelled")
			}
			return a, nil
		}

		// Keep system message - spinner stays animated until first real content arrives

		// Initialize typewriter effect (same as normal responses)
		a.chunks = msg.Chunks
		a.chunkIndex = 0
		a.dataModel.Streaming = true
		a.currentResp.Reset()

		// Start displaying chunks with typewriter effect
		// System message with animated spinner stays visible during this delay
		return a, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
			return displayChunkTickMsg{}
		})

	case toolExecutionErrorMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool execution error: %v", msg.Err)
		}

		// Clear execution state
		a.executingTool = ""
		a.dataModel.Streaming = false
		a.currentResp.Reset()

		// Remove loading message
		if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

		// Show error message
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "system",
			Content:   fmt.Sprintf("❌ Tool execution error: %v", msg.Err),
			Rendered:  fmt.Sprintf("❌ Tool execution error: %v", msg.Err),
			Timestamp: time.Now(),
		})

		a.updateViewportContent(true)
		return a, nil
	}

	return a, nil
}
