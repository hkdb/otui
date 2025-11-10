package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
)

// handleSessionMessage handles session-related messages
func (a AppView) handleSessionMessage(msg tea.Msg) (AppView, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionLoadedMsg:
		if config.DebugLog != nil && msg.Session != nil {
			config.DebugLog.Printf("[Session Load] === ENTRY === session=%s, enabledPlugins=%d, pluginsEnabledInConfig=%v, managerNil=%v",
				msg.Session.Name, len(msg.Session.EnabledPlugins), a.dataModel.Config.PluginsEnabled, a.dataModel.MCPManager == nil)
		}

		if msg.Err != nil {
			// Check if error is due to session being locked
			if msg.Err.Error() == "session_locked" {
				a.showAcknowledgeModal = true
				a.acknowledgeModalTitle = "Session In Use"
				a.acknowledgeModalMsg = "This session is currently being used in another OTUI instance.\n\n" +
					"Only one instance can use a session at a time.\n\n" +
					"Options:\n" +
					"• Close the other OTUI instance\n" +
					"• Use a different session\n" +
					"• Run OTUI in a container for isolated instances"
				a.acknowledgeModalType = ModalTypeWarning
				return a, nil
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("Error loading Session: %v", msg.Err)
			}
			return a, nil
		}

		// Unlock old session before switching
		if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
			_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
		}

		// Load session into UI and sync with MCP manager
		a.setCurrentSession(msg.Session)

		a.dataModel.SessionDirty = false
		a.showSessionManager = false

		// Save as current session so it's restored on next launch
		if a.dataModel.SessionStorage != nil && msg.Session != nil {
			a.dataModel.SessionStorage.SaveCurrentSessionID(msg.Session.ID)
		}

		// Convert storage messages to UI messages
		a.dataModel.Messages = []Message{}
		for _, sMsg := range msg.Session.Messages {
			// Use cached rendering if available, otherwise use content
			rendered := sMsg.Rendered
			if rendered == "" {
				rendered = sMsg.Content
			}
			a.dataModel.Messages = append(a.dataModel.Messages, Message{
				Role:      sMsg.Role,
				Content:   sMsg.Content,
				Rendered:  rendered,
				Timestamp: sMsg.Timestamp,
			})
		}

		// Set model and provider from session (Phase 1.6: multi-provider support)
		if msg.Session.Model != "" {
			sessionProvider := msg.Session.Provider
			if sessionProvider == "" {
				sessionProvider = a.dataModel.Config.DefaultProvider // Use config default for migrated sessions
			}

			// Switch to session's provider
			provider, ok := a.dataModel.Providers[sessionProvider]
			if !ok {
				// Fallback to current provider if session's provider not available
				if config.Debug && config.DebugLog != nil {
					config.DebugLog.Printf("[UI] WARNING: Session provider '%s' not found, using fallback", sessionProvider)
				}
				a.dataModel.Provider.SetModel(msg.Session.Model)
			} else {
				a.dataModel.Provider = provider
				provider.SetModel(msg.Session.Model)
				if config.Debug && config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Loaded session with provider '%s', model '%s'", sessionProvider, msg.Session.Model)
				}
			}
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("Loaded session %s with %d messages", msg.Session.ID, len(msg.Session.Messages))
		}

		// Check if we need to scroll to a specific message
		if a.pendingScrollToMessageIdx >= 0 && a.pendingScrollToMessageIdx < len(a.dataModel.Messages) {
			messageIdx := a.pendingScrollToMessageIdx
			a.pendingScrollToMessageIdx = -1

			var offsetContent strings.Builder
			for i := range messageIdx {
				msg := a.dataModel.Messages[i]

				timestamp := DimStyle.Render(msg.Timestamp.Format("[15:04]"))

				var roleStyle = DimStyle
				var roleName string
				switch msg.Role {
				case "user":
					roleStyle = UserStyle
					roleName = "You"
				case "assistant":
					roleStyle = AssistantStyle
					roleName = "Assistant"
				default:
					roleStyle = DimStyle
					roleName = "System"
				}

				role := roleStyle.Render(roleName)
				renderedContent := msg.Rendered

				if msg.Role == "user" {
					greenBold := "\x1b[32;1m"
					reset := "\x1b[0m"
					bar := greenBold + "┃" + reset

					lines := strings.Split(renderedContent, "\n")
					offsetContent.WriteString(fmt.Sprintf("%s %s %s\n", bar, timestamp, role))
					for _, line := range lines {
						offsetContent.WriteString(fmt.Sprintf("%s %s\n", bar, line))
					}
					offsetContent.WriteString("\n")
				} else {
					offsetContent.WriteString(fmt.Sprintf("%s %s\n%s\n\n", timestamp, role, renderedContent))
				}
			}

			actualOffset := strings.Count(offsetContent.String(), "\n")
			viewportHeight := a.viewport.Height
			centerOffset := actualOffset - (viewportHeight / 2)
			centerOffset = max(centerOffset, 0)

			a.highlightedMessageIdx = messageIdx
			a.highlightFlashCount = 1
			a.updateViewportContent(false)

			totalLines := a.viewport.TotalLineCount()
			if centerOffset > totalLines-viewportHeight {
				centerOffset = totalLines - viewportHeight
			}

			a.viewport.SetYOffset(centerOffset)

			// Trigger flash animation
			var renderCmds []tea.Cmd
			renderCmds = append(renderCmds, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return flashTickMsg{}
			}))

			// Trigger markdown rendering for user and assistant messages that need it
			// Render in REVERSE order (newest first) since viewport shows bottom
			for i := len(a.dataModel.Messages) - 1; i >= 0; i-- {
				if a.dataModel.Messages[i].Role == "assistant" || a.dataModel.Messages[i].Role == "user" {
					// Skip if already rendered (cached from disk)
					if a.dataModel.Messages[i].Rendered != "" && a.dataModel.Messages[i].Rendered != a.dataModel.Messages[i].Content {
						continue
					}
					renderCmds = append(renderCmds, a.renderMarkdownAsync(i, a.dataModel.Messages[i].Content))
				}
			}

			return a, tea.Batch(renderCmds...)
		}

		// No pending scroll, go to bottom as usual
		a.updateViewportContent(true)

		// Trigger markdown rendering for user and assistant messages that need it
		// Render in REVERSE order (newest first) since viewport shows bottom
		var renderCmds []tea.Cmd
		for i := len(a.dataModel.Messages) - 1; i >= 0; i-- {
			if a.dataModel.Messages[i].Role == "assistant" || a.dataModel.Messages[i].Role == "user" {
				// Skip if already rendered (cached from disk)
				if a.dataModel.Messages[i].Rendered != "" && a.dataModel.Messages[i].Rendered != a.dataModel.Messages[i].Content {
					continue
				}
				renderCmds = append(renderCmds, a.renderMarkdownAsync(i, a.dataModel.Messages[i].Content))
			}
		}

		// ORCHESTRATION: Plugin system startup after data directory change
		// When data directory changes in Settings, the current session is cleared (nil).
		// After the session list is fetched, the first session loads and triggers this handler.
		// At this point, if plugins are enabled and the session has enabled plugins, we need to:
		// 1. Recreate MCP manager (it was destroyed during data dir change)
		// 2. Start plugins for this session
		// This completes the data directory change flow.
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Session Load] Checking plugin startup: pluginsEnabled=%v, sessionPluginCount=%d, managerNil=%v, condition=%v",
				a.dataModel.Config.PluginsEnabled, len(msg.Session.EnabledPlugins), a.dataModel.MCPManager == nil,
				a.dataModel.Config.PluginsEnabled && len(msg.Session.EnabledPlugins) > 0)
		}
		if a.dataModel.Config.PluginsEnabled && len(msg.Session.EnabledPlugins) > 0 {
			// Check if manager needs recreation (destroyed during data dir change)
			if a.dataModel.MCPManager == nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Recreating MCP manager after data dir change - session '%s' has %d enabled plugins", msg.Session.Name, len(msg.Session.EnabledPlugins))
				}

				// Recreate manager (same pattern as settings.go line 291-292)
				if err := a.ensureMCPManager(); err != nil {
					if config.DebugLog != nil {
						config.DebugLog.Printf("[UI] Failed to recreate MCP manager: %v", err)
					}
					// Continue without plugins - don't block session load
					return a, tea.Batch(renderCmds...)
				}

				// Manager created successfully - show startup modal and start plugins
				if config.DebugLog != nil {
					config.DebugLog.Printf("[Session Load] === TRIGGERING PLUGIN STARTUP === session=%s", msg.Session.Name)
				}

				// Show startup modal (same pattern as settings.go line 313-321)
				a.pluginSystemState = PluginSystemState{
					Active:    true,
					Operation: "starting",
					Phase:     "waiting",
					Spinner:   spinner.New(),
					StartTime: time.Now(),
				}
				a.pluginSystemState.Spinner.Spinner = spinner.Dot
				a.pluginSystemState.Spinner.Style = lipgloss.NewStyle().Foreground(successColor)

				// Start plugins (same pattern as settings.go line 323-326)
				// Combine markdown rendering with plugin startup
				allCmds := append(renderCmds,
					a.pluginSystemState.Spinner.Tick,
					startPluginSystemCmd(a.dataModel.MCPManager, msg.Session),
				)
				return a, tea.Batch(allCmds...)
			}
		}

		return a, tea.Batch(renderCmds...)

	case sessionSavedMsg:
		if msg.Err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error saving Session: %v", msg.Err)
			}
			return a, nil
		}

		a.dataModel.SessionDirty = false

		if config.DebugLog != nil {
			config.DebugLog.Printf("Session saved successfully")
		}

		return a, nil

	case sessionRenamedMsg:
		if msg.Err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error renaming Session: %v", msg.Err)
			}
			return a, nil
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("Session renamed successfully")
		}

		return a, nil

	case sessionExportedMsg:
		if msg.Cancelled {
			// Export was cancelled - check if partial file exists
			a.exportingSession = false
			a.exportCancelCtx = nil
			a.exportCancelFunc = nil

			// Check if partial file was created
			if fileExists(a.exportTargetPath) {
				// Start cleanup phase
				a.exportCleaningUp = true
				return a, tea.Batch(
					a.exportSpinner.Tick,
					a.dataModel.CleanupPartialFileCmd(a.exportTargetPath),
				)
			}
			// No partial file - just close modal
			a.sessionExportMode = false
			a.exportTargetPath = ""
			return a, nil
		}

		if msg.Err != nil {
			// Export failed - close modal with error
			a.exportingSession = false
			a.exportCancelCtx = nil
			a.exportCancelFunc = nil
			a.sessionExportMode = false
			a.exportTargetPath = ""
			if config.DebugLog != nil {
				config.DebugLog.Printf("Export error: %v", msg.Err)
			}
			return a, nil
		}

		// Success - show success modal
		a.exportingSession = false
		a.exportCancelCtx = nil
		a.exportCancelFunc = nil
		a.sessionExportSuccess = msg.Path
		a.exportTargetPath = ""
		if config.DebugLog != nil {
			config.DebugLog.Printf("Session exported successfully to: %s", msg.Path)
		}
		return a, nil

	case sessionImportedMsg:
		a.sessionImportPicker.Processing = false
		a.sessionImportPicker.CleaningUp = false
		a.sessionImportCancelCtx = nil
		a.sessionImportCancelFunc = nil

		if msg.Cancelled {
			a.sessionImportPicker.Reset()
			return a, nil
		}

		if msg.Err != nil {
			// Import failed - close modal with error
			a.sessionImportPicker.Reset()
			if config.DebugLog != nil {
				config.DebugLog.Printf("Import error: %v", msg.Err)
			}
			return a, nil
		}

		// Success - show success modal and refresh session list
		successMsg := fmt.Sprintf("Imported: %s\nMessages: %d\nModel: %s",
			msg.Session.Name, len(msg.Session.Messages), msg.Session.Model)
		a.sessionImportPicker.Success = &successMsg
		a.sessionImportSuccess = msg.Session
		if config.DebugLog != nil {
			config.DebugLog.Printf("Session imported successfully: %s", msg.Session.Name)
		}

		// Refresh session list in background
		return a, func() tea.Msg {
			sessions, err := a.dataModel.SessionStorage.List()
			return sessionsListMsg{Sessions: sessions, Err: err}
		}

	case exportCleanupDoneMsg:
		// Cleanup finished - return to session manager
		a.exportCleaningUp = false
		a.sessionExportMode = false
		a.exportTargetPath = ""
		return a, nil
	}

	return a, nil
}
