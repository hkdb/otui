package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	"otui/mcp"
	"otui/ollama"
	"otui/provider"
	"otui/safetext"
	"otui/storage"
)

// boolToString converts a boolean to its string representation
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// stringToBool converts a string to boolean ("true" -> true, anything else -> false)
func stringToBool(s string) bool {
	return s == "true"
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ensureMCPManager ensures the MCP manager exists, recreating it if necessary.
// This restores the architectural invariant that plugin infrastructure persists
// throughout the app lifetime, even when plugins are disabled.
//
// Returns error if manager cannot be created due to missing prerequisites.
func (a *AppView) ensureMCPManager() error {
	// Manager already exists - nothing to do
	if a.dataModel.MCPManager != nil {
		return nil
	}

	config.Debugf("[AppView] ensureMCPManager: Manager is nil, attempting to recreate")

	// Validate prerequisites exist
	if a.dataModel.Plugins == nil {
		return fmt.Errorf("plugin state not initialized")
	}
	if a.dataModel.Plugins.Registry == nil {
		return fmt.Errorf("plugin registry not available")
	}
	if a.dataModel.Plugins.Config == nil {
		return fmt.Errorf("plugin config not available")
	}

	// Create plugin storage (might have been destroyed with manager)
	pluginStorage, err := storage.NewPluginStorage(a.dataModel.Config.DataDir())
	if err != nil {
		config.Debugf("[AppView] ensureMCPManager: Failed to create plugin storage: %v", err)
		return fmt.Errorf("failed to create plugin storage: %w", err)
	}

	// Recreate MCP manager
	a.dataModel.MCPManager = mcp.NewMCPManager(
		a.dataModel.Config,
		pluginStorage,
		a.dataModel.Plugins.Config,
		a.dataModel.Plugins.Registry,
		a.dataModel.Config.DataDir(),
	)

	config.Debugf("[AppView] ensureMCPManager: Manager successfully recreated")

	return nil
}

// shutdownPluginSystemWithModal initiates plugin shutdown with progress modal.
// Wraps existing stopPluginSystemCmd() with modal boilerplate (reusable pattern).
//
// Parameters:
//   - callbackType: What to do after shutdown ("datadir_switch", "plugin_disable", "app_quit")
//   - callbackData: Optional data for callback (e.g., newDataDir string)
//
// Returns: tea.Cmd to execute shutdown, or nil if no plugins running
func (a *AppView) shutdownPluginSystemWithModal(callbackType string, callbackData interface{}) tea.Cmd {
	// Check if any plugins are actually running
	if a.dataModel.MCPManager == nil {
		return nil
	}

	if len(a.dataModel.MCPManager.GetActivePluginNames()) == 0 {
		return nil
	}

	config.Debugf("[UI] Initiating plugin shutdown with modal (callback: %s)", callbackType)

	// Show the shutdown modal
	a.pluginSystemState = PluginSystemState{
		Active:    true,
		Operation: "stopping",
		Phase:     "waiting",
		Spinner:   spinner.New(),
		StartTime: time.Now(),
	}
	a.pluginSystemState.Spinner.Spinner = spinner.Dot

	// Store callback info
	a.pendingPluginOperation = callbackType
	a.pendingPluginOperationData = callbackData

	// Return shutdown command
	return tea.Batch(
		a.pluginSystemState.Spinner.Tick,
		stopPluginSystemCmd(a.dataModel.MCPManager),
	)
}

// startPluginSystemWithModal initiates plugin startup with progress modal.
// Wraps existing startPluginSystemCmd() with modal boilerplate (reusable pattern).
//
// Parameters:
//   - session: Optional session to set on manager before starting (nil = no session)
//
// Returns: tea.Cmd to execute startup, or nil if manager doesn't exist
func (a *AppView) startPluginSystemWithModal(session *storage.Session) tea.Cmd {
	if a.dataModel.MCPManager == nil {
		config.Debugf("[UI] Cannot start plugins - no MCP manager")
		return nil
	}

	config.Debugf("[UI] Starting plugin system with modal")

	// Show the startup modal
	a.pluginSystemState = PluginSystemState{
		Active:    true,
		Operation: "starting",
		Phase:     "waiting",
		Spinner:   spinner.New(),
		StartTime: time.Now(),
	}
	a.pluginSystemState.Spinner.Spinner = spinner.Dot
	a.pluginSystemState.Spinner.Style = lipgloss.NewStyle().Foreground(successColor)

	// Return startup command
	return tea.Batch(
		a.pluginSystemState.Spinner.Tick,
		startPluginSystemCmd(a.dataModel.MCPManager, session),
	)
}

// switchDataDirectory switches to another data directory. Running plugins
// are stopped first, behind the shutdown modal, whose result then calls
// completeDataDirSwitch. Each step finishes before the next starts.
func (a *AppView) switchDataDirectory(newDataDir string) tea.Cmd {
	config.Debugf("[UI] Starting data directory switch to: %s", newDataDir)

	// Stop running plugins first
	if a.dataModel.MCPManager != nil {
		activePlugins := a.dataModel.MCPManager.GetActivePluginNames()
		if len(activePlugins) > 0 {
			config.Debugf("[UI] Stopping plugins before the switch")
			return a.shutdownPluginSystemWithModal("datadir_switch", newDataDir)
		}
	}

	config.Debugf("[UI] No plugins running - switching now")
	return a.completeDataDirSwitch(newDataDir)
}

// completeDataDirSwitch switches the data directory once no plugins are
// running, then reinitializes providers, the plugin system and UI state.
// Called by switchDataDirectory or, after plugin shutdown, by the
// pluginSystemOperationMsg handler.
func (a *AppView) completeDataDirSwitch(newDataDir string) tea.Cmd {
	config.Debugf("[UI] Completing data dir switch")

	// Try without a passphrase first (the new data dir's credentials may need one)
	if err := a.dataModel.ApplyDataDirSwitch(newDataDir, ""); err != nil {
		// Check if SSH passphrase is required
		if strings.Contains(err.Error(), "passphrase required") {
			// Extract SSH key path from new data dir config
			systemCfg, _ := config.LoadSystemConfig()
			keyPath := ""
			if userCfg, err := config.LoadUserConfig(config.ExpandPath(systemCfg.DataDirectory)); err == nil {
				keyPath = userCfg.Security.SSHKeyPath
			}

			// Show passphrase modal, store newDataDir to retry after passphrase
			a.showPassphraseForDataDir = true
			a.passphraseSSHKeyPath = keyPath
			a.passphraseRetryDataDir = newDataDir
			a.passphraseError = ""
			a.passphraseForDataDir.Focus()
			return textinput.Blink
		}

		// Other errors (not passphrase related)
		a.showAcknowledgeModal = true
		a.acknowledgeModalTitle = "⚠️  Data Directory Switch Failed"
		a.acknowledgeModalMsg = fmt.Sprintf("Failed to switch data directory:\n\n%v", err)
		a.acknowledgeModalType = ModalTypeError
		return nil
	}

	// Reinitialize providers with the new config
	config.Debugf("[UI] Reinitializing providers")
	providerRefreshCmd := a.refreshProvidersAndModels()

	if a.dataModel.Config.PluginsEnabled {
		config.Debugf("[UI] Recreating MCP manager")
		if err := a.ensureMCPManager(); err != nil {
			config.Debugf("[UI] Recreating MCP manager failed: %v - continuing without plugins", err)
		}
	}

	config.Debugf("[UI] Resetting UI state")
	a.resetUIStateForDataDirSwitch()

	if a.dataModel.Config.PluginsEnabled && a.dataModel.MCPManager != nil {
		config.Debugf("[UI] Starting enabled plugins")
		return tea.Batch(
			a.startPluginSystemWithModal(nil),
			a.dataModel.FetchSessionList(),
			providerRefreshCmd,
		)
	}

	config.Debugf("[UI] No plugins to start")
	return tea.Batch(
		a.dataModel.FetchSessionList(),
		providerRefreshCmd,
	)
}

// resetUIStateForDataDirSwitch clears all data-directory-dependent UI caches.
// This ensures UI components fetch fresh data from the new data directory.
// Does NOT touch model state (model/ owns that).
func (a *AppView) resetUIStateForDataDirSwitch() {
	// Close all modals (except plugin system modal if showing progress)
	if a.pluginSystemState.Operation != "stopping" && a.pluginSystemState.Operation != "starting" {
		a.closeAllModals()
	}

	// Clear provider settings cache
	a.providerSettingsState = ProviderSettingsState{
		visible:            false,
		selectedProviderID: "ollama",
		selectedFieldIdx:   0,
		editMode:           false,
		editInput:          textinput.New(),
		hasChanges:         false,
		saveError:          "",
	}

	// Clear model cache (UI state only - model cache cleared by model)
	a.modelListCached = false
	a.modelList = []ollama.ModelInfo{}
	a.filteredModelList = []ollama.ModelInfo{}
	a.showModelSelector = false
	a.modelFilterMode = false

	// Reset session manager state
	a.showSessionManager = false
	a.sessionRenameMode = false
	a.sessionRenameInput.SetValue("")
	a.sessionFilterMode = false
	a.sessionFilterInput.SetValue("")
	a.filteredSessionList = nil
	a.sessionExportMode = false
	a.sessionExportSuccess = ""

	// Reset plugin manager state
	a.pluginManagerState.selection.selectedPluginIdx = 0
	a.pluginManagerState.selection.scrollOffset = 0
	a.pluginManagerState.selection.filterMode = false
	a.pluginManagerState.selection.filterInput.SetValue("")
	a.pluginManagerState.selection.filteredPlugins = nil
	a.pluginManagerState.selection.viewMode = "curated"
	a.pluginManagerState.registryRefresh.visible = false
	a.pluginManagerState.installModal.visible = false
	a.pluginManagerState.uninstallModal.visible = false

	// Reset search state
	a.showMessageSearch = false
	a.messageSearchInput.SetValue("")
	a.messageSearchResults = nil

	// Reset viewport
	a.updateViewportContent(true)
	a.viewport.SetYOffset(0)

	config.Debugf("[UI] UI state reset for the new data dir")
}

// removeLastNonPersistentSystemMessage removes the last system message if it's not persistent
// Used to clean up transient messages like "Waiting for response..." or "Analyzing results..."
func (a *AppView) removeLastNonPersistentSystemMessage() {
	if len(a.dataModel.Messages) == 0 {
		return
	}
	lastIdx := len(a.dataModel.Messages) - 1
	if a.dataModel.Messages[lastIdx].Role != "system" {
		return
	}
	if a.dataModel.Messages[lastIdx].Persistent {
		return
	}
	a.dataModel.Messages = a.dataModel.Messages[:lastIdx]
}

// createStepMessage creates a multi-step progress message with a spinner
func (a *AppView) createStepMessage(purpose string, stepNum int) {
	// The purpose is model-written: one line, limited length
	purpose = truncateWidth(safetext.Line(purpose), maxPurposeWidth)
	systemMsg := fmt.Sprintf("🔧 Step %d: %s...", stepNum, purpose)
	a.dataModel.Messages = append(a.dataModel.Messages, Message{
		Role:      "system",
		Content:   systemMsg,
		Rendered:  systemMsg,
		Timestamp: time.Now(),
	})
	a.updateViewportContent(!a.userScrolledUp)
}

// completeStepMessage adds checkmark to last step message and makes it persistent
func (a *AppView) completeStepMessage() {
	for i := len(a.dataModel.Messages) - 1; i >= 0; i-- {
		msgPtr := &a.dataModel.Messages[i]
		if msgPtr.Role != "system" {
			continue
		}
		if !strings.HasPrefix(msgPtr.Content, "🔧 ") {
			continue
		}

		checkmark := AssistantStyle.Render("✓")
		content := strings.TrimSpace(msgPtr.Content)

		// Remove spinner characters
		for _, spinChar := range []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"} {
			if idx := strings.LastIndex(content, spinChar); idx != -1 {
				content = content[:idx]
				break
			}
		}

		msgPtr.Content = strings.TrimSpace(content) + " " + checkmark
		msgPtr.Rendered = msgPtr.Content
		msgPtr.Persistent = true
		return
	}
}

// showAnalyzingResults sets streaming state and shows "Analyzing results..." message
func (a *AppView) showAnalyzingResults() {
	a.dataModel.Streaming = true
	a.dataModel.Messages = append(a.dataModel.Messages, Message{
		Role:      "system",
		Content:   "Analyzing results...",
		Rendered:  "Analyzing results...",
		Timestamp: time.Now(),
	})
	a.updateViewportContent(!a.userScrolledUp)
}

// staleRequest reports whether a message belongs to a request that was
// cancelled or replaced by a newer one
func (a *AppView) staleRequest(requestID uint64) bool {
	if requestID != a.dataModel.RequestID() {
		config.Debugf("Ignoring message of stale request %d (current %d)", requestID, a.dataModel.RequestID())
		return true
	}
	return false
}

// getPluginShortName extracts plugin ID from tool name and gets short display name
func (a *AppView) getPluginShortName(toolName string) string {
	pluginID := toolName
	if idx := strings.Index(toolName, "."); idx != -1 {
		pluginID = toolName[:idx]
	}

	if a.dataModel.MCPManager == nil {
		return pluginID
	}

	shortName := a.dataModel.MCPManager.GetPluginShortName(pluginID)
	if shortName == "" {
		return pluginID
	}
	return shortName
}

// startToolExecution sets up tool execution state
func (a *AppView) startToolExecution(toolName string) {
	a.executingTool = a.getPluginShortName(toolName)
	a.dataModel.Streaming = true
	// Initialize spinner for header indicator
	a.toolExecutionSpinner = spinner.New()
	a.toolExecutionSpinner.Spinner = spinner.Dot
}

// refreshProvidersAndModels re-initializes all providers and fetches models.
// Called after config changes (provider settings, data dir switch, etc.)
func (a *AppView) refreshProvidersAndModels() tea.Cmd {
	if config.DebugEnabled() {
		config.Debugf("[UI] refreshProvidersAndModels: config has %d providers:", len(a.dataModel.Config.Providers))
		for _, p := range a.dataModel.Config.Providers {
			config.Debugf("[UI]   Provider: %s, Enabled: %v", p.ID, p.Enabled)
		}
	}

	// Re-initialize providers with current config
	allProviders := provider.InitializeProviders(a.dataModel.Config)

	if config.DebugEnabled() {
		config.Debugf("[UI] Initialized %d providers:", len(allProviders))
		for id := range allProviders {
			config.Debugf("[UI]   Provider initialized: %s", id)
		}
	}

	a.dataModel.Providers = allProviders
	a.dataModel.Provider = allProviders[a.dataModel.Config.DefaultProvider]
	if a.dataModel.Provider == nil {
		a.dataModel.Provider = allProviders["ollama"]
	}

	// Clear cache and fetch models from all enabled providers (background refresh)
	// This ensures newly enabled providers' models are available when user opens selector
	config.Debugf("[UI] Clearing model cache and fetching from all enabled providers")
	a.dataModel.ClearModelCache("")

	// Provider settings (e.g. base URL) affect context windows - refetch
	a.dataModel.ClearModelMetadata()

	return tea.Batch(
		a.dataModel.FetchAllModels(false), // Background refresh, don't show selector
		a.dataModel.RefreshModelMetadataCmd(),
	)
}

// cancelRequest cancels the in-progress request (Esc): the provider call and
// any tool calls are stopped, their results will be ignored, and the
// response and tool step state is reset
func (a *AppView) cancelRequest() {
	a.dataModel.CancelRequest()
	a.flushTypewriter() // Keep everything that arrived
	a.dataModel.Streaming = false
	a.userScrolledUp = false

	a.executingTool = ""
	a.iterationCount = 0
	a.dataModel.ResetIterations()

	// Tools approved "once" for this request
	*a = a.cleanupTemporaryTools()
}

// blockedByActiveResponse shows a warning and returns true if a response is
// in progress. Used before actions that switch sessions or data directories:
// the response would otherwise end up in the other session.
func (a *AppView) blockedByActiveResponse(action string) bool {
	if !a.dataModel.Streaming {
		return false
	}
	a.showAcknowledgeModal = true
	a.acknowledgeModalTitle = "Response in Progress"
	a.acknowledgeModalMsg = fmt.Sprintf("A response is in progress.\nWait for it to finish or cancel it with Esc before %s.", action)
	a.acknowledgeModalType = ModalTypeWarning
	return true
}

// cancelRequestOnSwitch cancels a request still in progress when the current
// session is replaced, so its results can't end up in the new session
func (a *AppView) cancelRequestOnSwitch() {
	if !a.dataModel.Streaming && !a.waitingForPermission {
		return
	}
	config.Debugf("[Session] Switching while a response is in progress - cancelling it")
	a.cancelRequest()
	a.currentResp.Reset()
	a.waitingForPermission = false
	a.pendingPermission = nil
}
