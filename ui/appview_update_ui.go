package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// handleUIMessage handles UI-related messages (flash, markdown, models, sessions, editor, data export)
func (a AppView) handleUIMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case flashTickMsg:
		if a.highlightFlashCount > 0 && a.highlightFlashCount < 6 {
			a.highlightFlashCount++
			a.updateViewportContent(false)
			return a, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return flashTickMsg{}
			})
		}
		a.highlightedMessageIdx = -1
		a.highlightFlashCount = 0
		a.updateViewportContent(false)
		return a, nil

	case markdownRenderedMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("markdownRenderedMsg received for message %d", msg.MessageIndex)
		}

		if msg.MessageIndex >= 0 && msg.MessageIndex < len(a.dataModel.Messages) {
			a.dataModel.Messages[msg.MessageIndex].Rendered = msg.Rendered

			gotoBottom := a.highlightedMessageIdx < 0
			a.updateViewportContent(gotoBottom)
			if config.DebugLog != nil {
				config.DebugLog.Printf("Viewport updated with rendered markdown (gotoBottom=%v)", gotoBottom)
			}
		}
		return a, nil

	case modelsListMsg:
		if msg.Err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error fetching Models: %v", msg.Err)
			}
			// Optionally show error to user
			return a, nil
		}

		a.modelList = msg.Models
		a.modelListCached = true

		if config.DebugLog != nil {
			config.DebugLog.Printf("Fetched %d models", len(msg.Models))
		}

		// Only auto-show selector if explicitly requested (user-initiated fetch)
		// Background fetches (provider settings save, etc.) don't interrupt UX
		if a.showSettings && msg.ShowSelector {
			a.showModelSelector = true
			// Pre-select current model if in list
			currentModel := a.settingsFields[2].Value
			for i, model := range a.modelList {
				if model.Name == currentModel {
					a.selectedModelIdx = i
					break
				}
			}
		}

		return a, nil

	case sessionsListMsg:
		if msg.Err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error fetching Sessions: %v", msg.Err)
			}
			return a, nil
		}

		a.sessionList = msg.Sessions
		a.selectedSessionIdx = 0

		// Select current session if session manager is open
		if a.showSessionManager && a.dataModel.CurrentSession != nil {
			for i, session := range msg.Sessions {
				if session.ID == a.dataModel.CurrentSession.ID {
					a.selectedSessionIdx = i
					break
				}
			}
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("Fetched %d sessions", len(msg.Sessions))
		}

		// Check if we just deleted the current session
		if a.dataModel.CurrentSession == nil {
			if len(msg.Sessions) > 0 {
				// Load the first session in the list
				if config.DebugLog != nil {
					config.DebugLog.Printf("Current session deleted, loading first available Session: %s", msg.Sessions[0].ID)
				}
				return a, a.dataModel.LoadSession(msg.Sessions[0].ID)
			}
			// No sessions left - close modal and show empty state
			if config.DebugLog != nil {
				config.DebugLog.Printf("No sessions left after deletion, showing empty state")
			}
			a.showSessionManager = false
		}

		return a, nil

	case editorContentMsg:
		// Load edited content into textarea
		a.textarea.SetValue(msg.Content)
		a.textarea.Focus()

		// Load content and wait for user to press Enter (user can review/edit before sending)
		return a, nil

	case editorErrorMsg:
		// Show error modal
		a.showInfoModal = true
		a.infoModalTitle = "⚠️  Editor Error"
		a.infoModalMsg = fmt.Sprintf("Failed to open external editor:\n\n%v\n\nPlease check that your $EDITOR or $OTUI_EDITOR environment variable is set correctly.", msg.Err)
		return a, nil

	case dataExportedMsg:
		if msg.Cancelled {
			// Data export was cancelled - check if partial file exists
			a.exportingDataDir = false
			a.dataExportCancelCtx = nil
			a.dataExportCancelFunc = nil

			if fileExists(a.dataExportTargetPath) {
				// Start cleanup phase
				a.dataExportCleaningUp = true
				return a, tea.Batch(
					a.dataExportSpinner.Tick,
					a.dataModel.CleanupPartialDataExportCmd(a.dataExportTargetPath),
				)
			}
			// No partial file - just close modal
			a.dataExportMode = false
			a.dataExportTargetPath = ""
			return a, nil
		}

		if msg.Err != nil {
			// Data export failed - close modal with error
			a.exportingDataDir = false
			a.dataExportCancelCtx = nil
			a.dataExportCancelFunc = nil
			a.dataExportMode = false
			a.dataExportTargetPath = ""
			if config.DebugLog != nil {
				config.DebugLog.Printf("Data export error: %v", msg.Err)
			}
			return a, nil
		}

		// Success - show success modal
		a.exportingDataDir = false
		a.dataExportCancelCtx = nil
		a.dataExportCancelFunc = nil
		a.dataExportSuccess = msg.Path
		a.dataExportTargetPath = ""
		if config.DebugLog != nil {
			config.DebugLog.Printf("Data directory exported successfully to: %s", msg.Path)
		}
		return a, nil

	case dataExportCleanupDoneMsg:
		// Data export cleanup finished - return to settings
		a.dataExportCleaningUp = false
		a.dataExportMode = false
		a.dataExportTargetPath = ""
		return a, nil
	}

	return a, nil
}
