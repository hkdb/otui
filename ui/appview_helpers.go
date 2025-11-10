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

	if config.DebugLog != nil {
		config.DebugLog.Printf("[AppView] ensureMCPManager: Manager is nil, attempting to recreate")
	}

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
		if config.DebugLog != nil {
			config.DebugLog.Printf("[AppView] ensureMCPManager: Failed to create plugin storage: %v", err)
		}
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

	if config.DebugLog != nil {
		config.DebugLog.Printf("[AppView] ensureMCPManager: Manager successfully recreated")
	}

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
	// Check if any plugins actually running (use existing function)
	if a.dataModel.MCPManager == nil {
		return nil
	}

	if len(a.dataModel.MCPManager.GetActivePluginNames()) == 0 {
		return nil
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Initiating plugin shutdown with modal (callback: %s)", callbackType)
	}

	// Show shutdown modal (existing pattern from settings.go:177-185)
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

	// Return shutdown command (reuse existing function)
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
		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] Cannot start plugins - no MCP manager")
		}
		return nil
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Starting plugin system with modal")
	}

	// Show startup modal (existing pattern from settings.go:256-264)
	a.pluginSystemState = PluginSystemState{
		Active:    true,
		Operation: "starting",
		Phase:     "waiting",
		Spinner:   spinner.New(),
		StartTime: time.Now(),
	}
	a.pluginSystemState.Spinner.Spinner = spinner.Dot
	a.pluginSystemState.Spinner.Style = lipgloss.NewStyle().Foreground(successColor)

	// Return startup command (reuse existing function)
	return tea.Batch(
		a.pluginSystemState.Spinner.Tick,
		startPluginSystemCmd(a.dataModel.MCPManager, session),
	)
}

// switchDataDirectory orchestrates all 6 steps of data directory switching.
// CRITICAL: Each step must complete BEFORE the next step (synchronous execution).
// Rule #11: Follows user's explicit 6-step sequential requirement from day 1.
func (a *AppView) switchDataDirectory(newDataDir string) tea.Cmd {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Starting data directory switch to: %s", newDataDir)
	}

	// STEP 1: Shutdown plugins if running (async with animated modal)
	if a.dataModel.MCPManager != nil {
		activePlugins := a.dataModel.MCPManager.GetActivePluginNames()
		if len(activePlugins) > 0 {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] STEP 1: Initiating async plugin shutdown with modal")
			}

			// Use existing async shutdown helper - will callback to complete steps 2-6
			return a.shutdownPluginSystemWithModal("datadir_switch", newDataDir)
		}
	}

	// No plugins running - proceed directly to steps 2-6
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] STEP 1: No plugins running - proceeding with switch")
	}
	return a.completeDataDirSwitch(newDataDir)
}

// completeDataDirSwitch executes STEPS 2-6 after plugin shutdown (or immediately if no plugins).
// Called either by switchDataDirectory() directly, or by pluginSystemOperationMsg handler.
func (a *AppView) completeDataDirSwitch(newDataDir string) tea.Cmd {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Completing data dir switch: STEPS 2-6")
	}

	// STEP 2-3: Switch data directory (try without passphrase first)
	// Model will load config from new data dir and may need passphrase
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

	// STEP 3c: Re-initialize providers with new config (shared helper)
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] STEP 3c: Re-initializing providers")
	}
	providerRefreshCmd := a.refreshProvidersAndModels()

	// STEP 4: Re-create MCP manager if plugins enabled (use existing function)
	if a.dataModel.Config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] STEP 4: Recreating MCP manager")
		}
		if err := a.ensureMCPManager(); err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[UI] STEP 4 failed: %v - continuing without plugins", err)
			}
		}
	}

	// STEP 6: Refresh all UI components (use existing function)
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] STEP 6: Refreshing UI components")
	}
	a.resetUIStateForDataDirSwitch()

	// STEP 5: Start plugins if any enabled (use new helper)
	if a.dataModel.Config.PluginsEnabled && a.dataModel.MCPManager != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[UI] STEP 5: Starting enabled plugins")
		}
		return tea.Batch(
			a.startPluginSystemWithModal(nil),
			a.dataModel.FetchSessionList(),
			providerRefreshCmd,
		)
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] STEP 5: No plugins to start")
	}
	return tea.Batch(
		a.dataModel.FetchSessionList(),
		providerRefreshCmd,
	)
}

// resetUIStateForDataDirSwitch clears all data-directory-dependent UI caches.
// This ensures UI components fetch fresh data from the new data directory.
// Does NOT touch model state (model/ owns that).
//
// STEP 6: Refresh all different UI components (per user's original requirements)
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

	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] STEP 6 complete: All UI state caches reset")
	}
}

// refreshProvidersAndModels re-initializes all providers and fetches models.
// Called after config changes (provider settings, data dir switch, etc.)
// This is the SINGLE place where provider refresh logic lives.
//
// Rule #8: Shareable component - reused across settings changes and data dir switch
// Rule #11: Reuses existing InitializeProviders() pattern from completeDataDirSwitch()
func (a *AppView) refreshProvidersAndModels() tea.Cmd {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] ========== refreshProvidersAndModels CALLED ==========")
		config.DebugLog.Printf("[UI] Config has %d providers:", len(a.dataModel.Config.Providers))
		for _, p := range a.dataModel.Config.Providers {
			config.DebugLog.Printf("[UI]   Provider: %s, Enabled: %v", p.ID, p.Enabled)
		}
	}

	// Re-initialize providers with current config
	allProviders := provider.InitializeProviders(a.dataModel.Config)

	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Initialized %d providers:", len(allProviders))
		for id := range allProviders {
			config.DebugLog.Printf("[UI]   Provider initialized: %s", id)
		}
	}

	a.dataModel.Providers = allProviders
	a.dataModel.Provider = allProviders[a.dataModel.Config.DefaultProvider]
	if a.dataModel.Provider == nil {
		a.dataModel.Provider = allProviders["ollama"]
	}

	// Clear cache and fetch models from all enabled providers (background refresh)
	// This ensures newly enabled providers' models are available when user opens selector
	if config.DebugLog != nil {
		config.DebugLog.Printf("[UI] Clearing model cache and fetching from all enabled providers")
	}
	a.dataModel.ClearModelCache("")
	return a.dataModel.FetchAllModels(false) // Background refresh, don't show selector
}
