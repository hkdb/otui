package ui

import (
	"context"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
)

func (a AppView) handleSessionRenameMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		a.sessionRenameMode = false
		a.sessionRenameInput.Blur()
		return a, nil

	case "enter":
		newName := strings.TrimSpace(a.sessionRenameInput.Value())
		if newName == "" {
			return a, nil
		}

		sessionID := a.sessionList[a.selectedSessionIdx].ID
		a.sessionRenameMode = false
		a.sessionRenameInput.Blur()

		// Update current session name if it's the same session being renamed
		if a.dataModel.CurrentSession != nil && a.dataModel.CurrentSession.ID == sessionID {
			a.dataModel.CurrentSession.Name = newName
		}

		return a, a.dataModel.RenameSessionCmd(sessionID, newName)

	case "alt+u":
		a.sessionRenameInput.SetValue("")
		return a, nil
	}

	a.sessionRenameInput, cmd = a.sessionRenameInput.Update(msg)
	return a, cmd
}

func (a AppView) handlePassphraseForDataDir(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel passphrase entry
			a.showPassphraseForDataDir = false
			a.passphraseForDataDir.SetValue("")
			a.passphraseForDataDir.Blur()
			a.passphraseRetryDataDir = ""
			a.passphraseError = ""
			a.settingsSaveError = "Data directory switch cancelled (passphrase required)"
			return a, nil

		case "enter":
			passphrase := a.passphraseForDataDir.Value()
			if err := ValidatePassphraseNotEmpty(passphrase); err != nil {
				a.passphraseError = GetEmptyPassphraseError()
				return a, nil
			}

			// Retry ApplyDataDirSwitch with passphrase
			if err := a.dataModel.ApplyDataDirSwitch(a.passphraseRetryDataDir, passphrase); err != nil {
				// Still failed - wrong passphrase or other error
				a.passphraseError = GetIncorrectPassphraseError()
				a.passphraseForDataDir.SetValue("")
				return a, textinput.Blink
			}

			// Success - close passphrase modal
			a.showPassphraseForDataDir = false
			a.passphraseForDataDir.SetValue("")
			a.passphraseForDataDir.Blur()
			a.passphraseError = ""
			a.passphraseRetryDataDir = ""

			// Continue with steps 3c-6 (providers, plugins, UI refresh)
			// Step 3c: Re-initialize providers
			providerRefreshCmd := a.refreshProvidersAndModels()

			// Step 4: Re-create MCP manager if plugins enabled
			if a.dataModel.Config.PluginsEnabled {
				if err := a.ensureMCPManager(); err != nil && config.DebugLog != nil {
					config.DebugLog.Printf("[UI] Failed to recreate MCP manager: %v", err)
				}
			}

			// Step 6: Refresh UI
			a.resetUIStateForDataDirSwitch()

			// Step 5: Start plugins if enabled
			if a.dataModel.Config.PluginsEnabled && a.dataModel.MCPManager != nil {
				return a, tea.Batch(
					a.startPluginSystemWithModal(nil),
					a.dataModel.FetchSessionList(),
					providerRefreshCmd,
				)
			}

			return a, tea.Batch(
				a.dataModel.FetchSessionList(),
				providerRefreshCmd,
			)
		}

		// Update passphrase input
		a.passphraseForDataDir, cmd = a.passphraseForDataDir.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a AppView) handleSessionExportMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// If processing or cleaning up, only handle escape
	if a.exportingSession || a.exportCleaningUp {
		if msg.String() == "esc" && a.exportingSession && !a.exportCleaningUp {
			if a.exportCancelFunc != nil {
				a.exportCancelFunc()
			}
		}
		return a, nil
	}

	switch msg.String() {
	case "esc":
		a.sessionExportMode = false
		a.sessionExportInput.Blur()
		return a, nil

	case "enter":
		exportPath := strings.TrimSpace(a.sessionExportInput.Value())
		if exportPath == "" {
			return a, nil
		}

		sessionID := a.sessionList[a.selectedSessionIdx].ID

		// Expand path immediately to track it
		a.exportTargetPath = config.ExpandPath(exportPath)

		// Create cancellation context
		ctx, cancel := context.WithCancel(context.Background())
		a.exportCancelCtx = ctx
		a.exportCancelFunc = cancel

		// Initialize export spinner (reuse chat spinner style)
		a.exportSpinner = spinner.New()
		a.exportSpinner.Spinner = spinner.Dot
		a.exportSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

		// Set exporting state
		a.exportingSession = true
		a.sessionExportInput.Blur()

		// Start export with context and spinner tick
		return a, tea.Batch(
			a.dataModel.ExportSessionCmd(ctx, sessionID, a.exportTargetPath),
			a.exportSpinner.Tick,
		)

	case "alt+u":
		a.sessionExportInput.SetValue("")
		return a, nil
	}

	a.sessionExportInput, cmd = a.sessionExportInput.Update(msg)
	return a, cmd
}

func (a AppView) handleSessionImportMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Handle success acknowledgment
	if a.sessionImportPicker.Success != nil {
		if msg.String() == "enter" || msg.String() == "esc" {
			a.sessionImportSuccess = nil
			a.sessionImportPicker.Reset()
			return a, nil
		}
		return a, nil
	}

	// If processing, only handle escape
	if a.sessionImportPicker.Processing || a.sessionImportPicker.CleaningUp {
		if msg.String() == "esc" {
			if a.sessionImportCancelFunc != nil {
				a.sessionImportCancelFunc()
			}
		}
		return a, nil
	}

	switch msg.String() {
	case "esc":
		a.sessionImportPicker.Reset()
		return a, nil
	}

	// Update picker with the KeyMsg FIRST
	a.sessionImportPicker.Picker, cmd = a.sessionImportPicker.Picker.Update(msg)

	// Check if Path was set and it's a FILE (not directory)
	if a.sessionImportPicker.Picker.Path != "" {
		// Verify it's actually a file, not a directory
		if info, err := os.Stat(a.sessionImportPicker.Picker.Path); err == nil && !info.IsDir() {
			path := a.sessionImportPicker.Picker.Path

			if config.DebugLog != nil {
				config.DebugLog.Printf("FILE SELECTED: %s", path)
			}

			// Create cancellation context
			ctx, cancel := context.WithCancel(context.Background())
			a.sessionImportCancelCtx = ctx
			a.sessionImportCancelFunc = cancel

			// Start import
			a.sessionImportPicker.Processing = true

			if config.DebugLog != nil {
				config.DebugLog.Printf("SET Processing = true, Active = %v", a.sessionImportPicker.Active)
			}

			return a, tea.Batch(
				a.dataModel.ImportSessionCmd(ctx, path),
				a.sessionImportPicker.Spinner.Tick,
			)
		}
		// If it's a directory, clear Path so we don't trigger again
		a.sessionImportPicker.Picker.Path = ""
	}

	return a, cmd
}
