package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
)

// handleStreamingMessage handles all streaming-related messages
func (a AppView) handleStreamingMessage(msg tea.Msg) (AppView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case streamChunksCollectedMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("streamChunksCollectedMsg received - %d chunks collected", len(msg.Chunks))
		}

		// Ignore if user cancelled
		if !a.dataModel.Streaming {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Ignoring streamChunksCollectedMsg - user cancelled")
			}
			return a, nil
		}

		// Keep system message - spinner stays animated until first real content arrives

		// Initialize typewriter effect
		a.chunks = msg.Chunks
		a.chunkIndex = 0
		a.dataModel.Streaming = true
		a.currentResp.Reset()

		// Start displaying chunks with typewriter effect after a brief delay
		// System message with animated spinner stays visible during this delay
		return a, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
			return displayChunkTickMsg{}
		})

	case displayChunkTickMsg:
		// Stop typewriter if user cancelled
		if !a.dataModel.Streaming {
			return a, nil
		}

		if a.chunkIndex >= len(a.chunks) {
			// All chunks displayed - finalize
			fullResp := a.currentResp.String()
			a.dataModel.Streaming = false
			a.chunks = nil
			a.chunkIndex = 0
			a.currentResp.Reset()

			if config.DebugLog != nil {
				config.DebugLog.Printf("Typewriter complete - finalizing message")
			}

			// Add final message and trigger markdown render
			a.dataModel.Messages = append(a.dataModel.Messages, Message{
				Role:      "assistant",
				Content:   fullResp,
				Rendered:  fullResp, // Start with plain text
				Timestamp: time.Now(),
			})

			messageIndex := len(a.dataModel.Messages) - 1
			a.updateViewportContent(true)
			a.dataModel.SessionDirty = true

			// Auto-save session and render markdown
			cmds = []tea.Cmd{
				a.renderMarkdownAsync(messageIndex, fullResp),
				a.dataModel.AutoSaveSession(),
			}
			return a, tea.Batch(cmds...)
		}

		// Display next chunk
		chunk := a.chunks[a.chunkIndex]
		a.chunkIndex++
		a.currentResp.WriteString(chunk)

		// Remove loading message AFTER first NON-EMPTY chunk is written
		if a.currentResp.String() != "" {
			if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
				a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
			}
		}

		// Only update streaming message if system message is already gone
		// (While system message exists, spinner animates via updateViewportContent in appview_update.go)
		if len(a.dataModel.Messages) == 0 || a.dataModel.Messages[len(a.dataModel.Messages)-1].Role != "system" {
			a.updateStreamingMessage()
		}

		// Schedule next chunk with delay (30ms, but first chunk is immediate)
		delay := 30 * time.Millisecond
		if a.chunkIndex == 1 {
			delay = time.Millisecond // First chunk nearly immediate
		}

		return a, tea.Tick(delay, func(time.Time) tea.Msg {
			return displayChunkTickMsg{}
		})

	case streamChunkMsg:
		a.currentResp.WriteString(msg.Chunk)
		a.updateStreamingMessage()
		return a, nil

	case streamDoneMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("streamDoneMsg received - response length: %d", len(msg.FullResponse))
		}

		a.dataModel.Streaming = false

		// Remove loading message (last system message)
		if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

		// Add final assistant message with plain text initially
		if msg.FullResponse != "" {
			a.dataModel.Messages = append(a.dataModel.Messages, Message{
				Role:      "assistant",
				Content:   msg.FullResponse,
				Rendered:  msg.FullResponse, // Start with plain text
				Timestamp: time.Now(),
			})

			messageIndex := len(a.dataModel.Messages) - 1

			if config.DebugLog != nil {
				config.DebugLog.Printf("Message added as plain text, triggering async markdown render")
			}

			// Update viewport immediately with plain text
			a.updateViewportContent(true)

			// Trigger async markdown rendering (non-blocking)
			return a, a.renderMarkdownAsync(messageIndex, msg.FullResponse)
		}
		// No response received
		if config.DebugLog != nil {
			config.DebugLog.Printf("ERROR: No response in streamDoneMsg")
		}
		a.dataModel.Messages = append(a.dataModel.Messages, Message{
			Role:      "system",
			Content:   "⚠️ No response received from Ollama",
			Rendered:  "⚠️ No response received from Ollama",
			Timestamp: time.Now(),
		})
		a.updateViewportContent(true)
		a.currentResp.Reset()

		return a, nil

	case streamErrorMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("streamErrorMsg received: %v", msg.Err)
		}

		a.dataModel.Streaming = false
		a.currentResp.Reset()

		// Remove loading message
		if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

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
				"Press Alt+M to change model.", currentModel)
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
	}

	return a, nil
}
