package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	"otui/mcp"
	"otui/storage"
)

func (a AppView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Update spinner FIRST to handle TickMsg before anything else
	if a.dataModel.Streaming && len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
		a.loadingSpinner, cmd = a.loadingSpinner.Update(msg)
		cmds = append(cmds, cmd)
		// Update viewport to show animated spinner
		a.updateViewportContent(true)
	}

	// Update import spinner if processing or cleaning up
	if a.sessionImportPicker.Processing || a.sessionImportPicker.CleaningUp {
		a.sessionImportPicker.Spinner, cmd = a.sessionImportPicker.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	if a.exportingSession || a.exportCleaningUp {
		a.exportSpinner, cmd = a.exportSpinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update data export spinner if exporting or cleaning up
	if a.exportingDataDir || a.dataExportCleaningUp {
		a.dataExportSpinner, cmd = a.dataExportSpinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update plugin system spinner if modal is active
	if a.pluginSystemState.Active && a.pluginSystemState.Phase == "waiting" {
		a.pluginSystemState.Spinner, cmd = a.pluginSystemState.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update tool execution spinner if executing tools (Phase 6)
	if a.executingTool != "" {
		a.toolExecutionSpinner, cmd = a.toolExecutionSpinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update plugin operation spinner if modal is active
	if a.showPluginOperationModal && (a.pluginOperationPhase == "enabling" || a.pluginOperationPhase == "disabling") {
		a.pluginOperationSpinner, cmd = a.pluginOperationSpinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update registry refresh spinner if modal is active
	if a.pluginManagerState.registryRefresh.visible && a.pluginManagerState.registryRefresh.phase == "fetching" {
		a.pluginManagerState.registryRefresh.spinner, cmd = a.pluginManagerState.registryRefresh.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update install spinner if modal is active and not in error/complete state
	if a.pluginManagerState.installModal.visible && a.pluginManagerState.installModal.error == "" && a.pluginManagerState.installModal.progress.Stage != "complete" {
		a.pluginManagerState.installModal.spinner, cmd = a.pluginManagerState.installModal.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update uninstall spinner if modal is active and not in error/complete state
	if a.pluginManagerState.uninstallModal.visible && a.pluginManagerState.uninstallModal.error == "" && a.pluginManagerState.uninstallModal.progress.Stage != "complete" {
		a.pluginManagerState.uninstallModal.spinner, cmd = a.pluginManagerState.uninstallModal.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update file picker if active (needs to receive ALL message types EXCEPT KeyMsg)
	// KeyMsg is handled in handleSessionImportMode to check DidSelectFile before updating
	if a.sessionImportPicker.Active && !a.sessionImportPicker.Processing && !a.sessionImportPicker.CleaningUp {
		switch msg.(type) {
		case tea.KeyMsg:
			// Skip - handled in handleSessionImportMode
		default:
			// Forward non-KeyMsg (like readDirMsg)
			a.sessionImportPicker.Picker, cmd = a.sessionImportPicker.Picker.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		// Reserve space for title (1 line), separator (1 line), textarea (3 lines), and status bar (1 line)
		viewportHeight := a.height - 6
		a.viewport.Width = a.width
		a.viewport.Height = viewportHeight
		a.textarea.SetWidth(a.width)

		a.ready = true
		a.updateViewportContent(true)

		// Trigger initial rendering if needed (after we have width)
		if a.dataModel.NeedsInitialRender {
			a.dataModel.NeedsInitialRender = false
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
			return a, tea.Batch(renderCmds...)
		}

		return a, nil

	case tea.KeyMsg:
		// PRIORITY 0: Always-global shortcuts (quit, help toggle)
		if msg.String() == "alt+q" {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Alt+Q pressed (location 1) - beginning quit sequence")
			}

			// If shutdown modal is already showing, force quit immediately (user pressed Alt+Q twice)
			if a.pluginSystemState.Active {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Alt+Q pressed while shutdown modal active - FORCE QUITTING")
				}
				if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
					_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
				}
				return a, tea.Quit
			}

			// If plugins are enabled, show shutdown modal and attempt graceful shutdown
			if a.dataModel.MCPManager != nil && a.dataModel.Config.PluginsEnabled {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Alt+Q: Plugins enabled, showing shutdown modal")
				}
				a.dataModel.Quitting = true
				a.pluginSystemState = PluginSystemState{
					Active:    true,
					Operation: "stopping",
					Phase:     "waiting",
					Spinner:   spinner.New(),
					StartTime: time.Now(),
				}
				a.pluginSystemState.Spinner.Spinner = spinner.Dot
				return a, tea.Batch(
					a.pluginSystemState.Spinner.Tick,
					a.dataModel.StartPluginShutdown(3*time.Second), // 3 second timeout
				)
			}

			// No plugins or plugins disabled - quit immediately
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Alt+Q: No plugins, quitting immediately")
			}
			if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
				_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
			}
			return a, tea.Quit
		}

		// PRIORITY 1: Modal toggle shortcuts (close current modal, open new one)
		switch msg.String() {
		case "alt+h":
			a.showHelp = !a.showHelp
			return a, nil

		case "alt+n":
			a.closeAllModals()

			// Unlock current session before clearing
			if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
				_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
			}

			// Create and save new session (shared implementation)
			// This allows plugins to be enabled before first message (better UX + security)
			newSession, err := a.dataModel.CreateAndSaveNewSession("New Session", "", []string{})
			if err != nil {
				// If session creation fails, show error but don't crash
				if config.DebugLog != nil {
					config.DebugLog.Printf("Failed to create new Session: %v", err)
				}
				// Could show error modal here, but for now just return
				return a, nil
			}

			// Clear UI and sync with MCP manager
			a.dataModel.Messages = []Message{}
			a.setCurrentSession(newSession) // Syncs MCP manager - fixes security hole!
			a.dataModel.SessionDirty = false
			a.textarea.Reset()
			a.updateViewportContent(true)
			return a, nil

		case "alt+s":
			wasOpen := a.showSessionManager
			a.closeAllModals()
			a.showSessionManager = !wasOpen
			if a.showSessionManager {
				return a, a.dataModel.FetchSessionList()
			}
			return a, nil

		case "alt+e":
			// Only allow editing if we have a current session
			if a.dataModel.CurrentSession != nil {
				a.closeAllModals()

				// Lazy init inputs (consistent with "e" key handler)
				if a.newSessionNameInput.Width == 0 {
					a.newSessionNameInput = textinput.New()
					a.newSessionNameInput.Width = 60
					a.newSessionNameInput.CharLimit = 100
					a.newSessionNameInput.Placeholder = "Enter session name (optional)"
				}
				if a.newSessionPromptInput.Width() == 0 {
					a.newSessionPromptInput = textarea.New()
					a.newSessionPromptInput.SetWidth(60)
					a.newSessionPromptInput.SetHeight(5)
					a.newSessionPromptInput.CharLimit = 0
					a.newSessionPromptInput.Placeholder = "Enter system prompt (optional)"
				}

				a.showEditSessionModal = true
				a.editSessionID = a.dataModel.CurrentSession.ID
				a.newSessionFocusedField = 0
				a.editSessionPluginIdx = 0
				a.editSessionEnabledPlugins = make([]string, len(a.dataModel.CurrentSession.EnabledPlugins))
				copy(a.editSessionEnabledPlugins, a.dataModel.CurrentSession.EnabledPlugins)
				a.newSessionNameInput.SetValue(a.dataModel.CurrentSession.Name)
				a.newSessionPromptInput.SetValue(a.dataModel.CurrentSession.SystemPrompt)
				a.newSessionNameInput.Focus()
				a.newSessionPromptInput.Blur()
				return a, textinput.Blink
			}
			return a, nil

		case "alt+m":
			wasOpen := a.showModelSelector
			a.closeAllModals()
			a.showModelSelector = !wasOpen
			if a.showModelSelector {
				currentModel := a.dataModel.OllamaClient.GetModel()
				for i, model := range a.modelList {
					if model.Name == currentModel {
						a.selectedModelIdx = i
						break
					}
				}
			}
			return a, nil

		case "alt+f":
			wasOpen := a.showMessageSearch
			a.closeAllModals()
			a.showMessageSearch = !wasOpen
			if a.showMessageSearch {
				a.messageSearchInput.Focus()
				a.messageSearchInput.SetValue("")
				a.messageSearchResults = []storage.MessageMatch{}
				a.selectedSearchIdx = 0
				return a, textinput.Blink
			}
			return a, nil

		case "alt+F":
			wasOpen := a.showGlobalSearch
			a.closeAllModals()
			a.showGlobalSearch = !wasOpen
			if a.showGlobalSearch {
				a.globalSearchInput.Focus()
				a.globalSearchInput.SetValue("")
				a.globalSearchResults = []storage.SessionMessageMatch{}
				a.selectedGlobalIdx = 0
				return a, textinput.Blink
			}
			return a, nil

		case "alt+S":
			wasOpen := a.showSettings
			a.closeAllModals()
			a.showSettings = !wasOpen
			if a.showSettings {
				a.settingsFields = []SettingField{
					{
						Label:        "Data Directory",
						Value:        a.dataModel.Config.DataDirectory,
						DefaultValue: "~/.local/share/otui",
						Type:         SettingTypeDataDir,
						Validation:   FieldValidationNone,
					},
					{
						Label:        "Ollama Host",
						Value:        a.dataModel.Config.OllamaHost,
						DefaultValue: "http://localhost:11434",
						Type:         SettingTypeOllamaHost,
						Validation:   FieldValidationNone,
					},
					{
						Label:        "Default Model",
						Value:        a.dataModel.Config.DefaultModel,
						DefaultValue: "llama3.1:latest",
						Type:         SettingTypeModel,
						Validation:   FieldValidationNone,
					},
					{
						Label:        "System Prompt",
						Value:        a.dataModel.Config.DefaultSystemPrompt,
						DefaultValue: "",
						Type:         SettingTypeSystemPrompt,
						Validation:   FieldValidationNone,
					},
					{
						Label:        "Enable Plugins",
						Value:        boolToString(a.dataModel.Config.PluginsEnabled),
						DefaultValue: "false",
						Type:         SettingTypePluginsEnabled,
						Validation:   FieldValidationNone,
					},
				}
				a.selectedSettingIdx = 0
				a.settingsEditMode = false
				a.settingsHasChanges = false
				a.settingsConfirmExit = false
				a.settingsLoadedInfo = ""

				a.settingsEditInput = textinput.New()
				a.settingsEditInput.Width = 50
				a.settingsEditInput.CharLimit = 200
			}
			return a, nil

		case "alt+A":
			wasOpen := a.showAbout
			a.closeAllModals()
			a.showAbout = !wasOpen
			return a, nil

		case "alt+p":
			// Check if plugin system is enabled
			if !a.dataModel.Config.PluginsEnabled {
				a.closeAllModals()
				a.showInfoModal = true
				a.infoModalTitle = "⚠️  Plugin System Disabled"
				a.infoModalMsg = "The plugin system is currently disabled.\n\nEnable it in Settings (Alt+Shift+S) to use plugins."
				return a, nil
			}

			wasOpen := a.showPluginManager
			a.closeAllModals()
			a.showPluginManager = !wasOpen
			if a.showPluginManager {
				a.initPluginManager()
			}
			return a, nil
		}

		// PRIORITY 2: Modal-specific key handling (order matches View rendering)
		// Info modal (highest priority - close on any key)
		if a.showInfoModal {
			a.showInfoModal = false
			a.infoModalTitle = ""
			a.infoModalMsg = ""
			return a, nil
		}

		// Plugin system modal handlers
		if a.pluginSystemState.Active {
			if a.pluginSystemState.Phase == "error" {
				// Error phase - Enter to dismiss
				if msg.String() == "enter" {
					a.pluginSystemState = PluginSystemState{}
					return a, nil
				}
				return a, nil
			}

			if a.pluginSystemState.Phase == "unresponsive" {
				switch msg.String() {
				case "y":
					// Wait longer - try again with another 2 second timeout
					if config.DebugLog != nil {
						config.DebugLog.Printf("[UI] User chose to wait, trying shutdown again")
					}
					a.pluginSystemState.Phase = "waiting"
					a.pluginSystemState.UnresponsivePlugins = []string{}
					a.pluginSystemState.ErrorMsg = "" // Clear previous error before retry

					// Check if this is app quit or settings toggle
					if a.dataModel.Quitting {
						return a, tea.Batch(
							a.pluginSystemState.Spinner.Tick,
							a.dataModel.StartPluginShutdown(2*time.Second),
						)
					} else {
						// Settings toggle - use stopPluginSystemCmd
						return a, tea.Batch(
							a.pluginSystemState.Spinner.Tick,
							stopPluginSystemCmd(a.dataModel.MCPManager),
						)
					}
				case "n":
					if a.dataModel.Quitting {
						// Quit immediately without waiting
						if config.DebugLog != nil {
							config.DebugLog.Printf("[UI] User chose to quit immediately, unlocking and quitting")
						}
						if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
							_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
						}
						return a, tea.Quit
					} else {
						// Settings toggle - force shutdown, clear modal
						if config.DebugLog != nil {
							config.DebugLog.Printf("[UI] User chose to force shutdown plugins")
						}
						a.pluginSystemState = PluginSystemState{}
						a.dataModel.MCPManager = nil
						return a, nil
					}
				}
			}
			// Ignore all other keys while plugin system modal is active
			return a, nil
		}

		if a.showAcknowledgeModal {
			if msg.String() == "enter" {
				a.showAcknowledgeModal = false
				return a, nil
			}
			return a, nil
		}

		if a.showPluginOperationModal {
			// Allow escape to cancel operation at any time
			if msg.String() == "esc" {
				a.showPluginOperationModal = false
				a.pluginOperationPhase = ""
				a.pluginOperationName = ""
				a.pluginOperationError = ""

				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] User cancelled plugin operation with ESC")
				}

				return a, nil
			}

			// Allow enter to close error modal
			if a.pluginOperationPhase == "error" && msg.String() == "enter" {
				a.showPluginOperationModal = false
				a.pluginOperationPhase = ""
				a.pluginOperationName = ""
				a.pluginOperationError = ""
				return a, nil
			}
			// Block all other input while processing
			return a, nil
		}

		if a.showHelp {
			if msg.String() == "esc" {
				a.showHelp = false
			}
			return a, nil
		}

		if a.showModelSelector {
			return a.handleModelSelectorUpdate(msg)
		}

		if a.showToolWarningModal {
			return a.handleToolWarningModalUpdate(msg)
		}

		if a.showSystemPromptToolWarning {
			return a.handleSystemPromptToolWarningUpdate(msg)
		}

		if a.showSettings {
			return a.handleSettingsUpdate(msg)
		}

		// Check child modals BEFORE parent (New/Edit session before Session Manager)
		if a.showNewSessionModal {
			return a.handleNewSessionModalUpdate(msg)
		}

		if a.showEditSessionModal {
			return a.handleEditSessionModalUpdate(msg)
		}

		if a.showSessionManager {
			return a.handleSessionManagerUpdate(msg)
		}

		if a.showPluginManager {
			return a.handlePluginManagerUpdate(msg)
		}

		if a.showGlobalSearch {
			return a.handleGlobalSearchUpdate(msg)
		}

		if a.showMessageSearch {
			return a.handleMessageSearchUpdate(msg)
		}

		if a.showAbout {
			return a.handleAboutUpdate(msg)
		}

		// PRIORITY 3: Tab handling (chat input)
		if msg.String() == "tab" && !a.dataModel.Streaming {
			a.textarea.InsertString("   ")
			return a, nil
		}

		// PRIORITY 4: Streaming cancellation (only if no modal open)
		if msg.String() == "esc" && a.dataModel.Streaming {
			a.dataModel.Streaming = false

			partialResp := a.currentResp.String()

			if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
				a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
			}

			if partialResp != "" {
				a.dataModel.Messages = append(a.dataModel.Messages, Message{
					Role:      "assistant",
					Content:   partialResp + "\n\n⚠️ Response cancelled",
					Rendered:  partialResp + "\n\n⚠️ Response cancelled",
					Timestamp: time.Now(),
				})
			} else {
				a.dataModel.Messages = append(a.dataModel.Messages, Message{
					Role:      "system",
					Content:   "⚠️ Request cancelled",
					Rendered:  "⚠️ Request cancelled",
					Timestamp: time.Now(),
				})
			}

			a.chunks = nil
			a.chunkIndex = 0
			a.currentResp.Reset()

			a.updateViewportContent(true)
			return a, nil
		}

		// Handle Enter for sending messages - DON'T let textarea process it
		// But allow Alt+Enter to pass through for newlines
		if msg.Type == tea.KeyEnter && !msg.Alt && !a.dataModel.Streaming {
			if a.textarea.Value() != "" {
				// Check if we need to show system prompt + tools warning
				// Conditions: system prompt is set, tools are available, warning hasn't been shown yet
				if a.dataModel.CurrentSession != nil &&
					a.dataModel.CurrentSession.SystemPrompt != "" &&
					!a.systemPromptToolWarningShown &&
					a.dataModel.MCPManager != nil {
					// Check if current session has any enabled plugins
					enabledPlugins := a.dataModel.MCPManager.GetSessionEnabledPluginNames(a.dataModel.CurrentSession)
					if len(enabledPlugins) > 0 {
						// Show warning modal - DON'T clear textarea yet (user might cancel)
						a.showSystemPromptToolWarning = true
						return a, nil
					}
				}

				userMsg := a.textarea.Value()
				a.textarea.Reset()

				// Clear editor temp file (defense in depth)
				if err := config.ClearEditorTempFile(); err != nil {
					if config.DebugLog != nil {
						config.DebugLog.Printf("Warning: failed to clear editor temp file: %v", err)
					}
				}

				if config.DebugLog != nil {
					config.DebugLog.Printf("Enter pressed - sending message: %s", userMsg)
				}

				// Add user message
				a.dataModel.Messages = append(a.dataModel.Messages, Message{
					Role:      "user",
					Content:   userMsg,
					Rendered:  userMsg, // Start with plain text, will be rendered async
					Timestamp: time.Now(),
				})

				// Trigger markdown rendering for user message
				userMessageIndex := len(a.dataModel.Messages) - 1

				// Initialize and start spinner
				a.loadingSpinner = spinner.New()
				a.loadingSpinner.Spinner = spinner.Dot
				a.loadingSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // Bright white

				// Add loading message (will be updated with spinner in updateViewportContent)
				loadingMsg := "Waiting for response..."
				a.dataModel.Messages = append(a.dataModel.Messages, Message{
					Role:      "system",
					Content:   loadingMsg,
					Rendered:  loadingMsg,
					Timestamp: time.Now(),
				})

				a.dataModel.Streaming = true
				a.updateViewportContent(true)

				if config.DebugLog != nil {
					config.DebugLog.Printf("Firing sendToOllama() Cmd")
				}

				// Start streaming response, spinner animation, and render user message markdown
				return a, tea.Batch(
					a.renderMarkdownAsync(userMessageIndex, userMsg),
					a.dataModel.SendToOllama(),
					a.loadingSpinner.Tick,
				)
			}
			// Don't pass Enter to textarea - we handled it
			return a, nil
		}

		switch msg.String() {
		case "alt+q":
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Alt+Q pressed (location 2) - beginning quit sequence")
			}

			// If plugins are enabled, show shutdown modal and attempt graceful shutdown
			if a.dataModel.MCPManager != nil && a.dataModel.Config.PluginsEnabled {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Ctrl+C: Plugins enabled, showing shutdown modal")
				}
				a.dataModel.Quitting = true
				a.pluginSystemState = PluginSystemState{
					Active:    true,
					Operation: "stopping",
					Phase:     "waiting",
					Spinner:   spinner.New(),
					StartTime: time.Now(),
				}
				a.pluginSystemState.Spinner.Spinner = spinner.Dot
				return a, tea.Batch(
					a.pluginSystemState.Spinner.Tick,
					a.dataModel.StartPluginShutdown(3*time.Second), // 3 second timeout
				)
			}

			// No plugins or plugins disabled - quit immediately
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Alt+Q: No plugins, quitting immediately")
			}
			if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
				_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
			}
			return a, tea.Quit

		case "alt+A":
			a.showAbout = !a.showAbout
			return a, nil

		case "alt+i":
			// Open external editor (only if not streaming)
			if !a.dataModel.Streaming {
				return a, a.dataModel.OpenExternalEditor(a.textarea.Value())
			}
			return a, nil

		case "esc":
			if a.showAbout {
				a.showAbout = false
				return a, nil
			}
			// Fall through to other esc handlers

		case "alt+y":
			// Copy last assistant message
			for i := len(a.dataModel.Messages) - 1; i >= 0; i-- {
				if a.dataModel.Messages[i].Role == "assistant" {
					clipboard.WriteAll(a.dataModel.Messages[i].Content)
					return a, nil
				}
			}
			return a, nil

		case "alt+c":
			// Copy all messages
			var allText strings.Builder
			for _, msg := range a.dataModel.Messages {
				role := msg.Role
				switch role {
				case "user":
					role = "You"
				case "assistant":
					role = "Assistant"
				}
				allText.WriteString(fmt.Sprintf("[%s] %s:\n%s\n\n",
					msg.Timestamp.Format("15:04"),
					role,
					msg.Content))
			}
			clipboard.WriteAll(allText.String())
			return a, nil

		case "alt+j", "alt+down":
			a.viewport.HalfPageDown()
			return a, nil

		case "alt+k", "alt+up":
			a.viewport.HalfPageUp()
			return a, nil

		case "alt+J", "pgdown":
			a.viewport.PageDown()
			return a, nil

		case "alt+K", "pgup":
			a.viewport.PageUp()
			return a, nil

		case "alt+g":
			a.viewport.GotoTop()
			return a, nil

		case "alt+G":
			a.viewport.GotoBottom()
			return a, nil
		}

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

		// Remove loading message
		if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

		// Initialize typewriter effect
		a.chunks = msg.Chunks
		a.chunkIndex = 0
		a.dataModel.Streaming = true
		a.currentResp.Reset()

		// Start displaying chunks with typewriter effect after a brief delay
		// This gives the spinner time to be visible
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
		a.updateStreamingMessage()

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
		} else {
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
		}
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
		var displayMsg string
		if strings.Contains(errorMsg, "does not support tools") {
			// Extract model name from error if possible
			currentModel := a.dataModel.OllamaClient.GetModel()
			displayMsg = fmt.Sprintf("❌ Error: %s does not support tool calling.\n\n"+
				"Your session has enabled plugins that require tool support.\n"+
				"Switch to a tool-capable model marked with [🔧] next to it.\n\n"+
				"Press Alt+M to change model.", currentModel)
		} else {
			displayMsg = fmt.Sprintf("❌ Error: %v\n\nMake sure OLLAMA_HOST is set correctly.", msg.Err)
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

	// Phase 6: Tool execution handlers
	case toolCallsDetectedMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("Tool calls detected: %d calls", len(msg.ToolCalls))
		}

		// Set executing tool indicator (use first tool's plugin name)
		if len(msg.ToolCalls) > 0 {
			// Extract plugin ID from namespaced tool name (e.g., "ihor-sokoliuk-mcp-searxng.search" -> "ihor-sokoliuk-mcp-searxng")
			toolName := msg.ToolCalls[0].Function.Name
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

		// Remove loading message if present
		if len(a.dataModel.Messages) > 0 && a.dataModel.Messages[len(a.dataModel.Messages)-1].Role == "system" {
			a.dataModel.Messages = a.dataModel.Messages[:len(a.dataModel.Messages)-1]
		}

		// Initialize typewriter effect (same as normal responses)
		a.chunks = msg.Chunks
		a.chunkIndex = 0
		a.dataModel.Streaming = true
		a.currentResp.Reset()

		// Start displaying chunks with typewriter effect
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
			config.DebugLog.Printf("Fetched %d models from Ollama", len(msg.Models))
		}

		// If in settings mode, open model selector
		if a.showSettings {
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
			} else {
				// No sessions left - close modal and show empty state
				if config.DebugLog != nil {
					config.DebugLog.Printf("No sessions left after deletion, showing empty state")
				}
				a.showSessionManager = false
			}
		}

		return a, nil

	case shutdownProgressMsg:
		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] ========== shutdownProgressMsg RECEIVED ==========")
			config.DebugLog.Printf("[UI] shutdownProgressMsg: phase=%s, unresponsive=%v, err=%v", msg.Phase, msg.UnresponsiveNames, msg.Err)
		}

		if msg.Phase == "complete" {
			// Shutdown completed successfully, quit now
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] shutdownProgressMsg: Phase is 'complete' - shutting down cleanly")
			}
			if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
				_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
			}

			// Clear plugin system state
			a.pluginSystemState = PluginSystemState{}

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] shutdownProgressMsg: Returning tea.Quit")
			}
			return a, tea.Quit
		}

		if msg.Phase == "unresponsive" {
			// Plugins didn't respond, show unresponsive modal
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] shutdownProgressMsg: Phase is 'unresponsive' - switching to warning modal")
				config.DebugLog.Printf("[UI] shutdownProgressMsg: Setting phase='unresponsive', unresponsivePlugins=%v", msg.UnresponsiveNames)
			}
			a.pluginSystemState.Phase = "unresponsive"
			a.pluginSystemState.UnresponsivePlugins = msg.UnresponsiveNames

			// Capture error reason (timeout or other error)
			if msg.Err != nil {
				a.pluginSystemState.ErrorMsg = msg.Err.Error()
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] shutdownProgressMsg: Shutdown error: %v", msg.Err)
				}
			} else {
				a.pluginSystemState.ErrorMsg = "Shutdown timed out"
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] shutdownProgressMsg: Returning m (modal should now show unresponsive state)")
			}
			return a, nil
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] shutdownProgressMsg: WARNING - Unknown phase '%s', returning m", msg.Phase)
		}
		return a, nil

	case pluginSystemOperationMsg:
		// Handle plugin system start/stop completion (from Settings toggle)
		// IMPORTANT: This must be at top level because Settings modal closes before operation completes
		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] pluginSystemOperationMsg: operation=%s, success=%v", msg.operation, msg.success)
		}

		if msg.operation == "starting" {
			if !msg.success {
				// Startup failed - show error
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Plugin startup FAILED: %v", msg.err)
				}
				a.pluginSystemState.Phase = "error"
				if msg.err != nil {
					a.pluginSystemState.ErrorMsg = fmt.Sprintf("Failed to start plugins: %v", msg.err)
				} else {
					a.pluginSystemState.ErrorMsg = "Failed to start plugins (unknown error)"
				}
				return a, nil
			}

			// Success - reload config to reflect PluginsEnabled=true
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Plugin startup SUCCESS - reloading config")
			}
			cfg, err := config.Load()
			if err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] ERROR reloading config after plugin enable: %v", err)
				}
				a.pluginSystemState.Phase = "error"
				a.pluginSystemState.ErrorMsg = fmt.Sprintf("Plugins started but failed to reload config: %v", err)
				return a, nil
			}
			a.dataModel.Config = cfg

			// Dismiss modal
			a.pluginSystemState = PluginSystemState{}

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Plugins enabled successfully, config reloaded, modal dismissed")
			}
			return a, nil
		}

		if msg.operation == "stopping" {
			if !msg.success && len(msg.unresponsivePlugins) > 0 {
				// Some plugins didn't respond - show warning
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Plugin shutdown UNRESPONSIVE: %v", msg.unresponsivePlugins)
				}
				a.pluginSystemState.Phase = "unresponsive"
				a.pluginSystemState.UnresponsivePlugins = msg.unresponsivePlugins
				if msg.err != nil {
					a.pluginSystemState.ErrorMsg = msg.err.Error()
				}
				return a, nil
			}

			// Success - reload config to reflect PluginsEnabled=false
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Plugin shutdown SUCCESS - reloading config")
			}
			cfg, err := config.Load()
			if err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[UI] ERROR reloading config after plugin disable: %v", err)
				}
				// Still proceed with shutdown even if reload fails
			} else {
				a.dataModel.Config = cfg
			}

			// Dismiss modal, clear manager
			a.pluginSystemState = PluginSystemState{}
			a.dataModel.MCPManager = nil

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Plugins disabled successfully, config reloaded, modal dismissed")
			}
			return a, nil
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] pluginSystemOperationMsg: Unknown operation '%s'", msg.operation)
		}
		return a, nil

	case pluginOperationCompleteMsg:
		// If modal is closed, ignore the message (user cancelled with ESC)
		if !a.showPluginOperationModal {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Ignoring pluginOperationCompleteMsg - modal was cancelled")
			}
			return a, nil
		}

		// Handle plugin enable/disable completion
		if msg.Err != nil {
			// Error - show error in modal (config NOT saved, plugin remains in previous state)
			a.pluginOperationPhase = "error"
			a.pluginOperationError = msg.Err.Error()

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] pluginOperationCompleteMsg: ERROR for %s operation on plugin '%s': %v", msg.Operation, msg.PluginID, msg.Err)
			}
		} else {
			// Success - save config and close modal
			if a.pluginManagerState.pluginState.Config != nil {
				switch msg.Operation {
				case "enable":
					a.pluginManagerState.pluginState.Config.SetPluginEnabled(msg.PluginID, true)
					if config.DebugLog != nil {
						config.DebugLog.Printf("[UI] pluginOperationCompleteMsg: Plugin '%s' enabled successfully, saving config", msg.PluginID)
					}
				case "disable":
					a.pluginManagerState.pluginState.Config.SetPluginEnabled(msg.PluginID, false)
					if config.DebugLog != nil {
						config.DebugLog.Printf("[UI] pluginOperationCompleteMsg: Plugin '%s' disabled successfully, saving config", msg.PluginID)
					}
				}
				_ = config.SavePluginsConfig(a.dataModel.Config.DataDir(), a.pluginManagerState.pluginState.Config)
			}

			a.showPluginOperationModal = false
			a.pluginOperationPhase = ""
			a.pluginOperationName = ""
			a.pluginOperationError = ""

			// Refresh plugin list to show the updated status
			if a.showPluginManager && a.pluginManagerState.pluginState.Registry != nil && a.pluginManagerState.selection.filterMode {
				query := a.pluginManagerState.selection.filterInput.Value()
				if query != "" {
					a.pluginManagerState.selection.filteredPlugins = a.pluginManagerState.pluginState.Registry.Search(query)
				} else {
					a.pluginManagerState.selection.filteredPlugins = nil
				}
			}
		}
		return a, nil

	case registryRefreshCompleteMsg:
		// If modal is closed, ignore the message (user cancelled with ESC)
		if !a.pluginManagerState.registryRefresh.visible {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] Ignoring registryRefreshCompleteMsg - modal was closed")
			}
			return a, nil
		}

		// Handle registry refresh completion
		if msg.Err != nil {
			// Error - show error in modal
			a.pluginManagerState.registryRefresh.phase = "error"
			a.pluginManagerState.registryRefresh.error = msg.Err.Error()

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] registryRefreshCompleteMsg: ERROR refreshing registry: %v", msg.Err)
			}
		} else {
			// Success - show success message
			a.pluginManagerState.registryRefresh.phase = "success"

			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] registryRefreshCompleteMsg: Registry refreshed successfully")
			}

			// Update filter results if filter is active
			if a.showPluginManager && a.pluginManagerState.pluginState.Registry != nil && a.pluginManagerState.selection.filterMode {
				query := a.pluginManagerState.selection.filterInput.Value()
				if query != "" {
					a.pluginManagerState.selection.filteredPlugins = a.pluginManagerState.pluginState.Registry.Search(query)
				} else {
					a.pluginManagerState.selection.filteredPlugins = nil
				}
			}

			// Reset selection to first plugin
			a.pluginManagerState.selection.selectedPluginIdx = 0
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

	case pluginStartupCompleteMsg:
		// Handle plugin startup failures
		if len(msg.Failures) > 0 {
			// Build error message listing all failed plugins
			var errMsg strings.Builder
			errMsg.WriteString("The following plugins failed to start:\n\n")
			for pluginID, err := range msg.Failures {
				// Try to get plugin name from registry
				pluginName := pluginID
				if a.pluginManagerState.pluginState.Registry != nil {
					if plugin := a.pluginManagerState.pluginState.Registry.GetByID(pluginID); plugin != nil {
						pluginName = plugin.Name
					}
				}
				errMsg.WriteString(fmt.Sprintf("• %s: %v\n", pluginName, err))
			}
			errMsg.WriteString("\nYou can try disabling and re-enabling these plugins in the Plugin Manager (Alt+P).")

			// Show acknowledge modal with error
			a.showAcknowledgeModal = true
			a.acknowledgeModalTitle = "Plugin Startup Failures"
			a.acknowledgeModalMsg = errMsg.String()
			a.acknowledgeModalType = ModalTypeError
		}
		return a, nil

	case sessionLoadedMsg:
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

		// Set model from session
		if msg.Session.Model != "" {
			a.dataModel.OllamaClient.SetModel(msg.Session.Model)
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
			} else {
				// No partial file - just close modal
				a.sessionExportMode = false
				a.exportTargetPath = ""
				return a, nil
			}
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
			} else {
				// No partial file - just close modal
				a.dataExportMode = false
				a.dataExportTargetPath = ""
				return a, nil
			}
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

	// Plugin manager messages
	case installProgressMsg:
		if a.showPluginManager && a.pluginManagerState.installModal.visible {
			a.pluginManagerState.installModal.progress = mcp.InstallProgress(msg)
			// Keep listening for more progress updates
			if a.pluginManagerState.installModal.progress.Stage != "complete" && a.pluginManagerState.installModal.error == "" {
				return a, a.waitForInstallProgress
			}
		}
		return a, nil

	case installErrorMsg:
		if a.showPluginManager && a.pluginManagerState.installModal.visible {
			a.pluginManagerState.installModal.error = msg.err
		}
		return a, nil

	case uninstallProgressMsg:
		if a.showPluginManager && a.pluginManagerState.uninstallModal.visible {
			a.pluginManagerState.uninstallModal.progress = mcp.InstallProgress(msg)
			if msg.Stage == "error" {
				a.pluginManagerState.uninstallModal.error = msg.Message
			}
			if a.pluginManagerState.uninstallModal.progress.Stage != "complete" && a.pluginManagerState.uninstallModal.error == "" {
				return a, a.waitForUninstallProgress
			}
		}
		return a, nil

	case githubMetadataMsg:
		if a.showPluginManager && a.pluginManagerState.addCustomModal.visible {
			// Only apply if this is for the current repository URL
			currentRepoURL := a.pluginManagerState.addCustomModal.fields["repository"].Value()
			if msg.repoURL == currentRepoURL && msg.err == nil {
				// Auto-fill fields with GitHub metadata (only if empty)
				if msg.description != "" {
					descField := a.pluginManagerState.addCustomModal.fields["description"]
					if descField.Value() == "" {
						descField.SetValue(msg.description)
						a.pluginManagerState.addCustomModal.fields["description"] = descField
					}
				}

				if msg.language != "" {
					langField := a.pluginManagerState.addCustomModal.fields["language"]
					if langField.Value() == "" {
						langField.SetValue(msg.language)
						a.pluginManagerState.addCustomModal.fields["language"] = langField
					}
				}
			}
		}
		return a, nil

	// Settings modal custom messages
	case ollamaValidationMsg, dataDirectoryLoadedMsg, settingsSaveMsg:
		if a.showSettings {
			return a.handleSettingsInput(msg)
		}
		return a, nil

	}

	// Update textarea only if not streaming
	if !a.dataModel.Streaming {
		a.textarea, cmd = a.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}
