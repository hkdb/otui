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
	"otui/ollama"
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
	err            error
}

type settingsSaveMsg struct {
	success bool
	err     error
}

func (a AppView) handleSettingsInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ollamaValidationMsg:
		// Update validation state for Ollama Host field
		for i := range a.settingsFields {
			if a.settingsFields[i].Type == SettingTypeOllamaHost {
				if msg.success {
					a.settingsFields[i].Validation = FieldValidationSuccess
					a.settingsFields[i].ErrorMsg = ""
				} else {
					a.settingsFields[i].Validation = FieldValidationError
					if msg.err != nil {
						a.settingsFields[i].ErrorMsg = msg.err.Error()
					} else {
						a.settingsFields[i].ErrorMsg = "Failed to connect"
					}
				}
			}
		}
		return a, nil

	case dataDirectoryLoadedMsg:
		if msg.err != nil {
			// Show error
			a.settingsFields[0].Validation = FieldValidationError
			a.settingsFields[0].ErrorMsg = msg.err.Error()
			return a, nil
		}

		a.settingsFields[0].Value = msg.normalizedPath
		a.settingsFields[0].Validation = FieldValidationNone

		if msg.configLoaded {
			// Update other fields
			a.settingsFields[1].Value = msg.ollamaHost
			a.settingsFields[2].Value = msg.defaultModel
			a.settingsLoadedInfo = "ℹ Loaded config from data directory"
			a.settingsHasChanges = true
			// Trigger validation of new Ollama host
			return a, validateOllamaHostCmd(msg.ollamaHost)
		} else {
			a.settingsLoadedInfo = ""
		}

		return a, nil

	case settingsSaveMsg:
		if !msg.success {
			// Show error inline in settings modal
			a.settingsSaveError = msg.err.Error()
			return a, nil
		}

		// Preserve current state for rollback
		oldDataDir := a.dataModel.Config.DataDir()
		oldHost := a.dataModel.Config.OllamaURL()
		oldPluginsEnabled := a.dataModel.Config.PluginsEnabled
		currentModel := a.dataModel.OllamaClient.GetModel()

		// Reload config after successful save
		cfg, err := config.Load()
		if err != nil {
			a.settingsSaveError = fmt.Sprintf("Failed to reload config: %v", err)
			return a, nil
		}

		a.dataModel.Config = cfg

		// Handle plugins_enabled toggle
		if cfg.PluginsEnabled != oldPluginsEnabled {
			if cfg.PluginsEnabled {
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
			} else {
				// DISABLING: Show modal, shutdown plugins
				if a.dataModel.MCPManager != nil {
					if config.DebugLog != nil {
						config.DebugLog.Printf("[Settings] Plugin system disabling - showing shutdown modal")
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

					return a, tea.Batch(
						a.pluginSystemState.Spinner.Tick,
						stopPluginSystemCmd(a.dataModel.MCPManager),
					)
				}
			}
		}

		// Only update client if HOST changed
		if cfg.OllamaURL() != oldHost {
			newClient, err := ollama.NewClient(cfg.OllamaURL(), currentModel)
			if err == nil {
				a.dataModel.OllamaClient = newClient
			}
		}

		// If data directory changed, validate and refresh ALL components
		if cfg.DataDir() != oldDataDir {
			// Try to create new session storage
			newStorage, err := storage.NewSessionStorage(cfg.DataDir())
			if err != nil {
				// ROLLBACK: Restore old data dir
				oldCfg := &config.SystemConfig{DataDirectory: oldDataDir}
				_ = config.SaveSystemConfig(oldCfg)
				a.dataModel.Config, _ = config.Load()

				a.settingsSaveError = fmt.Sprintf("Failed to initialize session storage:\n%v\n\nReverted to previous data directory.", err)
				return a, nil
			}

			// Try to create new plugin storage
			newPluginStorage, err := storage.NewPluginStorage(cfg.DataDir())
			if err != nil {
				// ROLLBACK: Restore old data dir
				oldCfg := &config.SystemConfig{DataDirectory: oldDataDir}
				_ = config.SaveSystemConfig(oldCfg)
				a.dataModel.Config, _ = config.Load()

				a.settingsSaveError = fmt.Sprintf("Failed to initialize plugin storage:\n%v\n\nReverted to previous data directory.", err)
				return a, nil
			}

			// Try to load plugins config
			newPluginsConfig, err := config.LoadPluginsConfig(cfg.DataDir())
			if err != nil {
				// ROLLBACK: Restore old data dir
				oldCfg := &config.SystemConfig{DataDirectory: oldDataDir}
				_ = config.SaveSystemConfig(oldCfg)
				a.dataModel.Config, _ = config.Load()

				a.settingsSaveError = fmt.Sprintf("Failed to load plugins config:\n%v\n\nReverted to previous data directory.", err)
				return a, nil
			}

			// Try to create new registry
			newRegistry, err := mcp.NewRegistry(cfg.DataDir())
			if err != nil {
				// ROLLBACK: Restore old data dir
				oldCfg := &config.SystemConfig{DataDirectory: oldDataDir}
				_ = config.SaveSystemConfig(oldCfg)
				a.dataModel.Config, _ = config.Load()

				a.settingsSaveError = fmt.Sprintf("Failed to initialize plugin registry:\n%v\n\nReverted to previous data directory.", err)
				return a, nil
			}

			// ALL validations passed - commit changes
			a.dataModel.SessionStorage = newStorage
			a.dataModel.SearchIndex = storage.NewSearchIndex(newStorage)

			// Update plugin components
			newInstaller := mcp.NewInstaller(newPluginStorage, newPluginsConfig, cfg.DataDir())
			a.pluginManagerState.pluginState.Registry = newRegistry
			a.pluginManagerState.pluginState.Installer = newInstaller

			// Reset plugin manager UI state
			a.pluginManagerState.selection.selectedPluginIdx = 0
			a.pluginManagerState.selection.scrollOffset = 0
			a.pluginManagerState.selection.filterMode = false
			a.pluginManagerState.selection.filterInput.SetValue("")
			a.pluginManagerState.selection.filteredPlugins = nil
			a.pluginManagerState.selection.viewMode = "curated"

			// Re-initialize debug log for new data directory
			if config.Debug {
				config.InitDebugLog(cfg.DataDir())
			}

			// Clear current session
			a.dataModel.Messages = []Message{}
			a.setCurrentSession(nil) // Clear and sync with MCP manager
			a.dataModel.SessionDirty = false
			a.dataModel.NeedsInitialRender = false
		} else {
			// Data dir didn't change, just update session storage
			newStorage, err := storage.NewSessionStorage(cfg.DataDir())
			if err == nil {
				a.dataModel.SessionStorage = newStorage
				a.dataModel.SearchIndex = storage.NewSearchIndex(newStorage)
			}
		}

		a.showSettings = false
		a.settingsHasChanges = false
		a.settingsSaveError = ""
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
		// Check if this is the model field - open model selector instead
		if a.settingsFields[a.selectedSettingIdx].Type == SettingTypeModel {
			// Open model selector with current Ollama host
			ollamaHost := a.settingsFields[1].Value // Ollama Host is at index 1
			return a, a.fetchModelListFromHost(ollamaHost)
		}

		// Check if this is the plugins enabled field - toggle it
		if a.settingsFields[a.selectedSettingIdx].Type == SettingTypePluginsEnabled {
			currentValue := a.settingsFields[a.selectedSettingIdx].Value
			if currentValue == "true" {
				a.settingsFields[a.selectedSettingIdx].Value = "false"
			} else {
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

	case "alt+u":
		a.dataExportInput.SetValue("")
		return a, nil
	}

	a.dataExportInput, cmd = a.dataExportInput.Update(msg)
	return a, cmd
}

func (a AppView) handleSettingsEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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
				// Normalize and check for existing config
				a.settingsEditMode = false
				a.settingsEditInput.Blur()
				return a, a.handleDataDirectoryChangeCmd(newValue)

			case SettingTypeOllamaHost:
				// Validate Ollama host
				a.settingsFields[a.selectedSettingIdx].Validation = FieldValidationPending
				a.settingsEditMode = false
				a.settingsEditInput.Blur()
				return a, validateOllamaHostCmd(newValue)
			}
		}

		a.settingsEditMode = false
		a.settingsEditInput.Blur()
		return a, nil

	case "alt+u":
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
			// Config exists - load values
			return dataDirectoryLoadedMsg{
				normalizedPath: normalized,
				configLoaded:   true,
				ollamaHost:     userCfg.Ollama.Host,
				defaultModel:   userCfg.Ollama.DefaultModel,
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
		// Create a temporary client with the specified host
		tempClient, err := ollama.NewClient(host, "")
		if err != nil {
			return modelsListMsg{
				Models: nil,
				Err:    fmt.Errorf("failed to create client: %w", err),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		models, err := tempClient.ListModels(ctx)
		if err != nil {
			return modelsListMsg{
				Models: nil,
				Err:    fmt.Errorf("failed to fetch Models: %w", err),
			}
		}

		return modelsListMsg{
			Models: models,
			Err:    nil,
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

		// Check if validation passed
		for _, field := range a.settingsFields {
			if field.Type == SettingTypeOllamaHost && field.Validation == FieldValidationError {
				return settingsSaveMsg{success: false, err: fmt.Errorf("Ollama Host validation failed: %s", field.ErrorMsg)}
			}
			if field.Type == SettingTypeOllamaHost && field.Validation == FieldValidationPending {
				return settingsSaveMsg{success: false, err: fmt.Errorf("Ollama Host validation in progress")}
			}
		}

		// Save system config
		systemCfg := &config.SystemConfig{
			DataDirectory: a.settingsFields[0].Value,
		}
		if err := config.SaveSystemConfig(systemCfg); err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to save system config: %w", err)}
		}

		// Save user config
		dataDir, err := config.NormalizeDataDirectory(a.settingsFields[0].Value)
		if err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to normalize data directory: %w", err)}
		}

		userCfg := &config.UserConfig{
			Ollama: config.OllamaConfig{
				Host:         a.settingsFields[1].Value,
				DefaultModel: a.settingsFields[2].Value,
			},
			DefaultSystemPrompt: a.settingsFields[3].Value,
			PluginsEnabled:      stringToBool(a.settingsFields[4].Value),
		}
		if err := config.SaveUserConfig(userCfg, dataDir); err != nil {
			return settingsSaveMsg{success: false, err: fmt.Errorf("Failed to save user config: %w", err)}
		}

		return settingsSaveMsg{success: true}
	}
}

func renderDataExportModal(exportInput textinput.Model, exporting bool, cleaningUp bool, exportSpinner spinner.Model, successPath string, width, height int) string {
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
			Render("Esc Cancel  Enter Export  Alt+U Clear")

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

func renderSettings(fields []SettingField, selectedIdx int, editMode bool, editInput textinput.Model, hasChanges bool, confirmExit bool, loadedInfo string, saveError string, dataExportMode bool, dataExportInput textinput.Model, exportingDataDir bool, dataExportCleaningUp bool, dataExportSpinner spinner.Model, dataExportSuccess string, width, height int) string {
	// Check for data export modal first
	if dataExportMode {
		return renderDataExportModal(dataExportInput, exportingDataDir, dataExportCleaningUp, dataExportSpinner, dataExportSuccess, width, height)
	}

	if confirmExit {
		return renderSettingsConfirmExit(width, height)
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

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Settings (Alt+Shift+S)")

	// Border style
	borderStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Width(modalWidth)

	topBorder := borderStyle.Render("┌" + strings.Repeat("─", modalWidth-2) + "┐")
	middleBorder := borderStyle.Render("├" + strings.Repeat("─", modalWidth-2) + "┤")
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", modalWidth-2) + "┘")

	// Empty line
	emptyLine := "│" + strings.Repeat(" ", modalWidth-2) + "│"

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
			Width(modalWidth - 2).
			Render(line)
		settingsLines = append(settingsLines, "│"+paddedLine+"│")
	}

	// Footer
	var footerText string
	if editMode {
		footerText = FormatFooter("Enter", "Save", "Alt+U", "Clear", "Esc", "Cancel")
	} else if hasChanges {
		footerText = FormatFooter("Alt+Enter", "Save", "x", "Export Data", "r", "Reset", "Esc", "Cancel")
	} else {
		footerText = FormatFooter("j/k", "Navigate", "Enter", "Edit", "x", "Export Data", "r", "Reset", "Esc", "Close")
	}
	footer := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(modalWidth - 2).
		Render(footerText)

	// Info line
	var infoLine string
	if loadedInfo != "" {
		infoLine = "│" + lipgloss.NewStyle().
			Width(modalWidth-2).
			Foreground(accentColor).
			Render("  "+loadedInfo) + "│\n"
	}

	// Combine all parts
	var content strings.Builder
	content.WriteString(topBorder + "\n")
	content.WriteString("│" + title + "│\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString(emptyLine + "\n")
	for _, line := range settingsLines {
		content.WriteString(line + "\n")
	}
	content.WriteString(emptyLine + "\n")
	if infoLine != "" {
		content.WriteString(infoLine)
	}
	content.WriteString(middleBorder + "\n")
	content.WriteString("│" + footer + "│\n")
	content.WriteString(bottomBorder)

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
	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9")).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Error Saving Settings")

	// Wrap error message
	wrappedMsg := wordWrap(errorMsg, modalWidth-4)
	message := lipgloss.NewStyle().
		Width(modalWidth - 4).
		Foreground(lipgloss.Color("9")).
		Render(wrappedMsg)

	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth - 2).
		Render("Press Enter to acknowledge")

	borderStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Width(modalWidth)

	topBorder := borderStyle.Render("┌" + strings.Repeat("─", modalWidth-2) + "┐")
	middleBorder := borderStyle.Render("├" + strings.Repeat("─", modalWidth-2) + "┤")
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", modalWidth-2) + "┘")
	emptyLine := "│" + strings.Repeat(" ", modalWidth-2) + "│"

	var content strings.Builder
	content.WriteString(topBorder + "\n")
	content.WriteString("│" + title + "│\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString(emptyLine + "\n")

	// Add message lines
	for _, line := range strings.Split(message, "\n") {
		paddedLine := lipgloss.NewStyle().
			Width(modalWidth - 2).
			Render("  " + line)
		content.WriteString("│" + paddedLine + "│\n")
	}

	content.WriteString(emptyLine + "\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString("│" + footer + "│\n")
	content.WriteString(bottomBorder)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
	)
}

func renderSettingsConfirmExit(width, height int) string {
	modalWidth := 60
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Unsaved Changes")

	message := lipgloss.NewStyle().
		Width(modalWidth - 2).
		Align(lipgloss.Center).
		Render("You have unsaved changes. Discard them?")

	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth - 2).
		Render("y - Yes, discard  •  n - No, return to settings")

	borderStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Width(modalWidth)

	topBorder := borderStyle.Render("┌" + strings.Repeat("─", modalWidth-2) + "┐")
	middleBorder := borderStyle.Render("├" + strings.Repeat("─", modalWidth-2) + "┤")
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", modalWidth-2) + "┘")
	emptyLine := "│" + strings.Repeat(" ", modalWidth-2) + "│"

	var content strings.Builder
	content.WriteString(topBorder + "\n")
	content.WriteString("│" + title + "│\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString(emptyLine + "\n")
	content.WriteString("│" + message + "│\n")
	content.WriteString(emptyLine + "\n")
	content.WriteString(middleBorder + "\n")
	content.WriteString("│" + footer + "│\n")
	content.WriteString(bottomBorder)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
