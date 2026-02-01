package ui

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"otui/config"
	"otui/mcp"
	"otui/provider"
	"otui/storage"
)

type ollamaValidationMsg struct {
	success bool
	err     error
}

type dataDirectoryLoadedMsg struct {
	normalizedPath string
	configLoaded   bool
	ollamaHost     string
	defaultModel   string
	systemPrompt   string
	pluginsEnabled bool
	err            error
}

type settingsSaveMsg struct {
	success bool
	err     error
}

type dataDirectoryNotFoundMsg struct {
	path string
}

func (a AppView) handleSettingsInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[Settings] ========== handleSettingsInput CALLED ==========")
		config.DebugLog.Printf("[Settings] Message type: %T", msg)
		config.DebugLog.Printf("[Settings] providerSettingsState.visible = %v", a.providerSettingsState.visible)
	}

	// Route to provider settings sub-screen if visible (keyboard input only)
	if a.providerSettingsState.visible {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings] Routing to provider settings handler")
		}
		switch msg := msg.(type) {
		case tea.KeyMsg:
			return a.handleProviderSettingsInput(msg)
		}
		return a, nil
	}

	// Handle provider save messages regardless of visibility (fixes race condition where
	// user presses Alt+Enter then immediately Esc before message arrives)
	switch msg := msg.(type) {
	case providerFieldSavedMsg:
		// Handle save result (single field - legacy)
		switch {
		case msg.success:
			// Update config in memory
			a.dataModel.Config = msg.cfg
		case !msg.success:
			// Show error (for now, just ignore - could add error display later)
			a.providerSettingsState.saveError = msg.err.Error()
		}
		return a, nil
	case providerFieldsSavedMsg:
		// Handle batch save result (all providers' fields)
		switch {
		case msg.success:
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] ========== providerFieldsSavedMsg SUCCESS ==========")
			}

			// Update config in memory
			a.dataModel.Config = msg.cfg

			// Refresh provider settings cache (only if map exists - screen may be closed)
			if a.providerSettingsState.currentFieldsMap != nil {
				for _, providerID := range providerTabs {
					a.providerSettingsState.currentFieldsMap[providerID] = a.providerSettingsState.getProviderFields(providerID, msg.cfg)
				}
			}

			// Re-initialize providers and fetch models (shared helper with debug logging)
			return a, a.refreshProvidersAndModels()
		case !msg.success:
			// Show error
			a.providerSettingsState.saveError = msg.err.Error()
		}
		return a, nil
	}

	switch msg := msg.(type) {
	case dataDirectoryNotFoundMsg:
		// Data directory doesn't exist - show confirmation modal
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings] Showing new data dir confirmation for: %s", msg.path)
		}
		a.settingsDataDirNotFound = true
		a.settingsNewDataDirPath = msg.path
		return a, nil

	case dataDirectoryLoadedMsg:
		if msg.err != nil {
			// Show error and clear dependent fields
			a.settingsFields[0].Validation = FieldValidationError
			a.settingsFields[0].ErrorMsg = msg.err.Error()
			a.settingsFields[1].Value = ""
			a.settingsFields[2].Value = ""
			a.settingsFields[3].Value = ""
			a.settingsFields[4].Value = ""
			a.settingsLoadedInfo = ""
			return a, nil
		}

		a.settingsFields[0].Value = msg.normalizedPath
		a.settingsFields[0].Validation = FieldValidationNone

		if msg.configLoaded {
			// Update ALL fields from new data directory config
			a.settingsFields[1].Value = msg.ollamaHost
			a.settingsFields[2].Value = msg.defaultModel
			a.settingsFields[3].Value = msg.systemPrompt
			a.settingsFields[4].Value = boolToString(msg.pluginsEnabled)
			a.settingsLoadedInfo = "ℹ Loaded config from data directory"
			a.settingsHasChanges = true
			// Trigger validation of new Ollama host
			return a, validateOllamaHostCmd(msg.ollamaHost)
		} else {
			// No config found - clear all dependent fields
			a.settingsFields[1].Value = ""
			a.settingsFields[2].Value = ""
			a.settingsFields[3].Value = ""
			a.settingsFields[4].Value = ""
			a.settingsLoadedInfo = ""
		}

		return a, nil

	case settingsSaveMsg:
		if !msg.success {
			// Show error inline in settings modal
			a.settingsSaveError = msg.err.Error()
			return a, nil
		}

		// ========== DATA DIRECTORY CHANGE & PLUGIN LIFECYCLE ORCHESTRATION ==========
		//
		// Flow for handling data directory changes with plugins:
		//
		// SCENARIO 1: Switch from data dir A (plugins enabled) → data dir B (plugins disabled)
		//   1. STEP 1 (line 126-154): Shutdown plugins from old data dir, return early with modal
		//   2. pluginSystemOperationMsg handler (appview_update.go:1283): Destroy manager, dismiss modal
		//   3. User manually saves settings again (or shutdown completes and auto-continues)
		//   4. STEP 2 (line 159-256): Validate and switch to new data dir
		//   5. STEP 5 (line 324): Fetch session list, load first session
		//   6. sessionLoadedMsg handler: Plugins not enabled, nothing to do ✓
		//
		// SCENARIO 2: Switch from data dir A (plugins disabled) → data dir B (plugins enabled)
		//   1. STEP 1 (line 126-154): No plugins running, skip
		//   2. STEP 2 (line 159-256): Validate and switch to new data dir, clear session to nil
		//   3. STEP 3 (line 260-305): Condition FALSE (data dir changed), skip plugin enabling
		//   4. STEP 5 (line 324): Fetch session list, load first session
		//   5. sessionLoadedMsg handler (appview_update.go:~1512): Create manager, start plugins ✓
		//
		// SCENARIO 3: Just toggle plugins (no data dir change)
		//   1. STEP 1 (line 126-154): If disabling, shutdown and return
		//   2. STEP 2 (line 159-256): Skip (data dir didn't change)
		//   3. STEP 3 (line 260-305): If enabling, create manager and start plugins ✓
		//   4. STEP 5 (line 324): Fetch session list (refresh)
		//
		// NOTE: This orchestration logic lives in UI because Bubbletea has no controller layer.
		//       Business logic (shutdown, startup, manager lifecycle) is in model/ and mcp/ packages.
		//       UI just orchestrates: "when to call what, in what order" (Update pattern).
		//
		// ============================================================================

		// Preserve current state for rollback (capture BEFORE config reload)
		oldDataDir := a.dataModel.Config.DataDir()
		oldHost := a.dataModel.Config.OllamaURL()
		oldPluginsEnabled := a.dataModel.Config.PluginsEnabled
		currentModel := a.dataModel.Provider.GetModel()

		// Reload config immediately after successful save to pick up fresh values
		// This was removed during data dir refactor but is needed for ALL settings saves
		// (not just data dir changes). Fixes regression from commit 094e265.
		cfg, err := config.Load()
		if err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings Save] ERROR reloading config: %v", err)
			}
			a.showAcknowledgeModal = true
			a.acknowledgeModalTitle = "⚠️  Settings Save Error"
			a.acknowledgeModalMsg = fmt.Sprintf("Settings were saved to disk but failed to reload:\n\n%v\n\nPlease restart OTUI to ensure changes take effect.", err)
			a.acknowledgeModalType = ModalTypeError
			return a, nil
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings Save] Config reloaded successfully: PluginsEnabled=%v, DefaultSystemPrompt=%q",
				cfg.PluginsEnabled, cfg.DefaultSystemPrompt)
		}

		a.dataModel.Config = cfg

		// Get the new data dir from system config (to check if it changed)
		// Note: For data dir changes, config will be loaded AGAIN in ApplyDataDirSwitch()
		// (may prompt for passphrase if new data dir is SSH-encrypted)
		systemCfg, err := config.LoadSystemConfig()
		if err != nil {
			a.settingsSaveError = fmt.Sprintf("Failed to load system config: %v", err)
			return a, nil
		}
		newDataDir := config.ExpandPath(systemCfg.DataDirectory)

		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings Save] === ENTRY === oldDataDir=%s, newDataDir=%s, oldPlugins=%v",
				oldDataDir, newDataDir, oldPluginsEnabled)
		}

		// STEP 1: Handle plugin shutdown based on scenario
		// For plugin state changes, we need to check current config since we haven't reloaded yet
		// (Data dir changes are handled by switchDataDirectory which loads config after switch)

		// SCENARIO B: Just disabling plugins (no data dir change) - show modal and return
		// Note: We use oldDataDir == newDataDir comparison since config not reloaded yet
		if !a.dataModel.Config.PluginsEnabled && oldPluginsEnabled && oldDataDir == newDataDir {
			if a.dataModel.MCPManager != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[Settings] Plugin system disabling (no data dir change) - showing shutdown modal")
				}

				// Close settings modal
				a.showSettings = false
				a.settingsHasChanges = false

				// Show shutdown modal
				a.pluginSystemState = PluginSystemState{
					Active:    true,
					Operation: "stopping",
					Phase:     "waiting",
					Spinner:   spinner.New(),
					StartTime: time.Now(),
				}
				a.pluginSystemState.Spinner.Spinner = spinner.Dot

				if config.DebugLog != nil {
					config.DebugLog.Printf("[Settings Save] === EARLY RETURN for plugin shutdown (no data dir change) ===")
				}
				return a, tea.Batch(
					a.pluginSystemState.Spinner.Tick,
					stopPluginSystemCmd(a.dataModel.MCPManager),
				)
			}
		}

		// STEP 2: Handle data directory change (if any)
		// Delegated to switchDataDirectory() helper which orchestrates all 6 steps
		// Config will be loaded AFTER switch (may prompt for passphrase)
		if newDataDir != oldDataDir {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] Data directory changed: %s → %s", oldDataDir, newDataDir)
			}

			// Close settings modal
			a.showSettings = false
			a.settingsHasChanges = false

			// Call helper that orchestrates all 6 steps
			return a, a.switchDataDirectory(newDataDir)
		}

		// Data dir didn't change - update session storage
		newStorage, err := storage.NewSessionStorage(newDataDir)
		if err == nil {
			a.dataModel.SessionStorage = newStorage
			a.dataModel.SearchIndex = storage.NewSearchIndex(newStorage)
		}

		// STEP 3: Handle plugin enabling (only if data dir didn't change)
		// If data dir changed, plugins will start after session loads (see sessionLoadedMsg handler)
		// Use current config since we haven't reloaded yet (only reload happens during data dir switch)
		newPluginsEnabled := a.dataModel.Config.PluginsEnabled
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings Save] Checking plugin enable: oldPlugins=%v, newPlugins=%v, dataDirSame=%v, condition=%v",
				oldPluginsEnabled, newPluginsEnabled, newDataDir == oldDataDir, !oldPluginsEnabled && newPluginsEnabled && newDataDir == oldDataDir)
		}
		if !oldPluginsEnabled && newPluginsEnabled && newDataDir == oldDataDir {
			// ENABLING: Force manager recreation to pick up new config
			// This fixes stale config after reload (disable → quit → launch → enable)
			if a.dataModel.MCPManager != nil {
				a.dataModel.MCPManager = nil
			}

			// Ensure manager exists, then start plugins
			if err := a.ensureMCPManager(); err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[Settings] Plugin system enable FAILED: cannot recreate manager: %v", err)
				}
				// Show error modal
				a.showSettings = false
				a.settingsHasChanges = false
				a.showAcknowledgeModal = true
				a.acknowledgeModalTitle = "⚠️  Plugin System Error"
				a.acknowledgeModalMsg = fmt.Sprintf("Failed to enable plugin system:\n\n%v\n\nPlugin infrastructure may be corrupted. Try restarting OTUI.", err)
				a.acknowledgeModalType = ModalTypeError
				return a, nil
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] Plugin system enabling - showing startup modal")
			}

			// Close settings modal
			a.showSettings = false
			a.settingsHasChanges = false

			// Show startup modal
			a.pluginSystemState = PluginSystemState{
				Active:    true,
				Operation: "starting",
				Phase:     "waiting",
				Spinner:   spinner.New(),
				StartTime: time.Now(),
			}
			a.pluginSystemState.Spinner.Spinner = spinner.Dot
			a.pluginSystemState.Spinner.Style = lipgloss.NewStyle().Foreground(successColor)

			return a, tea.Batch(
				a.pluginSystemState.Spinner.Tick,
				startPluginSystemCmd(a.dataModel.MCPManager, a.dataModel.CurrentSession),
			)
		}

		// STEP 4: Handle other config changes (Ollama host, etc.)
		// Only update provider if HOST changed
		// Use current config since we're not reloading (only happens during data dir switch)
		if a.dataModel.Config.OllamaURL() != oldHost {
			providerCfg := provider.Config{
				Type:    provider.ProviderTypeOllama,
				BaseURL: a.dataModel.Config.OllamaURL(),
				Model:   currentModel,
			}
			newProvider, err := provider.NewProvider(providerCfg)
			if err == nil {
				a.dataModel.Provider = newProvider
			}
		}

		// STEP 5: Close settings and fetch session list
		a.showSettings = false
		a.settingsHasChanges = false
		a.settingsSaveError = ""
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Settings Save] === NORMAL RETURN === fetching session list")
		}
		return a, a.dataModel.FetchSessionList()

	case tea.KeyMsg:
		// Handle confirmation modal
		if a.settingsConfirmExit {
			switch msg.String() {
			case "y", "Y":
				a.settingsConfirmExit = false
				a.settingsHasChanges = false
				a.showSettings = false
				return a, nil
			case "n", "N", "esc":
				a.settingsConfirmExit = false
				return a, nil
			}
			return a, nil
		}

		// Handle data export mode
		if a.dataExportMode {
			// Handle Esc cancellation during export
			if msg.String() == "esc" && a.exportingDataDir && !a.dataExportCleaningUp {
				if a.dataExportCancelFunc != nil {
					a.dataExportCancelFunc()
				}
				return a, nil
			}
			return a.handleDataExportMode(msg)
		}

		// Handle edit mode
		if a.settingsEditMode {
			return a.handleSettingsEditMode(msg)
		}

		// Handle navigation mode
		return a.handleSettingsNavigationMode(msg)
	}

	return a, nil
}

func (a AppView) handleSettingsNavigationMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle new data directory confirmation modal (y/n)
	if a.settingsDataDirNotFound {
		switch msg.String() {
		case "y", "Y":
			// User confirmed - create new data directory via restart
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] User confirmed new data dir creation: %s", a.settingsNewDataDirPath)
			}

			a.settingsDataDirNotFound = false
			a.showSettings = false

			// Write system config with new data dir
			systemCfg := &config.SystemConfig{
				DataDirectory: a.settingsNewDataDirPath,
			}
			if err := config.SaveSystemConfig(systemCfg); err != nil {
				a.showAcknowledgeModal = true
				a.acknowledgeModalTitle = "⚠️  Error"
				a.acknowledgeModalMsg = fmt.Sprintf("Failed to save config:\n\n%v", err)
				a.acknowledgeModalType = ModalTypeError
				return a, nil
			}

			// Set restart flag
			a.RestartAfterQuit = true

			// Trigger quit flow (reuse existing logic)
			a.dataModel.Quitting = true

			// If plugins running, show shutdown modal
			if a.dataModel.MCPManager != nil && a.dataModel.Config.PluginsEnabled {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[Settings] New data dir creation: shutting down plugins before restart")
				}
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
					stopPluginSystemCmd(a.dataModel.MCPManager),
				)
			}

			// No plugins - quit immediately
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] New data dir creation: no plugins, quitting immediately")
			}
			if a.dataModel.CurrentSession != nil && a.dataModel.SessionStorage != nil {
				_ = a.dataModel.SessionStorage.UnlockSession(a.dataModel.CurrentSession.ID)
			}
			return a, tea.Quit

		case "n", "N", "esc":
			// User cancelled - return to Settings
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] User cancelled new data dir creation")
			}
			a.settingsDataDirNotFound = false
			return a, nil
		}
		return a, nil
	}

	// If showing save error, Enter/Esc clears it
	if a.settingsSaveError != "" {
		if msg.String() == "enter" || msg.String() == "esc" {
			a.settingsSaveError = ""
			return a, nil
		}
		return a, nil
	}

	switch msg.String() {
	case "q":
		a.showSettings = false
		return a, nil

	case "esc":
		if a.settingsHasChanges {
			a.settingsConfirmExit = true
			return a, nil
		}
		a.showSettings = false
		return a, nil

	case "j", "down":
		if a.selectedSettingIdx < len(a.settingsFields)-1 {
			a.selectedSettingIdx++
		}
		return a, nil

	case "k", "up":
		if a.selectedSettingIdx > 0 {
			a.selectedSettingIdx--
		}
		return a, nil

	case "enter":
		// Check if this is the provider link field - open provider settings sub-screen
		if a.settingsFields[a.selectedSettingIdx].Type == SettingTypeProviderLink {
			a.providerSettingsState.visible = true
			a.providerSettingsState.selectedProviderID = "ollama" // Default tab
			a.providerSettingsState.selectedFieldIdx = 0
			a.providerSettingsState.editMode = false
			a.providerSettingsState.hasChanges = false

			// Initialize edit input
			a.providerSettingsState.editInput = textinput.New()
			a.providerSettingsState.editInput.Width = 50
			a.providerSettingsState.editInput.CharLimit = 500

			// Load ALL providers' fields into cache (not just current tab)
			a.providerSettingsState.currentFieldsMap = make(map[string][]ProviderField)

			if config.DebugLog != nil {
				config.DebugLog.Printf("[CacheInit] ========== INITIALIZING PROVIDER CACHE ==========")
			}

			for _, providerID := range providerTabs {
				a.providerSettingsState.currentFieldsMap[providerID] = a.providerSettingsState.getProviderFields(
					providerID,
					a.dataModel.Config,
				)

				if config.DebugLog != nil {
					fields := a.providerSettingsState.currentFieldsMap[providerID]
					config.DebugLog.Printf("[CacheInit] Provider: %s (%d fields)", providerID, len(fields))
					for i, f := range fields {
						config.DebugLog.Printf("[CacheInit]   Field[%d]: %s = '%s'", i, f.Label, f.Value)
					}
				}
			}

			return a, nil
		}

		// Check if this is the model field - open model selector instead
		if a.settingsFields[a.selectedSettingIdx].Type == SettingTypeModel {
			// Get Ollama host from config (no longer in settings fields)
			ollamaHost := a.dataModel.Config.OllamaHost
			return a, a.fetchModelListFromHost(ollamaHost)
		}

		// Check if this is the plugins enabled field - toggle it
		if a.settingsFields[a.selectedSettingIdx].Type == SettingTypePluginsEnabled {
			currentValue := a.settingsFields[a.selectedSettingIdx].Value
			switch currentValue {
			case "true":
				a.settingsFields[a.selectedSettingIdx].Value = "false"
			case "false":
				a.settingsFields[a.selectedSettingIdx].Value = "true"
			}
			a.settingsHasChanges = true
			return a, nil
		}

		// Enter edit mode for other fields
		a.settingsEditMode = true
		a.settingsEditInput.SetValue(a.settingsFields[a.selectedSettingIdx].Value)
		a.settingsEditInput.Focus()
		return a, textinput.Blink

	case "r":
		// Reset to default
		a.settingsFields[a.selectedSettingIdx].Value = a.settingsFields[a.selectedSettingIdx].DefaultValue
		a.settingsFields[a.selectedSettingIdx].Validation = FieldValidationNone
		a.settingsFields[a.selectedSettingIdx].ErrorMsg = ""
		a.settingsHasChanges = true
		return a, nil

	case "x":
		// Open data export modal
		a.dataExportMode = true

		// Lazy init textinput
		if a.dataExportInput.Width == 0 {
			a.dataExportInput = textinput.New()
			a.dataExportInput.Width = 70
			a.dataExportInput.CharLimit = 500
		}

		// Generate default filename
		now := time.Now()
		defaultFilename := fmt.Sprintf("~/Downloads/otui-data-%s.tar.gz",
			now.Format("010206-1504")) // MMDDYY-HHMM

		a.dataExportInput.SetValue(defaultFilename)
		a.dataExportInput.Focus()
		return a, textinput.Blink

	case "alt+enter":
		// Save settings
		return a, a.saveSettingsCmd()
	}

	return a, nil
}

func (a AppView) handleDataExportMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	kb := a.dataModel.Config.Keybindings

	// Handle success acknowledgment
	if a.dataExportSuccess != "" {
		if msg.String() == "enter" || msg.String() == "esc" {
			a.dataExportSuccess = ""
			a.dataExportMode = false
			return a, nil
		}
		return a, nil
	}

	switch msg.String() {
	case "esc":
		a.dataExportMode = false
		a.dataExportInput.Blur()
		return a, nil

	case "enter":
		exportPath := strings.TrimSpace(a.dataExportInput.Value())
		if exportPath == "" {
			return a, nil
		}

		// Get data directory from config (already loaded at launch)
		dataDir := a.dataModel.Config.DataDir()

		// Expand export path
		a.dataExportTargetPath = config.ExpandPath(exportPath)

		// Create cancellation context
		ctx, cancel := context.WithCancel(context.Background())
		a.dataExportCancelCtx = ctx
		a.dataExportCancelFunc = cancel

		// Initialize spinner
		a.dataExportSpinner = spinner.New()
		a.dataExportSpinner.Spinner = spinner.Dot
		a.dataExportSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

		// Set exporting state
		a.exportingDataDir = true
		a.dataExportInput.Blur()

		// Start export
		return a, tea.Batch(
			a.exportDataDirCmd(ctx, dataDir, a.dataExportTargetPath),
			a.dataExportSpinner.Tick,
		)

	case kb.GetActionKey("clear_input"):
		a.dataExportInput.SetValue("")
		return a, nil
	}

	a.dataExportInput, cmd = a.dataExportInput.Update(msg)
	return a, cmd
}

func (a AppView) handleSettingsEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	kb := a.dataModel.Config.Keybindings

	switch msg.String() {
	case "esc":
		// Cancel edit
		a.settingsEditMode = false
		a.settingsEditInput.Blur()
		return a, nil

	case "enter":
		// Save edited value
		newValue := a.settingsEditInput.Value()
		if newValue != a.settingsFields[a.selectedSettingIdx].Value {
			a.settingsFields[a.selectedSettingIdx].Value = newValue
			a.settingsHasChanges = true

			// Handle specific field logic
			switch a.settingsFields[a.selectedSettingIdx].Type {
			case SettingTypeDataDir:
				a.settingsFields[a.selectedSettingIdx].Value = newValue
				a.settingsHasChanges = true
				a.settingsEditMode = false
				a.settingsEditInput.Blur()
				return a, a.handleDataDirectoryChangeCmd(newValue)
			}
		}

		a.settingsEditMode = false
		a.settingsEditInput.Blur()
		return a, nil

	case kb.GetActionKey("clear_input"):
		// Clear input
		a.settingsEditInput.SetValue("")
		return a, nil
	}

	a.settingsEditInput, cmd = a.settingsEditInput.Update(msg)
	return a, cmd
}

func (a AppView) handleDataDirectoryChangeCmd(newPath string) tea.Cmd {
	return func() tea.Msg {
		normalized, err := config.NormalizeDataDirectory(newPath)
		if err != nil {
			return dataDirectoryLoadedMsg{err: err}
		}

		// Check if config.toml exists in that directory
		configPath := filepath.Join(normalized, "config.toml")
		userCfg, err := config.LoadUserConfigFromPath(configPath)
		if err != nil {
			return dataDirectoryLoadedMsg{
				normalizedPath: normalized,
				err:            fmt.Errorf("failed to load config: %w", err),
			}
		}

		if userCfg != nil {
			// Config exists - load ALL values
			return dataDirectoryLoadedMsg{
				normalizedPath: normalized,
				configLoaded:   true,
				ollamaHost:     userCfg.Ollama.Host,
				defaultModel:   userCfg.Ollama.DefaultModel,
				systemPrompt:   userCfg.DefaultSystemPrompt,
				pluginsEnabled: userCfg.PluginsEnabled,
			}
		}

		// No existing config
		return dataDirectoryLoadedMsg{
			normalizedPath: normalized,
			configLoaded:   false,
		}
	}
}

func (a AppView) fetchModelListFromHost(host string) tea.Cmd {
	return func() tea.Msg {
		// Create a temporary provider with the specified host
		providerCfg := provider.Config{
			Type:    provider.ProviderTypeOllama,
			BaseURL: host,
			Model:   "",
		}
		tempProvider, err := provider.NewProvider(providerCfg)
		if err != nil {
			return modelsListMsg{
				Models:       nil,
				Err:          fmt.Errorf("failed to create provider: %w", err),
				ShowSelector: true,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		models, err := tempProvider.ListModels(ctx)
		if err != nil {
			return modelsListMsg{
				Models:       nil,
				Err:          fmt.Errorf("failed to fetch Models: %w", err),
				ShowSelector: true,
			}
		}

		return modelsListMsg{
			Models:       models,
			Err:          nil,
			ShowSelector: true,
		}
	}
}

func validateOllamaHostCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return ollamaValidationMsg{success: false, err: fmt.Errorf("URL cannot be empty")}
		}

		// Create HTTP client with timeout
		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		// Ping Ollama's version endpoint
		resp, err := client.Get(url + "/api/version")
		if err != nil {
			return ollamaValidationMsg{success: false, err: fmt.Errorf("failed to connect: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return ollamaValidationMsg{success: false, err: fmt.Errorf("server returned status %d", resp.StatusCode)}
		}

		return ollamaValidationMsg{success: true}
	}
}

func (a AppView) saveSettingsCmd() tea.Cmd {
	return func() tea.Msg {
		// Check if plugin installation is in progress
		if a.pluginManagerState.installModal.visible {
			return settingsSaveMsg{
				success: false,
				err:     fmt.Errorf("Plugin installation in progress...\n\nPlease wait until installation completes before switching data directories."),
			}
		}

		// Note: Ollama Host validation removed - now handled in provider sub-screen

		// Normalize and check if data directory exists
		dataDir, err := config.NormalizeDataDirectory(a.settingsFields[0].Value)
		if err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to normalize data directory: %w", err)}
		}

		// Check if data directory exists
		if !fileExists(dataDir) {
			// Directory doesn't exist - prompt user to create new data directory
			if config.DebugLog != nil {
				config.DebugLog.Printf("[Settings] Data directory doesn't exist: %s", dataDir)
			}
			return dataDirectoryNotFoundMsg{path: dataDir}
		}

		// Save system config
		systemCfg := &config.SystemConfig{
			DataDirectory: a.settingsFields[0].Value,
		}
		if err := config.SaveSystemConfig(systemCfg); err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to save system config: %w", err)}
		}

		// Load existing config to preserve multi-provider settings
		existingCfg, err := config.LoadUserConfig(dataDir)
		if err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to load existing config: %w", err)}
		}

		// Update ONLY the fields exposed in current Settings UI
		// Note: Field 1 is Provider(s) link (not saved), Ollama Host now in provider sub-screen
		existingCfg.DefaultModel = a.settingsFields[2].Value
		existingCfg.DefaultSystemPrompt = a.settingsFields[3].Value
		existingCfg.PluginsEnabled = stringToBool(a.settingsFields[4].Value)

		// Save updated config (preserves DefaultProvider, Providers[], Security, etc.)
		if err := config.SaveUserConfig(existingCfg, dataDir); err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to save user config: %w", err)}
		}

		return settingsSaveMsg{success: true}
	}
}

func renderDataExportModal(a AppView, exportInput textinput.Model, exporting bool, cleaningUp bool, exportSpinner spinner.Model, successPath string, width, height int) string {
	// Check for success state first
	if successPath != "" {
		return renderExportSuccess(successPath, "✓ Data Export Successful", width, height)
	}

	modalWidth := width - 10
	if modalWidth > 80 {
		modalWidth = 80
	}

	borderStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Width(modalWidth)

	topBorder := borderStyle.Render("┌" + strings.Repeat("─", modalWidth-2) + "┐")
	middleBorder := borderStyle.Render("├" + strings.Repeat("─", modalWidth-2) + "┤")
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", modalWidth-2) + "┘")
	emptyLine := "│" + strings.Repeat(" ", modalWidth-2) + "│"

	var content strings.Builder
	content.WriteString(topBorder + "\n")

	if cleaningUp {
		// State 3: Cleaning up
		content.WriteString(emptyLine + "\n")

		cleanupLine := fmt.Sprintf("%s Cleaning up...", exportSpinner.View())
		styledCleanup := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Align(lipgloss.Center).
			Width(modalWidth - 2).
			Render(cleanupLine)

		content.WriteString("│" + styledCleanup + "│\n")
		content.WriteString(emptyLine + "\n")

	} else if exporting {
		// State 2: Exporting with spinner
		content.WriteString(emptyLine + "\n")

		exportLine := fmt.Sprintf("%s Exporting data directory...", exportSpinner.View())
		styledExport := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Align(lipgloss.Center).
			Width(modalWidth - 2).
			Render(exportLine)

		content.WriteString("│" + styledExport + "│\n")
		content.WriteString(emptyLine + "\n")
		content.WriteString(middleBorder + "\n")

		cancelHint := lipgloss.NewStyle().
			Foreground(dimColor).
			Align(lipgloss.Center).
			Width(modalWidth - 2).
			Render("Press Esc to cancel")

		content.WriteString("│" + cancelHint + "│\n")

	} else {
		// State 1: Input mode
		title := lipgloss.NewStyle().
			Bold(true).
			Align(lipgloss.Center).
			Width(modalWidth).
			Render("Export Data Directory")

		content.WriteString("│" + title + "│\n")
		content.WriteString(middleBorder + "\n")
		content.WriteString(emptyLine + "\n")

		prompt := lipgloss.NewStyle().
			Width(modalWidth - 6).
			Render("Export to:")

		inputLine := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Width(modalWidth - 6).
			Render(exportInput.View())

		content.WriteString("│  " + prompt + "  │\n")
		content.WriteString("│  " + inputLine + "  │\n")
		content.WriteString(emptyLine + "\n")
		content.WriteString(middleBorder + "\n")

		footer := lipgloss.NewStyle().
			Foreground(dimColor).
			Align(lipgloss.Center).
			Width(modalWidth - 2).
			Render(fmt.Sprintf("Esc Cancel  Enter Export  %s Clear", a.formatKeyDisplay("primary", "U")))

		content.WriteString("│" + footer + "│\n")
	}

	content.WriteString(bottomBorder)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
	)
}

func renderSettings(a AppView, fields []SettingField, selectedIdx int, editMode bool, editInput textinput.Model, hasChanges bool, confirmExit bool, loadedInfo string, saveError string, dataExportMode bool, dataExportInput textinput.Model, exportingDataDir bool, dataExportCleaningUp bool, dataExportSpinner spinner.Model, dataExportSuccess string, dataDirNotFound bool, newDataDirPath string, width, height int) string {
	// Check for new data directory confirmation modal first
	if dataDirNotFound {
		return renderDataDirNotFoundModal(newDataDirPath, width, height)
	}

	// Check for data export modal
	if dataExportMode {
		return renderDataExportModal(a, dataExportInput, exportingDataDir, dataExportCleaningUp, dataExportSpinner, dataExportSuccess, width, height)
	}

	if confirmExit {
		return RenderUnsavedChangesModal(width, height)
	}

	if saveError != "" {
		return renderSettingsSaveError(saveError, width, height)
	}

	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := width - 10
	if modalWidth > 80 {
		modalWidth = 80
	}
	if modalWidth < 40 {
		modalWidth = 40
	}

	// Title section (centered, no borders - following modal_helpers.go pattern)
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(fmt.Sprintf("Settings (%s)", a.formatKeyDisplay("secondary", "S")))

	// Separator (simple horizontal line - following modal_helpers.go pattern)
	separator := lipgloss.NewStyle().
		Foreground(dimColor).
		Render(strings.Repeat("─", modalWidth))

	// Settings list
	var settingsLines []string
	for i, field := range fields {
		var line string

		if editMode && i == selectedIdx {
			// Show edit input
			label := field.Label
			labelPadding := strings.Repeat(" ", 20-len(label))
			inputBox := lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				Width(modalWidth - 24).
				Render(editInput.View())
			line = fmt.Sprintf("  %s%s%s", label, labelPadding, inputBox)
		} else {
			// Show value
			indicator := "  "
			if i == selectedIdx {
				indicator = "▶ "
			}

			// Format value with validation indicator
			value := field.Value
			validationIndicator := ""
			switch field.Validation {
			case FieldValidationPending:
				validationIndicator = "  ⏳"
			case FieldValidationSuccess:
				validationIndicator = "  ✓"
			case FieldValidationError:
				validationIndicator = "  ✗"
			}

			// Calculate spacing
			label := fmt.Sprintf("%s%s", indicator, field.Label)
			maxLabelWidth := 20
			if len(label) < maxLabelWidth {
				label = label + strings.Repeat(" ", maxLabelWidth-len(label))
			}

			valueWithIndicator := value + validationIndicator
			maxValueWidth := modalWidth - maxLabelWidth - 4
			if len(valueWithIndicator) > maxValueWidth {
				valueWithIndicator = valueWithIndicator[:maxValueWidth-3] + "..."
			}

			line = label + valueWithIndicator

			// Style the line
			lineStyle := lipgloss.NewStyle()
			if i == selectedIdx {
				lineStyle = lineStyle.Foreground(successColor).Bold(true)
			}

			line = lineStyle.Render(line)
		}

		paddedLine := lipgloss.NewStyle().
			Width(modalWidth).
			Render(line)
		settingsLines = append(settingsLines, paddedLine)
	}

	// Footer
	var footerText string
	if editMode {
		footerText = FormatFooter("Enter", "Save", a.formatKeyDisplay("primary", "U"), "Clear", "Esc", "Cancel")
	} else if hasChanges {
		footerText = FormatFooter(a.formatKeyDisplay("primary", "Enter"), "Save", "x", "Export Data", "r", "Reset", "Esc", "Cancel")
	} else {
		footerText = FormatFooter("j/k", "Navigate", "Enter", "Edit", "x", "Export Data", "r", "Reset", "Esc", "Close")
	}
	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(footerText)

	// Info line
	var infoLine string
	if loadedInfo != "" {
		infoLine = lipgloss.NewStyle().
			Width(modalWidth).
			Foreground(accentColor).
			Render("  "+loadedInfo) + "\n"
	}

	// Combine all parts (Title/Separator/Content/Separator/Footer pattern)
	var content strings.Builder
	content.WriteString(title + "\n")
	content.WriteString(separator + "\n")
	content.WriteString(strings.Repeat(" ", modalWidth) + "\n") // Top padding
	for _, line := range settingsLines {
		content.WriteString(line + "\n")
	}
	content.WriteString(strings.Repeat(" ", modalWidth) + "\n") // Bottom padding
	if infoLine != "" {
		content.WriteString(infoLine)
	}
	content.WriteString(separator + "\n")
	content.WriteString(footer)

	// Center the modal
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
	)
}

func renderSettingsSaveError(errorMsg string, width, height int) string {
	return RenderAcknowledgeModal(
		"Error Saving Settings",
		errorMsg,
		ModalTypeError,
		width,
		height,
	)
}

func renderDataDirNotFoundModal(path string, width, height int) string {
	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Build message lines
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center)

	messageLines = append(messageLines, messageStyle.Render("The directory does not exist:"))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))
	messageLines = append(messageLines, messageStyle.Render(path))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))
	messageLines = append(messageLines, messageStyle.Render("Would you like to create a new data directory here?"))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))
	messageLines = append(messageLines, messageStyle.Render("(OTUI will restart and launch the setup wizard)"))

	// Use RenderThreeSectionModal for consistent pattern
	footer := FormatFooter("y", "Yes, create new data directory", "n", "No, return to Settings")
	return RenderThreeSectionModal(
		"⚠️  Data Directory Not Found",
		messageLines,
		footer,
		ModalTypeWarning,
		modalWidth,
		width,
		height,
	)
}

func (a AppView) exportDataDirCmd(ctx context.Context, dataDir, exportPath string) tea.Cmd {
	return func() tea.Msg {
		// Cancellation point 1: Before starting
		select {
		case <-ctx.Done():
			return dataExportedMsg{Cancelled: true}
		default:
		}

		// Check if data directory exists
		if _, err := os.Stat(dataDir); os.IsNotExist(err) {
			return dataExportedMsg{Err: fmt.Errorf("data directory does not exist: %s", dataDir)}
		}

		// Create parent directory for export (0700 - user-only access)
		exportDir := filepath.Dir(exportPath)
		if err := os.MkdirAll(exportDir, 0700); err != nil {
			return dataExportedMsg{Err: fmt.Errorf("failed to create export directory: %w", err)}
		}

		// Cancellation point 2: Before creating tar file
		select {
		case <-ctx.Done():
			return dataExportedMsg{Cancelled: true}
		default:
		}

		// Create tar.gz file (0600 - data exports contain all sensitive user data)
		outFile, err := os.OpenFile(exportPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return dataExportedMsg{Err: fmt.Errorf("failed to create tar file: %w", err)}
		}
		defer outFile.Close()

		// Create gzip writer
		gzWriter := gzip.NewWriter(outFile)
		defer gzWriter.Close()

		// Create tar writer
		tarWriter := tar.NewWriter(gzWriter)
		defer tarWriter.Close()

		// Walk the data directory and add files to tar
		err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check for cancellation during walk
			select {
			case <-ctx.Done():
				return fmt.Errorf("cancelled")
			default:
			}

			// Create tar header
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}

			// Update header name to be relative to data dir with "otui/" prefix
			relPath, err := filepath.Rel(dataDir, path)
			if err != nil {
				return err
			}
			header.Name = filepath.Join("otui", relPath)

			// Write header
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}

			// If it's a file, write its content
			if !info.IsDir() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				if _, err := io.Copy(tarWriter, file); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			if err.Error() == "cancelled" {
				return dataExportedMsg{Cancelled: true}
			}
			return dataExportedMsg{Err: fmt.Errorf("failed to create archive: %w", err)}
		}

		return dataExportedMsg{Path: exportPath}
	}
}

// startPluginSystemCmd starts all enabled plugins and reports progress
func startPluginSystemCmd(manager *mcp.MCPManager, session *storage.Session) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if session != nil {
			_ = manager.SetSession(session)
		}

		if err := manager.StartAllEnabledPlugins(ctx); err != nil {
			return pluginSystemOperationMsg{
				operation: "starting",
				success:   false,
				err:       err,
			}
		}

		return pluginSystemOperationMsg{
			operation: "starting",
			success:   true,
		}
	}
}

// stopPluginSystemCmd shuts down all plugins and reports progress
func stopPluginSystemCmd(manager *mcp.MCPManager) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Use ShutdownWithTracking to detect unresponsive plugins
		unresponsiveNames, err := manager.ShutdownWithTracking(ctx)

		if len(unresponsiveNames) > 0 {
			// Some plugins didn't respond
			return pluginSystemOperationMsg{
				operation:           "stopping",
				success:             false,
				unresponsivePlugins: unresponsiveNames,
				err:                 err,
			}
		}

		return pluginSystemOperationMsg{
			operation: "stopping",
			success:   true,
		}
	}
}

// pluginSystemOperationMsg reports progress of plugin system start/stop operations
type pluginSystemOperationMsg struct {
	operation           string // "starting" or "stopping"
	success             bool
	err                 error
	unresponsivePlugins []string
}
