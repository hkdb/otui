package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"otui/config"
	"otui/ollama"
	"otui/storage"
)

func (a AppView) handleSessionManagerUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	// Handle delete confirmation
	if a.confirmDeleteSession != nil {
		switch msg.String() {
		case "y":
			sessionID := a.confirmDeleteSession.ID
			isDeletingCurrentSession := a.dataModel.CurrentSession != nil && a.dataModel.CurrentSession.ID == sessionID

			// Block deletion if current session is streaming
			if isDeletingCurrentSession && a.dataModel.Streaming {
				a.confirmDeleteSession = nil
				a.showAcknowledgeModal = true
				a.acknowledgeModalTitle = "Cannot Delete Session"
				a.acknowledgeModalMsg = "Session has an active response.\nCancel the response before deleting."
				a.acknowledgeModalType = ModalTypeWarning
				return a, nil
			}

			storage := a.dataModel.SessionStorage
			a.confirmDeleteSession = nil

			if isDeletingCurrentSession {
				// Unlock before deleting
				if a.dataModel.SessionStorage != nil {
					_ = a.dataModel.SessionStorage.UnlockSession(sessionID)
				}

				a.dataModel.Messages = []Message{}
				a.setCurrentSession(nil) // Clear and sync with MCP manager

				a.dataModel.SessionDirty = false
				a.textarea.Reset()
				a.updateViewportContent(true)
			}

			return a, func() tea.Msg {
				err := storage.Delete(sessionID)
				if err != nil {
					return sessionsListMsg{Err: err}
				}
				sessions, err := storage.List()
				return sessionsListMsg{
					Sessions: sessions,
					Err:      err,
				}
			}
		case "n", "esc":
			a.confirmDeleteSession = nil
			return a, nil
		}
		return a, nil
	}

	if a.sessionRenameMode {
		model, cmd := a.handleSessionRenameMode(msg)
		return model.(AppView), cmd
	}

	if a.sessionImportPicker.Active {
		if msg.String() == "esc" && a.sessionImportPicker.Processing && !a.sessionImportPicker.CleaningUp {
			if a.sessionImportCancelFunc != nil {
				a.sessionImportCancelFunc()
			}
			return a, nil
		}
		model, cmd := a.handleSessionImportMode(msg)
		return model.(AppView), cmd
	}

	if a.sessionExportMode {
		if msg.String() == "esc" && a.exportingSession && !a.exportCleaningUp {
			if a.exportCancelFunc != nil {
				a.exportCancelFunc()
			}
			return a, nil
		}
		model, cmd := a.handleSessionExportMode(msg)
		return model.(AppView), cmd
	}

	if a.sessionFilterMode {
		switch msg.String() {
		case "esc":
			a.sessionFilterMode = false
			a.sessionFilterInput.Blur()
			a.sessionFilterInput.SetValue("")
			a.filteredSessionList = []storage.SessionMetadata{}
			a.selectedSessionIdx = 0
			return a, nil

		case "enter":
			list := a.getSessionList()
			if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
				selectedSession := list[a.selectedSessionIdx]
				a.showSessionManager = false
				a.sessionFilterMode = false
				return a, a.dataModel.LoadSession(selectedSession.ID)
			}
			return a, nil

		case "alt+j", "alt+down", "down":
			list := a.getSessionList()
			if a.selectedSessionIdx < len(list)-1 {
				a.selectedSessionIdx++
			}
			return a, nil

		case "alt+k", "alt+up", "up":
			if a.selectedSessionIdx > 0 {
				a.selectedSessionIdx--
			}
			return a, nil
		}

		var cmd tea.Cmd
		a.sessionFilterInput, cmd = a.sessionFilterInput.Update(msg)

		filterValue := a.sessionFilterInput.Value()
		if filterValue == "" {
			a.filteredSessionList = a.sessionList
		} else {
			targets := make([]string, len(a.sessionList))
			for i, s := range a.sessionList {
				targets[i] = s.Name
			}

			matches := fuzzy.Find(filterValue, targets)
			a.filteredSessionList = make([]storage.SessionMetadata, len(matches))
			for i, match := range matches {
				a.filteredSessionList[i] = a.sessionList[match.Index]
			}
		}

		list := a.getSessionList()
		if a.selectedSessionIdx >= len(list) && len(list) > 0 {
			a.selectedSessionIdx = len(list) - 1
		}

		return a, cmd
	}

	switch msg.String() {
	case "/":
		if !a.sessionFilterMode {
			a.sessionFilterMode = true
			a.sessionFilterInput.Focus()
			a.sessionFilterInput.SetValue("")
			a.filteredSessionList = a.sessionList
			return a, textinput.Blink
		}
	case "esc":
		a.showSessionManager = false
		return a, nil
	case "j", "down":
		list := a.getSessionList()
		if a.selectedSessionIdx < len(list)-1 {
			a.selectedSessionIdx++
		}
		return a, nil
	case "k", "up":
		if a.selectedSessionIdx > 0 {
			a.selectedSessionIdx--
		}
		return a, nil
	case "enter":
		list := a.getSessionList()
		if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
			selectedSession := list[a.selectedSessionIdx]
			return a, a.dataModel.LoadSession(selectedSession.ID)
		}
		return a, nil
	case "n":
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
		a.showNewSessionModal = true
		a.newSessionFocusedField = 0
		a.newSessionPluginIdx = 0
		a.newSessionEnabledPlugins = []string{}
		a.newSessionNameInput.SetValue("")
		a.newSessionPromptInput.SetValue("")
		a.newSessionNameInput.Focus()
		a.newSessionPromptInput.Blur()
		return a, textinput.Blink
	case "i":
		a.sessionImportPicker.Activate()
		return a, a.sessionImportPicker.Picker.Init()
	case "r":
		list := a.getSessionList()
		if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
			if a.sessionRenameInput.Width == 0 {
				a.sessionRenameInput = textinput.New()
				a.sessionRenameInput.Width = 50
				a.sessionRenameInput.CharLimit = 100
			}
			a.sessionRenameMode = true
			a.sessionRenameInput.SetValue(list[a.selectedSessionIdx].Name)
			a.sessionRenameInput.Focus()
			return a, textinput.Blink
		}
		return a, nil
	case "e":
		list := a.getSessionList()
		if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
			sessionMeta := list[a.selectedSessionIdx]

			// Lazy init inputs (same pattern as "n" key)
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

			// Set edit mode with current values
			a.showEditSessionModal = true
			a.editSessionID = sessionMeta.ID
			a.newSessionFocusedField = 0
			a.editSessionPluginIdx = 0

			// Load full session to get enabled plugins
			fullSession, err := a.dataModel.SessionStorage.Load(sessionMeta.ID)
			if err == nil && fullSession != nil {
				a.editSessionEnabledPlugins = make([]string, len(fullSession.EnabledPlugins))
				copy(a.editSessionEnabledPlugins, fullSession.EnabledPlugins)
			} else {
				a.editSessionEnabledPlugins = []string{}
			}

			a.newSessionNameInput.SetValue(sessionMeta.Name)
			a.newSessionPromptInput.SetValue(sessionMeta.SystemPrompt)
			a.newSessionNameInput.Focus()
			a.newSessionPromptInput.Blur()
			return a, textinput.Blink
		}
		return a, nil
	case "x":
		list := a.getSessionList()
		if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
			if a.sessionExportInput.Width == 0 {
				a.sessionExportInput = textinput.New()
				a.sessionExportInput.Width = 70
				a.sessionExportInput.CharLimit = 500
			}
			sessionName := list[a.selectedSessionIdx].Name
			defaultPath := storage.GenerateExportPath(sessionName)
			a.sessionExportMode = true
			a.sessionExportInput.SetValue(defaultPath)
			a.sessionExportInput.Focus()
			return a, textinput.Blink
		}
		return a, nil
	case "d":
		list := a.getSessionList()
		if a.selectedSessionIdx >= 0 && a.selectedSessionIdx < len(list) {
			sessionMeta := list[a.selectedSessionIdx]
			a.confirmDeleteSession = &sessionMeta
		}
		return a, nil
	}
	return a, nil
}

func (a AppView) handleNewSessionModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showNewSessionModal = false
		a.newSessionNameInput.Blur()
		a.newSessionPromptInput.Blur()
		a.newSessionPluginIdx = 0
		a.newSessionEnabledPlugins = []string{}
		return a, nil

	case "tab":
		// Cycle through fields: 0=name, 1=prompt, 2=plugins
		switch a.newSessionFocusedField {
		case 0:
			a.newSessionFocusedField = 1
			a.newSessionNameInput.Blur()
			a.newSessionPromptInput.Focus()
		case 1:
			a.newSessionFocusedField = 2
			a.newSessionPromptInput.Blur()
		default:
			// From plugins back to name
			a.newSessionFocusedField = 0
			a.newSessionNameInput.Focus()
		}
		return a, textarea.Blink

	case "shift+tab":
		// Cycle backward through fields: 0=name, 2=plugins, 1=prompt
		switch a.newSessionFocusedField {
		case 0:
			a.newSessionFocusedField = 2
			a.newSessionNameInput.Blur()
		case 1:
			a.newSessionFocusedField = 0
			a.newSessionPromptInput.Blur()
			a.newSessionNameInput.Focus()
		default:
			// From plugins back to prompt
			a.newSessionFocusedField = 1
			a.newSessionPromptInput.Focus()
		}
		return a, textarea.Blink

	case "j", "down":
		// Navigate plugins if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get available plugins count (Layer 2 enabled)
			var availableCount int
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availableCount++
					}
				}
			}
			if a.newSessionPluginIdx < availableCount-1 {
				a.newSessionPluginIdx++
			}
			return a, nil
		}
		// Fall through to update input fields

	case "k", "up":
		// Navigate plugins if in plugin section
		if a.newSessionFocusedField == 2 {
			if a.newSessionPluginIdx > 0 {
				a.newSessionPluginIdx--
			}
			return a, nil
		}
		// Fall through to update input fields

	case "e":
		// Enable selected plugin if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get the selected plugin
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				var availablePlugins []string
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availablePlugins = append(availablePlugins, p.ID)
					}
				}

				if a.newSessionPluginIdx >= 0 && a.newSessionPluginIdx < len(availablePlugins) {
					selectedPluginID := availablePlugins[a.newSessionPluginIdx]

					// Check if already enabled
					alreadyEnabled := false
					for _, id := range a.newSessionEnabledPlugins {
						if id == selectedPluginID {
							alreadyEnabled = true
							break
						}
					}

					// Add to enabled list if not already enabled
					if !alreadyEnabled {
						a.newSessionEnabledPlugins = append(a.newSessionEnabledPlugins, selectedPluginID)
					}
				}
			}
			return a, nil
		}
		// Fall through to update input fields

	case "d":
		// Disable selected plugin if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get the selected plugin
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				var availablePlugins []string
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availablePlugins = append(availablePlugins, p.ID)
					}
				}

				if a.newSessionPluginIdx >= 0 && a.newSessionPluginIdx < len(availablePlugins) {
					selectedPluginID := availablePlugins[a.newSessionPluginIdx]

					// Remove from enabled list
					newEnabled := []string{}
					for _, id := range a.newSessionEnabledPlugins {
						if id != selectedPluginID {
							newEnabled = append(newEnabled, id)
						}
					}
					a.newSessionEnabledPlugins = newEnabled
				}
			}
			return a, nil
		}
		// Fall through to update input fields

	case "enter":
		if a.newSessionFocusedField == 1 {
			var cmd tea.Cmd
			a.newSessionPromptInput, cmd = a.newSessionPromptInput.Update(msg)
			return a, cmd
		}

		// Only create session if not in plugin navigation field
		if a.newSessionFocusedField != 2 {
			sessionName := strings.TrimSpace(a.newSessionNameInput.Value())
			systemPrompt := strings.TrimSpace(a.newSessionPromptInput.Value())

			// Create and save new session (shared implementation - fixes bug where session wasn't saved)
			newSession, err := a.dataModel.CreateAndSaveNewSession(sessionName, systemPrompt, a.newSessionEnabledPlugins)
			if err != nil {
				// If session creation fails, show error but don't crash
				if config.DebugLog != nil {
					config.DebugLog.Printf("Failed to create new Session: %v", err)
				}
				// Could show error modal here, but for now just close the modal
				a.showNewSessionModal = false
				a.newSessionNameInput.Blur()
				a.newSessionPromptInput.Blur()
				a.newSessionPluginIdx = 0
				a.newSessionEnabledPlugins = []string{}
				return a, nil
			}

			a.dataModel.Messages = []Message{}
			a.setCurrentSession(newSession) // Set and sync with MCP manager

			a.dataModel.SessionDirty = false
			a.showNewSessionModal = false
			a.showSessionManager = false
			a.newSessionNameInput.Blur()
			a.newSessionPromptInput.Blur()
			a.newSessionPluginIdx = 0
			a.newSessionEnabledPlugins = []string{}
			a.textarea.Reset()
			a.updateViewportContent(true)
			return a, nil
		}

	case "alt+enter":
		// Save from any field
		sessionName := strings.TrimSpace(a.newSessionNameInput.Value())
		systemPrompt := strings.TrimSpace(a.newSessionPromptInput.Value())

		// Create and save new session
		newSession, err := a.dataModel.CreateAndSaveNewSession(sessionName, systemPrompt, a.newSessionEnabledPlugins)
		if err != nil {
			// If session creation fails, show error but don't crash
			if config.DebugLog != nil {
				config.DebugLog.Printf("Failed to create new Session: %v", err)
			}
			// Could show error modal here, but for now just close the modal
			a.showNewSessionModal = false
			a.newSessionNameInput.Blur()
			a.newSessionPromptInput.Blur()
			a.newSessionPluginIdx = 0
			a.newSessionEnabledPlugins = []string{}
			return a, nil
		}

		a.dataModel.Messages = []Message{}
		a.setCurrentSession(newSession) // Set and sync with MCP manager

		a.dataModel.SessionDirty = false
		a.showNewSessionModal = false
		a.showSessionManager = false
		a.newSessionNameInput.Blur()
		a.newSessionPromptInput.Blur()
		a.newSessionPluginIdx = 0
		a.newSessionEnabledPlugins = []string{}
		a.textarea.Reset()
		a.updateViewportContent(true)
		return a, nil
	}

	// Update focused input field with the key (for fields 0 and 1)
	// This allows normal typing in name and system prompt fields
	var cmd tea.Cmd
	switch a.newSessionFocusedField {
	case 0:
		a.newSessionNameInput, cmd = a.newSessionNameInput.Update(msg)
	case 1:
		a.newSessionPromptInput, cmd = a.newSessionPromptInput.Update(msg)
	}
	// Field 2 (plugins) doesn't have an input component to update

	return a, cmd
}

func (a AppView) handleModelSelectorUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	// Handle model filter mode
	if a.modelFilterMode {
		switch msg.String() {
		case "esc":
			a.modelFilterMode = false
			a.modelFilterInput.Blur()
			a.modelFilterInput.SetValue("")
			a.filteredModelList = []ollama.ModelInfo{}
			a.selectedModelIdx = 0
			return a, nil

		case "enter":
			list := a.getModelList()
			if a.selectedModelIdx >= 0 && a.selectedModelIdx < len(list) {
				selectedModelInfo := list[a.selectedModelIdx]
				selectedModel := selectedModelInfo.Name

				// If we're in settings mode, update the settings field
				if a.showSettings {
					a.settingsFields[2].Value = selectedModel
					a.settingsHasChanges = true
					a.showModelSelector = false
					a.modelFilterMode = false
				} else {
					// Normal mode - check if model supports tools and warn if needed
					hasEnabledPlugins := a.dataModel.CurrentSession != nil && len(a.dataModel.CurrentSession.EnabledPlugins) > 0
					modelSupportsTools := ollama.ModelSupportsToolCalling(selectedModel)

					if hasEnabledPlugins && !modelSupportsTools {
						// Show warning modal
						a.pendingModelSwitch = selectedModel
						a.toolWarningPluginList = a.dataModel.MCPManager.GetSessionEnabledPluginNames(a.dataModel.CurrentSession)
						a.showToolWarningModal = true
						a.showModelSelector = false
						a.modelFilterMode = false
					} else {
						// No warning needed - proceed with switch (Phase 1.6: use SwitchModel)
						a.showModelSelector = false
						a.modelFilterMode = false
						return a, a.dataModel.SwitchModel(selectedModelInfo)
					}
				}
			}
			return a, nil

		case "alt+j", "alt+down", "down":
			list := a.getModelList()
			if a.selectedModelIdx < len(list)-1 {
				a.selectedModelIdx++
			}
			return a, nil

		case "alt+k", "alt+up", "up":
			if a.selectedModelIdx > 0 {
				a.selectedModelIdx--
			}
			return a, nil
		}

		var cmd tea.Cmd
		a.modelFilterInput, cmd = a.modelFilterInput.Update(msg)

		filterValue := a.modelFilterInput.Value()
		if filterValue == "" {
			a.filteredModelList = a.modelList
		} else {
			targets := make([]string, len(a.modelList))
			for i, mdl := range a.modelList {
				targets[i] = mdl.Name
			}

			matches := fuzzy.Find(filterValue, targets)
			a.filteredModelList = make([]ollama.ModelInfo, len(matches))
			for i, match := range matches {
				a.filteredModelList[i] = a.modelList[match.Index]
			}
		}

		list := a.getModelList()
		if a.selectedModelIdx >= len(list) && len(list) > 0 {
			a.selectedModelIdx = len(list) - 1
		}

		return a, cmd
	}

	// Normal model selector mode
	switch msg.String() {
	case "/":
		if !a.modelFilterMode {
			a.modelFilterMode = true
			a.modelFilterInput.Focus()
			a.modelFilterInput.SetValue("")
			a.filteredModelList = a.modelList
			return a, textinput.Blink
		}
	case "esc", "alt+m":
		a.showModelSelector = false
		return a, nil
	case "alt+r":
		// Refresh model list (user-initiated, keep selector open)
		return a, a.dataModel.FetchModelList(true)
	case "j", "down":
		list := a.getModelList()
		if a.selectedModelIdx < len(list)-1 {
			a.selectedModelIdx++
		}
		return a, nil
	case "k", "up":
		if a.selectedModelIdx > 0 {
			a.selectedModelIdx--
		}
		return a, nil
	case "enter":
		// Select model and close modal
		list := a.getModelList()
		if a.selectedModelIdx >= 0 && a.selectedModelIdx < len(list) {
			selectedModelInfo := list[a.selectedModelIdx]
			selectedModel := selectedModelInfo.Name

			// Settings mode - update the settings field
			if a.showSettings {
				a.settingsFields[2].Value = selectedModel
				a.settingsHasChanges = true
				a.showModelSelector = false
				return a, nil
			}

			// Normal mode - update the client (Phase 1.6: use SwitchModel)
			a.showModelSelector = false
			return a, a.dataModel.SwitchModel(selectedModelInfo)
		}
		return a, nil
	}
	return a, nil
}

func (a AppView) handleAboutUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	if msg.String() == "esc" || msg.String() == "alt+a" {
		a.showAbout = false
		return a, nil
	}
	return a, nil
}

func (a AppView) handleSettingsUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	model, cmd := a.handleSettingsInput(msg)
	return model.(AppView), cmd
}

func (a AppView) handleMessageSearchUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showMessageSearch = false
		return a, nil
	case "up", "alt+k":
		if a.selectedSearchIdx > 0 {
			a.selectedSearchIdx--
		}
		return a, nil
	case "down", "alt+j":
		if a.selectedSearchIdx < len(a.messageSearchResults)-1 {
			a.selectedSearchIdx++
		}
		return a, nil
	case "enter":
		if a.selectedSearchIdx >= 0 && a.selectedSearchIdx < len(a.messageSearchResults) {
			match := a.messageSearchResults[a.selectedSearchIdx]
			messageIdx := match.MessageIndex

			a.highlightedMessageIdx = messageIdx
			a.highlightFlashCount = 1
			a.showMessageSearch = false
			a.updateViewportContent(false)

			var offsetContent strings.Builder
			for i := 0; i < messageIdx; i++ {
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
			if centerOffset < 0 {
				centerOffset = 0
			}
			totalLines := a.viewport.TotalLineCount()
			if centerOffset > totalLines-viewportHeight {
				centerOffset = totalLines - viewportHeight
			}
			a.viewport.SetYOffset(centerOffset)

			return a, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return flashTickMsg{}
			})
		}
		return a, nil
	}

	var cmd tea.Cmd
	a.messageSearchInput, cmd = a.messageSearchInput.Update(msg)
	query := a.messageSearchInput.Value()
	if query != "" {
		// Convert []Message to []storage.Message
		storageMessages := make([]storage.Message, len(a.dataModel.Messages))
		for i, msg := range a.dataModel.Messages {
			storageMessages[i] = storage.Message{
				Role:      msg.Role,
				Content:   msg.Content,
				Rendered:  msg.Rendered,
				Timestamp: msg.Timestamp,
			}
		}
		a.messageSearchResults = storage.SearchMessages(storageMessages, query)
		a.selectedSearchIdx = 0
	} else {
		a.messageSearchResults = []storage.MessageMatch{}
	}
	return a, cmd
}

func (a AppView) handleGlobalSearchUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showGlobalSearch = false
		return a, nil
	case "up", "alt+k":
		if a.selectedGlobalIdx > 0 {
			a.selectedGlobalIdx--
		}
		return a, nil
	case "down", "alt+j":
		if a.selectedGlobalIdx < len(a.globalSearchResults)-1 {
			a.selectedGlobalIdx++
		}
		return a, nil
	case "enter":
		if a.selectedGlobalIdx >= 0 && a.selectedGlobalIdx < len(a.globalSearchResults) {
			selectedMatch := a.globalSearchResults[a.selectedGlobalIdx]
			a.showGlobalSearch = false
			a.pendingScrollToMessageIdx = selectedMatch.MessageIndex
			return a, a.dataModel.LoadSession(selectedMatch.SessionID)
		}
		return a, nil
	}

	var cmd tea.Cmd
	a.globalSearchInput, cmd = a.globalSearchInput.Update(msg)
	query := a.globalSearchInput.Value()
	if query != "" && a.dataModel.SearchIndex != nil {
		results, err := a.dataModel.SearchIndex.SearchAllSessions(query)
		if err == nil {
			a.globalSearchResults = results
			a.selectedGlobalIdx = 0
		}
	} else {
		a.globalSearchResults = []storage.SessionMessageMatch{}
	}
	return a, cmd
}

func (a AppView) handleEditSessionModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showEditSessionModal = false
		a.newSessionNameInput.Blur()
		a.newSessionPromptInput.Blur()
		a.editSessionID = ""
		return a, nil

	case "tab":
		// Cycle through fields: 0=name, 1=prompt, 2=plugins (edit mode only)
		switch a.newSessionFocusedField {
		case 0:
			a.newSessionFocusedField = 1
			a.newSessionNameInput.Blur()
			a.newSessionPromptInput.Focus()
		case 1:
			a.newSessionFocusedField = 2
			a.newSessionPromptInput.Blur()
		default:
			// From plugins back to name
			a.newSessionFocusedField = 0
			a.newSessionNameInput.Focus()
		}
		return a, textarea.Blink

	case "shift+tab":
		// Cycle backward through fields: 0=name, 2=plugins, 1=prompt
		switch a.newSessionFocusedField {
		case 0:
			a.newSessionFocusedField = 2
			a.newSessionNameInput.Blur()
		case 1:
			a.newSessionFocusedField = 0
			a.newSessionPromptInput.Blur()
			a.newSessionNameInput.Focus()
		default:
			// From plugins back to prompt
			a.newSessionFocusedField = 1
			a.newSessionPromptInput.Focus()
		}
		return a, textarea.Blink

	case "j", "down":
		// Navigate plugins if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get available plugins count (Layer 2 enabled)
			var availableCount int
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availableCount++
					}
				}
			}
			if a.editSessionPluginIdx < availableCount-1 {
				a.editSessionPluginIdx++
			}
			return a, nil
		}
		// Fall through to update input fields

	case "k", "up":
		// Navigate plugins if in plugin section
		if a.newSessionFocusedField == 2 {
			if a.editSessionPluginIdx > 0 {
				a.editSessionPluginIdx--
			}
			return a, nil
		}
		// Fall through to update input fields

	case "e":
		// Enable selected plugin if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get the selected plugin
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				var availablePlugins []string
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availablePlugins = append(availablePlugins, p.ID)
					}
				}

				if a.editSessionPluginIdx >= 0 && a.editSessionPluginIdx < len(availablePlugins) {
					selectedPluginID := availablePlugins[a.editSessionPluginIdx]

					// Check if already enabled
					alreadyEnabled := false
					for _, id := range a.editSessionEnabledPlugins {
						if id == selectedPluginID {
							alreadyEnabled = true
							break
						}
					}

					// Add to enabled list if not already enabled
					if !alreadyEnabled {
						a.editSessionEnabledPlugins = append(a.editSessionEnabledPlugins, selectedPluginID)
					}
				}
			}
			return a, nil
		}
		// Fall through to update input fields

	case "d":
		// Disable selected plugin if in plugin section
		if a.newSessionFocusedField == 2 {
			// Get the selected plugin
			if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
				allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()
				var availablePlugins []string
				for _, p := range allPlugins {
					if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
						availablePlugins = append(availablePlugins, p.ID)
					}
				}

				if a.editSessionPluginIdx >= 0 && a.editSessionPluginIdx < len(availablePlugins) {
					selectedPluginID := availablePlugins[a.editSessionPluginIdx]

					// Remove from enabled list
					newEnabled := []string{}
					for _, id := range a.editSessionEnabledPlugins {
						if id != selectedPluginID {
							newEnabled = append(newEnabled, id)
						}
					}
					a.editSessionEnabledPlugins = newEnabled
				}
			}
			return a, nil
		}
		// Fall through to update input fields

	case "alt+u":
		// Clear the focused field
		switch a.newSessionFocusedField {
		case 0: // Name field
			a.newSessionNameInput.SetValue("")
		case 1: // Prompt field
			a.newSessionPromptInput.SetValue("")
		}
		return a, nil

	case "alt+enter":
		// Save from any field
		newName := strings.TrimSpace(a.newSessionNameInput.Value())
		newSystemPrompt := strings.TrimSpace(a.newSessionPromptInput.Value())
		enabledPlugins := a.editSessionEnabledPlugins

		sessionID := a.editSessionID
		a.showEditSessionModal = false
		a.newSessionNameInput.Blur()
		a.newSessionPromptInput.Blur()
		a.editSessionID = ""

		return a, a.dataModel.UpdateSessionPropertiesCmd(sessionID, newName, newSystemPrompt, enabledPlugins)
	}

	// Update focused input field with the key (for fields 0 and 1)
	// This allows normal typing in name and system prompt fields
	var cmd tea.Cmd
	switch a.newSessionFocusedField {
	case 0:
		a.newSessionNameInput, cmd = a.newSessionNameInput.Update(msg)
	case 1:
		a.newSessionPromptInput, cmd = a.newSessionPromptInput.Update(msg)
	}
	// Field 2 (plugins) doesn't have an input component to update

	return a, cmd
}

func (a AppView) handleToolWarningModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		// User confirmed - proceed with model switch (Phase 1.6: use SwitchModel)
		// Find the model info from the list
		var selectedModelInfo ollama.ModelInfo
		for _, model := range a.modelList {
			if model.Name == a.pendingModelSwitch {
				selectedModelInfo = model
				break
			}
		}

		a.showToolWarningModal = false
		a.pendingModelSwitch = ""
		a.toolWarningPluginList = nil
		return a, a.dataModel.SwitchModel(selectedModelInfo)

	case "esc":
		// User cancelled - don't switch model
		a.showToolWarningModal = false
		a.pendingModelSwitch = ""
		a.toolWarningPluginList = nil
		return a, nil
	}

	return a, nil
}

// handleSystemPromptToolWarningUpdate handles key presses when the system prompt + tools warning modal is shown
func (a AppView) handleSystemPromptToolWarningUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// User chose to send anyway - mark warning as shown and proceed with sending
		a.systemPromptToolWarningShown = true
		a.showSystemPromptToolWarning = false

		// Now send the message (copy the logic from the Enter handler)
		if a.textarea.Value() != "" {
			userMsg := a.textarea.Value()
			a.textarea.Reset()

			// Clear editor temp file (defense in depth)
			if err := config.ClearEditorTempFile(); err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("Warning: failed to clear editor temp file: %v", err)
				}
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("System prompt warning acknowledged - sending message: %s", userMsg)
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

		return a, nil

	case "esc":
		// User cancelled - don't send message, restore textarea content
		a.showSystemPromptToolWarning = false
		// Don't mark as shown - user might want to try again later
		return a, nil
	}

	return a, nil
}
