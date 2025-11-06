package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	"otui/mcp"
)

// handleConfigModalUpdate handles all input for the configure modal
func (a AppView) handleConfigModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	plugin := a.pluginManagerState.configModal.plugin
	if plugin == nil {
		return a, nil
	}

	fullSchema := mcp.BuildFullConfigSchema(plugin)
	numSchemaFields := len(fullSchema)
	numEnvVars := len(a.pluginManagerState.configModal.customEnvVars)

	// Calculate command field offset (command field is shown for manual/docker/binary)
	commandFieldOffset := 0
	if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
		commandFieldOffset = 1
	}

	// maxIdx accounts for: schema fields + command field (if shown) + env vars
	// +1 for "Add Environment Variable" is the final selectable item
	maxIdx := numSchemaFields + commandFieldOffset + numEnvVars

	// Handle adding new env var mode
	if a.pluginManagerState.configModal.addingCustomEnv {
		switch msg.String() {
		case "esc":
			a.pluginManagerState.configModal.addingCustomEnv = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvValInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
			a.pluginManagerState.configModal.customEnvValInput.SetValue("")
			a.pluginManagerState.configModal.customEnvFocusField = 0
		case "tab":
			// Toggle between key and value input
			if a.pluginManagerState.configModal.customEnvFocusField == 0 {
				a.pluginManagerState.configModal.customEnvFocusField = 1
				a.pluginManagerState.configModal.customEnvKeyInput.Blur()
				a.pluginManagerState.configModal.customEnvValInput.Focus()
			} else {
				a.pluginManagerState.configModal.customEnvFocusField = 0
				a.pluginManagerState.configModal.customEnvValInput.Blur()
				a.pluginManagerState.configModal.customEnvKeyInput.Focus()
			}
		case "enter":
			// Add the new env var
			key := strings.TrimSpace(a.pluginManagerState.configModal.customEnvKeyInput.Value())
			value := a.pluginManagerState.configModal.customEnvValInput.Value()
			if key != "" {
				a.pluginManagerState.configModal.customEnvVars = append(a.pluginManagerState.configModal.customEnvVars, EnvVarPair{
					Key:   key,
					Value: value,
				})
			}
			a.pluginManagerState.configModal.addingCustomEnv = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvValInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
			a.pluginManagerState.configModal.customEnvValInput.SetValue("")
			a.pluginManagerState.configModal.customEnvFocusField = 0
		default:
			// Forward to the appropriate input
			var cmd tea.Cmd
			if a.pluginManagerState.configModal.customEnvFocusField == 0 {
				a.pluginManagerState.configModal.customEnvKeyInput, cmd = a.pluginManagerState.configModal.customEnvKeyInput.Update(msg)
			} else {
				a.pluginManagerState.configModal.customEnvValInput, cmd = a.pluginManagerState.configModal.customEnvValInput.Update(msg)
			}
			return a, cmd
		}
		return a, nil
	}

	// Handle editing existing field/env var
	if a.pluginManagerState.configModal.editMode {
		selectedIdx := a.pluginManagerState.configModal.selectedIdx

		switch msg.String() {
		case "esc":
			a.pluginManagerState.configModal.editMode = false
			a.pluginManagerState.configModal.editInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvValInput.Blur()
			a.pluginManagerState.configModal.editInput.SetValue("")
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
			a.pluginManagerState.configModal.customEnvValInput.SetValue("")
			a.pluginManagerState.configModal.customEnvFocusField = 0
		case "tab":
			// For env vars, toggle between key and value
			if selectedIdx >= numSchemaFields && selectedIdx < numSchemaFields+numEnvVars {
				if a.pluginManagerState.configModal.customEnvFocusField == 0 {
					a.pluginManagerState.configModal.customEnvFocusField = 1
					a.pluginManagerState.configModal.customEnvKeyInput.Blur()
					a.pluginManagerState.configModal.customEnvValInput.Focus()
				} else {
					a.pluginManagerState.configModal.customEnvFocusField = 0
					a.pluginManagerState.configModal.customEnvValInput.Blur()
					a.pluginManagerState.configModal.customEnvKeyInput.Focus()
				}
			}
		case "alt+u":
			// Clear current input
			if selectedIdx < numSchemaFields {
				a.pluginManagerState.configModal.editInput.SetValue("")
			} else if a.pluginManagerState.configModal.customEnvFocusField == 0 {
				a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
			} else {
				a.pluginManagerState.configModal.customEnvValInput.SetValue("")
			}
		case "enter":
			// Save the edit
			commandFieldIdx := numSchemaFields
			commandFieldOffset := 0
			if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
				commandFieldOffset = 1
				if selectedIdx == commandFieldIdx {
					// Editing command field
					value := a.pluginManagerState.configModal.editInput.Value()
					a.pluginManagerState.configModal.fields["__command__"] = value
					a.pluginManagerState.configModal.editInput.SetValue("")
					a.pluginManagerState.configModal.editInput.Blur()
					a.pluginManagerState.configModal.editMode = false
					return a, nil
				}
			}

			if selectedIdx < numSchemaFields {
				// Editing schema field
				field := fullSchema[selectedIdx]
				value := a.pluginManagerState.configModal.editInput.Value()
				a.pluginManagerState.configModal.fields[field.Key] = value
				a.pluginManagerState.configModal.editInput.SetValue("")
				a.pluginManagerState.configModal.editInput.Blur()
			} else {
				// Editing env var
				envIdx := selectedIdx - numSchemaFields - commandFieldOffset
				if envIdx < len(a.pluginManagerState.configModal.customEnvVars) {
					newKey := strings.TrimSpace(a.pluginManagerState.configModal.customEnvKeyInput.Value())
					newVal := a.pluginManagerState.configModal.customEnvValInput.Value()
					if newKey != "" {
						a.pluginManagerState.configModal.customEnvVars[envIdx].Key = newKey
						a.pluginManagerState.configModal.customEnvVars[envIdx].Value = newVal
					}
					a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
					a.pluginManagerState.configModal.customEnvValInput.SetValue("")
					a.pluginManagerState.configModal.customEnvKeyInput.Blur()
					a.pluginManagerState.configModal.customEnvValInput.Blur()
				}
			}
			a.pluginManagerState.configModal.editMode = false
			a.pluginManagerState.configModal.customEnvFocusField = 0
		default:
			// Forward to appropriate input
			var cmd tea.Cmd
			if selectedIdx < numSchemaFields {
				a.pluginManagerState.configModal.editInput, cmd = a.pluginManagerState.configModal.editInput.Update(msg)
			} else {
				if a.pluginManagerState.configModal.customEnvFocusField == 0 {
					a.pluginManagerState.configModal.customEnvKeyInput, cmd = a.pluginManagerState.configModal.customEnvKeyInput.Update(msg)
				} else {
					a.pluginManagerState.configModal.customEnvValInput, cmd = a.pluginManagerState.configModal.customEnvValInput.Update(msg)
				}
			}
			return a, cmd
		}
		return a, nil
	}

	// Normal navigation mode
	switch msg.String() {
	case "esc":
		// Close modal without saving
		a.pluginManagerState.configModal.visible = false

		// If in pre-install mode, cancel the entire installation
		if a.pluginManagerState.configModal.preInstallMode {
			a.pluginManagerState.configModal.preInstallMode = false
			a.pluginManagerState.configModal.plugin = nil
			a.pluginManagerState.configModal.fields = nil
			a.pluginManagerState.configModal.customEnvVars = nil
			a.pluginManagerState.configModal.selectedIdx = 0
			// Don't proceed to installation - user cancelled
			return a, nil
		}

		// Regular mode - just close
		a.pluginManagerState.configModal.plugin = nil
		a.pluginManagerState.configModal.fields = nil
		a.pluginManagerState.configModal.customEnvVars = nil
		a.pluginManagerState.configModal.selectedIdx = 0
	case "up", "k", "shift+tab":
		if a.pluginManagerState.configModal.selectedIdx > 0 {
			a.pluginManagerState.configModal.selectedIdx--
		}
	case "down", "j", "tab":
		if a.pluginManagerState.configModal.selectedIdx < maxIdx {
			a.pluginManagerState.configModal.selectedIdx++
		}
	case "enter":
		selectedIdx := a.pluginManagerState.configModal.selectedIdx

		// Check if editing command field (for manual/docker/binary)
		commandFieldIdx := numSchemaFields
		commandFieldOffset := 0
		if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
			commandFieldOffset = 1
			if selectedIdx == commandFieldIdx {
				// Editing command field
				a.pluginManagerState.configModal.editMode = true
				a.pluginManagerState.configModal.editInput.Focus()
				a.pluginManagerState.configModal.editInput.SetValue(a.pluginManagerState.configModal.fields["__command__"])
				return a, textinput.Blink
			}
		}

		if selectedIdx == maxIdx {
			// User selected "Add Environment Variable"
			a.pluginManagerState.configModal.addingCustomEnv = true
			a.pluginManagerState.configModal.customEnvFocusField = 0
			a.pluginManagerState.configModal.customEnvKeyInput.Focus()
			return a, textinput.Blink
		} else if selectedIdx < numSchemaFields {
			// Editing schema field
			field := fullSchema[selectedIdx]
			a.pluginManagerState.configModal.editMode = true
			a.pluginManagerState.configModal.editInput.Focus()
			a.pluginManagerState.configModal.editInput.SetValue(a.pluginManagerState.configModal.fields[field.Key])
			return a, textinput.Blink
		} else {
			// Editing env var
			envIdx := selectedIdx - numSchemaFields - commandFieldOffset
			if envIdx < len(a.pluginManagerState.configModal.customEnvVars) {
				envVar := a.pluginManagerState.configModal.customEnvVars[envIdx]
				a.pluginManagerState.configModal.editMode = true
				a.pluginManagerState.configModal.customEnvFocusField = 0
				a.pluginManagerState.configModal.customEnvKeyInput.SetValue(envVar.Key)
				a.pluginManagerState.configModal.customEnvValInput.SetValue(envVar.Value)
				a.pluginManagerState.configModal.customEnvKeyInput.Focus()
				return a, textinput.Blink
			}
		}
	case "d":
		// Delete env var
		selectedIdx := a.pluginManagerState.configModal.selectedIdx
		firstEnvIdx := numSchemaFields + commandFieldOffset
		lastEnvIdx := firstEnvIdx + numEnvVars
		if selectedIdx >= firstEnvIdx && selectedIdx < lastEnvIdx {
			envIdx := selectedIdx - firstEnvIdx
			// Remove the env var
			a.pluginManagerState.configModal.customEnvVars = append(
				a.pluginManagerState.configModal.customEnvVars[:envIdx],
				a.pluginManagerState.configModal.customEnvVars[envIdx+1:]...,
			)
			// Adjust selection if needed
			if a.pluginManagerState.configModal.selectedIdx >= firstEnvIdx+len(a.pluginManagerState.configModal.customEnvVars) {
				a.pluginManagerState.configModal.selectedIdx--
			}
		}
	case "alt+enter":
		// Save all configuration
		if a.pluginManagerState.pluginState.Config != nil {
			// Merge schema fields and custom env vars
			mergedConfig := make(map[string]string)

			// Add schema fields
			for key, val := range a.pluginManagerState.configModal.fields {
				mergedConfig[key] = val
			}

			// Add custom env vars
			for _, envVar := range a.pluginManagerState.configModal.customEnvVars {
				mergedConfig[envVar.Key] = envVar.Value
			}

			a.pluginManagerState.pluginState.Config.SetPluginConfig(plugin.ID, mergedConfig)
			_ = config.SavePluginsConfig(a.dataModel.Config.DataDir(), a.pluginManagerState.pluginState.Config)
		}

		// Close modal
		a.pluginManagerState.configModal.visible = false

		// Check if we're in pre-install mode
		if a.pluginManagerState.configModal.preInstallMode {
			a.pluginManagerState.configModal.preInstallMode = false

			// Route based on install type
			if plugin.InstallType == "manual" || plugin.InstallType == "docker" {
				// Show manual instructions next
				return a.startManualInstall(plugin)
			} else {
				// Proceed to automatic installation
				return a.startPluginInstall(plugin)
			}
		}

		// Regular post-install configure mode - just close modal
		a.pluginManagerState.configModal.plugin = nil
		a.pluginManagerState.configModal.fields = nil
		a.pluginManagerState.configModal.customEnvVars = nil
		a.pluginManagerState.configModal.selectedIdx = 0
	}

	return a, nil
}

// handleAddCustomFormUpdate handles the multi-field custom plugin add form
func (a AppView) handleAddCustomFormUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	// Build fieldKeys dynamically based on install type
	installType := a.pluginManagerState.addCustomModal.fields["install_type"].Value()

	fieldKeys := []string{"repository", "id", "name", "install_type", "package"}

	// Add command if install type requires it
	if installType == "manual" || installType == "docker" || installType == "binary" {
		fieldKeys = append(fieldKeys, "command")
	}

	fieldKeys = append(fieldKeys, "description", "category", "language")

	// Calculate dynamic indices
	numTextFields := len(fieldKeys)
	envIndex := numTextFields       // Environment index
	argsIndex := numTextFields + 1  // Arguments index
	fieldCount := numTextFields + 2 // Total including env and args

	// Handle form submission (Alt+Enter)
	if msg.String() == "alt+enter" {
		// Collect all field values
		fields := a.pluginManagerState.addCustomModal.fields

		// Convert env vars to keys
		envKeys := make([]string, len(a.pluginManagerState.addCustomModal.envVars))
		for i, ev := range a.pluginManagerState.addCustomModal.envVars {
			envKeys[i] = ev.Key
		}

		// Extract author from repository URL
		repoURL := fields["repository"].Value()
		author := ""
		if repoPath, found := strings.CutPrefix(repoURL, "https://github.com/"); found {
			repoPath = strings.TrimSuffix(repoPath, ".git")
			parts := strings.Split(repoPath, "/")
			if len(parts) > 0 {
				author = parts[0]
			}
		}

		// Build plugin from form data
		plugin := &mcp.Plugin{
			ID:          fields["id"].Value(),
			Name:        fields["name"].Value(),
			Repository:  fields["repository"].Value(),
			Author:      author,
			InstallType: fields["install_type"].Value(),
			Package:     fields["package"].Value(),
			Command:     fields["command"].Value(),
			Description: fields["description"].Value(),
			Category:    fields["category"].Value(),
			Language:    fields["language"].Value(),
			Custom:      true,
			Environment: mcp.EnvVarsToString(envKeys),
			Args:        mcp.ArgsToString(a.pluginManagerState.addCustomModal.args),
		}

		// Validate required fields
		if plugin.Repository == "" || plugin.ID == "" || plugin.Name == "" || plugin.InstallType == "" || plugin.Package == "" {
			a.showAcknowledgeModal = true
			a.acknowledgeModalTitle = "Invalid Plugin Configuration"
			a.acknowledgeModalMsg = "Plugin configuration is missing required fields.\n\nPlease ensure Repository, ID, Name, Install Type, and Package are all filled in."
			a.acknowledgeModalType = ModalTypeError
			return a, nil
		}

		// Add to registry
		warnings, err := a.pluginManagerState.pluginState.Registry.AddCustomPluginDirect(plugin)
		if err != nil {
			a.pluginManagerState.installModal.error = fmt.Sprintf("Failed to add custom plugin: %v", err)
			a.pluginManagerState.addCustomModal.visible = false
			a.clearAddCustomFormState()
			return a, nil
		}

		a.pluginManagerState.addCustomModal.visible = false
		a.clearAddCustomFormState()

		a.pluginManagerState.warnings.showTrust = true
		a.pluginManagerState.warnings.trustPlugin = plugin
		a.pluginManagerState.warnings.trustWarnings = warnings

		return a, nil
	}

	// Handle Esc
	if msg.String() == "esc" {
		a.pluginManagerState.addCustomModal.visible = false
		a.clearAddCustomFormState()
		return a, nil
	}

	// Handle Enter key
	if msg.String() == "enter" {
		// If on Environment item (dynamic index), open env modal
		if a.pluginManagerState.addCustomModal.fieldIdx == envIndex {
			a.pluginManagerState.addCustomModal.showEnvModal = true
			a.pluginManagerState.addCustomModal.envIdx = 0
			return a, nil
		}
		// If on Arguments item (dynamic index), open args modal
		if a.pluginManagerState.addCustomModal.fieldIdx == argsIndex {
			a.pluginManagerState.addCustomModal.showArgsModal = true
			a.pluginManagerState.addCustomModal.argsIdx = 0
			return a, nil
		}
		// Otherwise, pass through to text input
	}

	// Handle Tab navigation
	if msg.String() == "tab" {
		// Blur current field if it's a text field
		if a.pluginManagerState.addCustomModal.fieldIdx < len(fieldKeys) {
			key := fieldKeys[a.pluginManagerState.addCustomModal.fieldIdx]
			field := a.pluginManagerState.addCustomModal.fields[key]
			field.Blur()
			a.pluginManagerState.addCustomModal.fields[key] = field
		}

		// Move to next item
		a.pluginManagerState.addCustomModal.fieldIdx = (a.pluginManagerState.addCustomModal.fieldIdx + 1) % fieldCount

		// Focus new field if it's a text field
		if a.pluginManagerState.addCustomModal.fieldIdx < len(fieldKeys) {
			key := fieldKeys[a.pluginManagerState.addCustomModal.fieldIdx]
			field := a.pluginManagerState.addCustomModal.fields[key]
			field.Focus()
			a.pluginManagerState.addCustomModal.fields[key] = field
			return a, textinput.Blink
		}

		return a, nil
	}

	// Handle Shift+Tab navigation
	if msg.String() == "shift+tab" {
		// Blur current field if it's a text field
		if a.pluginManagerState.addCustomModal.fieldIdx < len(fieldKeys) {
			key := fieldKeys[a.pluginManagerState.addCustomModal.fieldIdx]
			field := a.pluginManagerState.addCustomModal.fields[key]
			field.Blur()
			a.pluginManagerState.addCustomModal.fields[key] = field
		}

		// Move to previous item
		a.pluginManagerState.addCustomModal.fieldIdx = (a.pluginManagerState.addCustomModal.fieldIdx - 1 + fieldCount) % fieldCount

		// Focus new field if it's a text field
		if a.pluginManagerState.addCustomModal.fieldIdx < len(fieldKeys) {
			key := fieldKeys[a.pluginManagerState.addCustomModal.fieldIdx]
			field := a.pluginManagerState.addCustomModal.fields[key]
			field.Focus()
			a.pluginManagerState.addCustomModal.fields[key] = field
			return a, textinput.Blink
		}

		return a, nil
	}

	// Forward input to the focused field (only if on a text field)
	if a.pluginManagerState.addCustomModal.fieldIdx < len(fieldKeys) {
		key := fieldKeys[a.pluginManagerState.addCustomModal.fieldIdx]
		field := a.pluginManagerState.addCustomModal.fields[key]

		// Store old value for repository field
		oldRepoURL := ""
		if key == "repository" {
			oldRepoURL = field.Value()
		}

		var cmd tea.Cmd
		field, cmd = field.Update(msg)
		a.pluginManagerState.addCustomModal.fields[key] = field

		// If repository URL changed, auto-generate ID/Name and fetch metadata
		if key == "repository" {
			newRepoURL := field.Value()
			if newRepoURL != oldRepoURL && newRepoURL != "" && newRepoURL != a.pluginManagerState.addCustomModal.lastFetchedURL {
				// Auto-generate ID and Name
				repoPath := strings.TrimPrefix(newRepoURL, "https://github.com/")
				repoPath = strings.TrimSuffix(repoPath, ".git")
				parts := strings.Split(repoPath, "/")

				if len(parts) >= 2 {
					// Auto-fill ID
					idField := a.pluginManagerState.addCustomModal.fields["id"]
					generatedID := strings.ToLower(parts[0]) + "-" + strings.ToLower(parts[1])
					idField.SetValue(generatedID)
					a.pluginManagerState.addCustomModal.fields["id"] = idField

					// Auto-fill Name
					nameField := a.pluginManagerState.addCustomModal.fields["name"]
					nameField.SetValue(parts[0] + "/" + parts[1])
					a.pluginManagerState.addCustomModal.fields["name"] = nameField

					// Trigger GitHub metadata fetch
					a.pluginManagerState.addCustomModal.lastFetchedURL = newRepoURL
					return a, tea.Batch(cmd, fetchGitHubMetadata(newRepoURL))
				}
			}
		}

		return a, cmd
	}

	return a, nil
}

// clearAddCustomFormState clears all add custom form state
func (a *AppView) clearAddCustomFormState() {
	a.pluginManagerState.addCustomModal.fieldIdx = 0
	a.pluginManagerState.addCustomModal.envVars = nil
	a.pluginManagerState.addCustomModal.args = nil
	a.pluginManagerState.addCustomModal.lastFetchedURL = ""

	// Clear all field values
	for key := range a.pluginManagerState.addCustomModal.fields {
		field := a.pluginManagerState.addCustomModal.fields[key]
		field.SetValue("")
		field.Blur()
		a.pluginManagerState.addCustomModal.fields[key] = field
	}
}

// handleEnvEditModalUpdate handles environment variable editing in add form
func (a AppView) handleEnvEditModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	numEnvVars := len(a.pluginManagerState.addCustomModal.envVars)

	// Handle adding new env var
	if a.pluginManagerState.configModal.addingCustomEnv {
		switch msg.String() {
		case "esc":
			a.pluginManagerState.configModal.addingCustomEnv = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
		case "enter":
			key := strings.TrimSpace(a.pluginManagerState.configModal.customEnvKeyInput.Value())
			if key != "" {
				a.pluginManagerState.addCustomModal.envVars = append(a.pluginManagerState.addCustomModal.envVars, EnvVarPair{Key: key, Value: ""})
			}
			a.pluginManagerState.configModal.addingCustomEnv = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
		default:
			var cmd tea.Cmd
			a.pluginManagerState.configModal.customEnvKeyInput, cmd = a.pluginManagerState.configModal.customEnvKeyInput.Update(msg)
			return a, cmd
		}
		return a, nil
	}

	// Handle editing existing env var
	if a.pluginManagerState.addCustomModal.envEditMode {
		switch msg.String() {
		case "esc":
			a.pluginManagerState.addCustomModal.envEditMode = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
		case "enter":
			newKey := strings.TrimSpace(a.pluginManagerState.configModal.customEnvKeyInput.Value())
			if newKey != "" && a.pluginManagerState.addCustomModal.envIdx < len(a.pluginManagerState.addCustomModal.envVars) {
				a.pluginManagerState.addCustomModal.envVars[a.pluginManagerState.addCustomModal.envIdx].Key = newKey
			}
			a.pluginManagerState.addCustomModal.envEditMode = false
			a.pluginManagerState.configModal.customEnvKeyInput.Blur()
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue("")
		default:
			var cmd tea.Cmd
			a.pluginManagerState.configModal.customEnvKeyInput, cmd = a.pluginManagerState.configModal.customEnvKeyInput.Update(msg)
			return a, cmd
		}
		return a, nil
	}

	// Normal navigation
	switch msg.String() {
	case "esc":
		a.pluginManagerState.addCustomModal.showEnvModal = false
		a.pluginManagerState.addCustomModal.envIdx = 0
	case "j", "down":
		if a.pluginManagerState.addCustomModal.envIdx < numEnvVars {
			a.pluginManagerState.addCustomModal.envIdx++
		}
	case "k", "up":
		if a.pluginManagerState.addCustomModal.envIdx > 0 {
			a.pluginManagerState.addCustomModal.envIdx--
		}
	case "enter":
		if a.pluginManagerState.addCustomModal.envIdx == numEnvVars {
			// Add new env var
			a.pluginManagerState.configModal.addingCustomEnv = true
			a.pluginManagerState.configModal.customEnvKeyInput.Focus()
			return a, textinput.Blink
		} else if a.pluginManagerState.addCustomModal.envIdx < numEnvVars {
			// Edit existing
			a.pluginManagerState.addCustomModal.envEditMode = true
			a.pluginManagerState.configModal.customEnvKeyInput.SetValue(a.pluginManagerState.addCustomModal.envVars[a.pluginManagerState.addCustomModal.envIdx].Key)
			a.pluginManagerState.configModal.customEnvKeyInput.Focus()
			return a, textinput.Blink
		}
	case "d":
		if a.pluginManagerState.addCustomModal.envIdx < numEnvVars {
			// Delete env var
			a.pluginManagerState.addCustomModal.envVars = append(
				a.pluginManagerState.addCustomModal.envVars[:a.pluginManagerState.addCustomModal.envIdx],
				a.pluginManagerState.addCustomModal.envVars[a.pluginManagerState.addCustomModal.envIdx+1:]...,
			)
			if a.pluginManagerState.addCustomModal.envIdx >= len(a.pluginManagerState.addCustomModal.envVars) && a.pluginManagerState.addCustomModal.envIdx > 0 {
				a.pluginManagerState.addCustomModal.envIdx--
			}
		}
	}

	return a, nil
}

// handleArgsEditModalUpdate handles arguments editing in add form
func (a AppView) handleArgsEditModalUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	numArgs := len(a.pluginManagerState.addCustomModal.args)

	// If we're in the arg edit form (sub-modal)
	if a.pluginManagerState.addCustomModal.argsEditMode {
		return a.handleArgEditFormUpdate(msg)
	}

	// Normal navigation in args list
	switch msg.String() {
	case "esc":
		a.pluginManagerState.addCustomModal.showArgsModal = false
		a.pluginManagerState.addCustomModal.argsIdx = 0
	case "j", "down":
		if a.pluginManagerState.addCustomModal.argsIdx < numArgs {
			a.pluginManagerState.addCustomModal.argsIdx++
		}
	case "k", "up":
		if a.pluginManagerState.addCustomModal.argsIdx > 0 {
			a.pluginManagerState.addCustomModal.argsIdx--
		}
	case "enter":
		if a.pluginManagerState.addCustomModal.argsIdx == numArgs {
			// Add new argument - open arg edit form
			a.pluginManagerState.addCustomModal.argsEditMode = true
			a.pluginManagerState.addCustomModal.argFocusField = 0
			a.pluginManagerState.addCustomModal.argTypeHighlight = 0
			a.pluginManagerState.addCustomModal.argFlagInput.Focus()
			a.pluginManagerState.addCustomModal.argFlagInput.SetValue("")
			a.pluginManagerState.addCustomModal.argValueInput.SetValue("")
			a.pluginManagerState.addCustomModal.argLabelInput.SetValue("")
			return a, textinput.Blink
		}
	case "d":
		if a.pluginManagerState.addCustomModal.argsIdx < numArgs {
			// Delete argument
			a.pluginManagerState.addCustomModal.args = append(
				a.pluginManagerState.addCustomModal.args[:a.pluginManagerState.addCustomModal.argsIdx],
				a.pluginManagerState.addCustomModal.args[a.pluginManagerState.addCustomModal.argsIdx+1:]...,
			)
			if a.pluginManagerState.addCustomModal.argsIdx >= len(a.pluginManagerState.addCustomModal.args) && a.pluginManagerState.addCustomModal.argsIdx > 0 {
				a.pluginManagerState.addCustomModal.argsIdx--
			}
		}
	}

	return a, nil
}

// handleArgEditFormUpdate handles the add/edit argument form
func (a AppView) handleArgEditFormUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel arg edit
		a.pluginManagerState.addCustomModal.argsEditMode = false
		a.pluginManagerState.addCustomModal.argFlagInput.Blur()
		a.pluginManagerState.addCustomModal.argValueInput.Blur()
		a.pluginManagerState.addCustomModal.argLabelInput.Blur()
		return a, nil

	case "alt+enter":
		// Save argument
		flag := strings.TrimSpace(a.pluginManagerState.addCustomModal.argFlagInput.Value())
		if flag == "" {
			return a, nil
		}

		types := []string{"none", "value", "fixed"}
		selectedType := types[a.pluginManagerState.addCustomModal.argTypeHighlight]

		arg := mcp.ArgPair{
			Flag: flag,
			Type: selectedType,
		}

		if selectedType == "fixed" {
			arg.Value = a.pluginManagerState.addCustomModal.argValueInput.Value()
		} else if selectedType == "value" {
			arg.Label = a.pluginManagerState.addCustomModal.argLabelInput.Value()
		}

		a.pluginManagerState.addCustomModal.args = append(a.pluginManagerState.addCustomModal.args, arg)

		// Close arg edit form
		a.pluginManagerState.addCustomModal.argsEditMode = false
		a.pluginManagerState.addCustomModal.argFlagInput.Blur()
		a.pluginManagerState.addCustomModal.argValueInput.Blur()
		a.pluginManagerState.addCustomModal.argLabelInput.Blur()
		return a, nil

	case "tab":
		// Navigate to next field
		a.pluginManagerState.addCustomModal.argFocusField = (a.pluginManagerState.addCustomModal.argFocusField + 1) % 3
		a.updateArgEditFormFocus()
		return a, textinput.Blink

	case "shift+tab":
		// Navigate to previous field
		a.pluginManagerState.addCustomModal.argFocusField = (a.pluginManagerState.addCustomModal.argFocusField - 1 + 3) % 3
		a.updateArgEditFormFocus()
		return a, textinput.Blink

	case "j", "k", "down", "up":
		// Only cycle type if on type field
		if a.pluginManagerState.addCustomModal.argFocusField == 1 {
			if msg.String() == "j" || msg.String() == "down" {
				a.pluginManagerState.addCustomModal.argTypeHighlight = (a.pluginManagerState.addCustomModal.argTypeHighlight + 1) % 3
			} else {
				a.pluginManagerState.addCustomModal.argTypeHighlight = (a.pluginManagerState.addCustomModal.argTypeHighlight - 1 + 3) % 3
			}
		}
		return a, nil

	case " ", "enter":
		// Select type if on type field
		if a.pluginManagerState.addCustomModal.argFocusField == 1 {
			// Type selected - move to next field
			a.pluginManagerState.addCustomModal.argFocusField = 2
			a.updateArgEditFormFocus()
			return a, textinput.Blink
		}
		return a, nil

	case "alt+u":
		// Clear current field
		switch a.pluginManagerState.addCustomModal.argFocusField {
		case 0:
			a.pluginManagerState.addCustomModal.argFlagInput.SetValue("")
		case 2:
			types := []string{"none", "value", "fixed"}
			selectedType := types[a.pluginManagerState.addCustomModal.argTypeHighlight]
			if selectedType == "fixed" {
				a.pluginManagerState.addCustomModal.argValueInput.SetValue("")
			} else if selectedType == "value" {
				a.pluginManagerState.addCustomModal.argLabelInput.SetValue("")
			}
		}
		return a, nil

	default:
		// Forward to appropriate input
		var cmd tea.Cmd
		switch a.pluginManagerState.addCustomModal.argFocusField {
		case 0:
			a.pluginManagerState.addCustomModal.argFlagInput, cmd = a.pluginManagerState.addCustomModal.argFlagInput.Update(msg)
		case 2:
			types := []string{"none", "value", "fixed"}
			selectedType := types[a.pluginManagerState.addCustomModal.argTypeHighlight]
			switch selectedType {
			case "fixed":
				a.pluginManagerState.addCustomModal.argValueInput, cmd = a.pluginManagerState.addCustomModal.argValueInput.Update(msg)
			case "value":
				a.pluginManagerState.addCustomModal.argLabelInput, cmd = a.pluginManagerState.addCustomModal.argLabelInput.Update(msg)
			}
		}
		return a, cmd
	}
}

// updateArgEditFormFocus updates which input has focus in arg edit form
func (a *AppView) updateArgEditFormFocus() {
	a.pluginManagerState.addCustomModal.argFlagInput.Blur()
	a.pluginManagerState.addCustomModal.argValueInput.Blur()
	a.pluginManagerState.addCustomModal.argLabelInput.Blur()

	switch a.pluginManagerState.addCustomModal.argFocusField {
	case 0:
		a.pluginManagerState.addCustomModal.argFlagInput.Focus()
	case 2:
		types := []string{"none", "value", "fixed"}
		selectedType := types[a.pluginManagerState.addCustomModal.argTypeHighlight]
		switch selectedType {
		case "fixed":
			a.pluginManagerState.addCustomModal.argValueInput.Focus()
		case "value":
			a.pluginManagerState.addCustomModal.argLabelInput.Focus()
		}
	}
}

// handlePluginManagerUpdate handles keyboard input for the plugin manager
func (a AppView) handlePluginManagerUpdate(msg tea.KeyMsg) (AppView, tea.Cmd) {
	if a.pluginManagerState.warnings.showSecurity {
		switch msg.String() {
		case "y":
			plugin := a.pluginManagerState.warnings.securityPlugin
			a.pluginManagerState.warnings.showSecurity = false
			a.pluginManagerState.warnings.securityPlugin = nil
			return a.checkRuntimeAndProceed(plugin)
		case "n", "esc":
			a.pluginManagerState.warnings.showSecurity = false
			a.pluginManagerState.warnings.securityPlugin = nil
			return a, nil
		}
		return a, nil
	}

	if a.pluginManagerState.registryRefresh.visible {
		// Success or error phase - any key dismisses
		if a.pluginManagerState.registryRefresh.phase == "success" ||
			a.pluginManagerState.registryRefresh.phase == "error" {
			a.pluginManagerState.registryRefresh.visible = false
			a.pluginManagerState.registryRefresh.phase = ""
			a.pluginManagerState.registryRefresh.error = ""
			return a, nil
		}
		// Fetching phase - ignore keys (no cancellation)
		return a, nil
	}

	if a.pluginManagerState.warnings.showRuntimeMissing {
		a.pluginManagerState.warnings.showRuntimeMissing = false
		a.pluginManagerState.warnings.runtimeType = ""
		a.pluginManagerState.warnings.runtimePlugin = nil
		return a, nil
	}

	if a.pluginManagerState.warnings.showApiKey {
		switch msg.String() {
		case "y":
			plugin := a.pluginManagerState.warnings.apiKeyPlugin
			a.pluginManagerState.warnings.showApiKey = false
			a.pluginManagerState.warnings.apiKeyPlugin = nil
			return a.startPluginInstall(plugin)
		case "n", "esc":
			a.pluginManagerState.warnings.showApiKey = false
			a.pluginManagerState.warnings.apiKeyPlugin = nil
			return a, nil
		}
		return a, nil
	}

	if a.pluginManagerState.warnings.showManual {
		switch msg.String() {
		case "y":
			plugin := a.pluginManagerState.warnings.manualPlugin
			a.pluginManagerState.warnings.showManual = false
			a.pluginManagerState.warnings.manualPlugin = nil
			return a.completeManualInstall(plugin)
		case "n", "esc":
			plugin := a.pluginManagerState.warnings.manualPlugin
			a.pluginManagerState.warnings.showManual = false
			a.pluginManagerState.warnings.manualPlugin = nil
			return a.cancelManualInstall(plugin)
		}
		return a, nil
	}

	if a.pluginManagerState.confirmations.deletePlugin != nil {
		switch msg.String() {
		case "y":
			plugin := a.pluginManagerState.confirmations.deletePlugin
			a.pluginManagerState.confirmations.deletePlugin = nil
			return a.startPluginUninstall(plugin)
		case "n", "esc":
			a.pluginManagerState.confirmations.deletePlugin = nil
			return a, nil
		}
		return a, nil
	}

	if a.pluginManagerState.uninstallModal.visible {
		if a.pluginManagerState.uninstallModal.progress.Stage == "complete" || a.pluginManagerState.uninstallModal.error != "" {
			a.pluginManagerState.uninstallModal.visible = false
			a.pluginManagerState.uninstallModal.plugin = nil
			a.pluginManagerState.uninstallModal.error = ""
			a.pluginManagerState.uninstallModal.cancelFunc = nil

			a.pluginManagerState.detailsModal.visible = false
			a.pluginManagerState.detailsModal.plugin = nil

			if a.pluginManagerState.uninstallModal.progress.Stage == "complete" {
				a.pluginManagerState.selection.viewMode = "installed"
				a.pluginManagerState.selection.selectedPluginIdx = 0
				a.pluginManagerState.selection.scrollOffset = 0
			}
		}
		return a, nil
	}

	if a.pluginManagerState.configModal.visible {
		return a.handleConfigModalUpdate(msg)
	}

	if a.pluginManagerState.addCustomModal.visible {
		// Check for sub-modals
		if a.pluginManagerState.addCustomModal.showEnvModal {
			return a.handleEnvEditModalUpdate(msg)
		}
		if a.pluginManagerState.addCustomModal.showArgsModal {
			return a.handleArgsEditModalUpdate(msg)
		}

		// Handle main add custom form
		return a.handleAddCustomFormUpdate(msg)
	}

	if a.pluginManagerState.warnings.showTrust {
		switch msg.String() {
		case "y":
			a.pluginManagerState.warnings.showTrust = false
			a.pluginManagerState.warnings.trustPlugin = nil
			a.pluginManagerState.warnings.trustWarnings = nil

			a.pluginManagerState.selection.viewMode = "custom"
			a.pluginManagerState.selection.selectedPluginIdx = 0
			a.pluginManagerState.selection.scrollOffset = 0
			return a, nil
		case "n", "esc":
			pluginToRemove := a.pluginManagerState.warnings.trustPlugin
			if pluginToRemove != nil {
				_ = a.pluginManagerState.pluginState.Registry.RemoveCustomPlugin(pluginToRemove.ID)
			}

			a.pluginManagerState.warnings.showTrust = false
			a.pluginManagerState.warnings.trustPlugin = nil
			a.pluginManagerState.warnings.trustWarnings = nil
			return a, nil
		}
		return a, nil
	}

	if a.pluginManagerState.confirmations.deleteCustomPlugin != nil {
		switch msg.String() {
		case "y":
			plugin := a.pluginManagerState.confirmations.deleteCustomPlugin
			_ = a.pluginManagerState.pluginState.Registry.RemoveCustomPlugin(plugin.ID)

			if a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID) {
				_ = a.pluginManagerState.pluginState.Installer.Uninstall(plugin.ID)
			}

			a.pluginManagerState.confirmations.deleteCustomPlugin = nil
			a.pluginManagerState.selection.selectedPluginIdx = 0
			a.pluginManagerState.selection.scrollOffset = 0
			return a, nil
		case "n", "esc":
			a.pluginManagerState.confirmations.deleteCustomPlugin = nil
			return a, nil
		}
		return a, nil
	}

	if a.pluginManagerState.installModal.visible {
		switch msg.String() {
		case "esc":
			if a.pluginManagerState.installModal.progress.Stage != "complete" && a.pluginManagerState.installModal.error == "" {
				if a.pluginManagerState.installModal.cancelFunc != nil {
					a.pluginManagerState.installModal.cancelFunc()
					a.pluginManagerState.installModal.error = "Installation cancelled by user"
				}
			} else {
				a.pluginManagerState.installModal.visible = false
				a.pluginManagerState.installModal.plugin = nil
				a.pluginManagerState.installModal.error = ""
				a.pluginManagerState.installModal.cancelFunc = nil
			}
		default:
			if a.pluginManagerState.installModal.progress.Stage == "complete" || a.pluginManagerState.installModal.error != "" {
				installedPluginID := ""
				if a.pluginManagerState.installModal.plugin != nil {
					installedPluginID = a.pluginManagerState.installModal.plugin.ID
				}

				a.pluginManagerState.installModal.visible = false
				a.pluginManagerState.installModal.plugin = nil
				a.pluginManagerState.installModal.error = ""
				a.pluginManagerState.installModal.cancelFunc = nil

				a.pluginManagerState.detailsModal.visible = false
				a.pluginManagerState.detailsModal.plugin = nil

				if a.pluginManagerState.installModal.progress.Stage == "complete" && installedPluginID != "" {
					a.pluginManagerState.selection.viewMode = "installed"
					a.pluginManagerState.selection.selectedPluginIdx = 0
					a.pluginManagerState.selection.scrollOffset = 0

					installedPlugins := a.getVisiblePlugins()
					for i, p := range installedPlugins {
						if p.ID == installedPluginID {
							a.pluginManagerState.selection.selectedPluginIdx = i
							if i >= 10 {
								a.pluginManagerState.selection.scrollOffset = i - 9
							}
							break
						}
					}
				}
			}
		}
		return a, nil
	}

	if a.pluginManagerState.detailsModal.visible {
		switch msg.String() {
		case "esc":
			a.pluginManagerState.detailsModal.visible = false
			a.pluginManagerState.detailsModal.plugin = nil
		case "i":
			if !a.pluginManagerState.pluginState.Installer.IsInstalled(a.pluginManagerState.detailsModal.plugin.ID) {
				a.pluginManagerState.warnings.showSecurity = true
				a.pluginManagerState.warnings.securityPlugin = a.pluginManagerState.detailsModal.plugin
				return a, nil
			}
		case "u":
			if a.pluginManagerState.pluginState.Installer.IsInstalled(a.pluginManagerState.detailsModal.plugin.ID) {
				a.pluginManagerState.confirmations.deletePlugin = a.pluginManagerState.detailsModal.plugin
			}
		case "c":
			if a.pluginManagerState.pluginState.Installer.IsInstalled(a.pluginManagerState.detailsModal.plugin.ID) {
				plugin := a.pluginManagerState.detailsModal.plugin

				// Initialize config modal state
				a.pluginManagerState.configModal.visible = true
				a.pluginManagerState.configModal.plugin = plugin
				a.pluginManagerState.configModal.selectedIdx = 0
				a.pluginManagerState.configModal.editMode = false
				a.pluginManagerState.configModal.addingCustomEnv = false
				a.pluginManagerState.configModal.customEnvFocusField = 0

				// Initialize config fields from existing config
				a.pluginManagerState.configModal.fields = make(map[string]string)
				existingConfig := a.pluginManagerState.pluginState.Config.GetPluginConfig(plugin.ID)

				// Build full config schema (includes Environment and Args fields)
				fullSchema := mcp.BuildFullConfigSchema(plugin)

				// Load schema fields
				for _, field := range fullSchema {
					if val, ok := existingConfig[field.Key]; ok {
						a.pluginManagerState.configModal.fields[field.Key] = val
					} else {
						a.pluginManagerState.configModal.fields[field.Key] = field.DefaultValue
					}
				}

				// Load command for manual/docker/binary plugins
				if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
					if cmdValue, ok := existingConfig["__command__"]; ok {
						a.pluginManagerState.configModal.fields["__command__"] = cmdValue
					} else {
						a.pluginManagerState.configModal.fields["__command__"] = ""
					}
				}

				// Load custom env vars (those not in full schema)
				a.pluginManagerState.configModal.customEnvVars = []EnvVarPair{}
				schemaKeys := make(map[string]bool)
				for _, field := range fullSchema {
					schemaKeys[field.Key] = true
				}
				for key, val := range existingConfig {
					if !schemaKeys[key] {
						a.pluginManagerState.configModal.customEnvVars = append(a.pluginManagerState.configModal.customEnvVars, EnvVarPair{
							Key:   key,
							Value: val,
						})
					}
				}
			}
		}
		return a, nil
	}

	if a.pluginManagerState.selection.filterMode {
		plugins := a.getVisiblePlugins()
		switch msg.String() {
		case "esc":
			a.pluginManagerState.selection.filterMode = false
			a.pluginManagerState.selection.filterInput.Blur()
			a.pluginManagerState.selection.filterInput.SetValue("")
			a.pluginManagerState.selection.filteredPlugins = nil
		case "enter":
			if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
				a.pluginManagerState.detailsModal.visible = true
				a.pluginManagerState.detailsModal.plugin = &plugins[a.pluginManagerState.selection.selectedPluginIdx]
			}
		case "up", "alt+k":
			if a.pluginManagerState.selection.selectedPluginIdx > 0 {
				a.pluginManagerState.selection.selectedPluginIdx--
			}
		case "down", "alt+j":
			if a.pluginManagerState.selection.selectedPluginIdx < len(plugins)-1 {
				a.pluginManagerState.selection.selectedPluginIdx++
			}
		case "alt+i":
			if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
				plugin := &plugins[a.pluginManagerState.selection.selectedPluginIdx]
				if !a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID) {
					a.pluginManagerState.warnings.showSecurity = true
					a.pluginManagerState.warnings.securityPlugin = plugin
					return a, nil
				}
			}
		default:
			var cmd tea.Cmd
			a.pluginManagerState.selection.filterInput, cmd = a.pluginManagerState.selection.filterInput.Update(msg)

			query := a.pluginManagerState.selection.filterInput.Value()
			if query != "" {
				a.pluginManagerState.selection.filteredPlugins = a.pluginManagerState.pluginState.Registry.Search(query)
			}
			a.pluginManagerState.selection.selectedPluginIdx = 0
			a.pluginManagerState.selection.scrollOffset = 0

			return a, cmd
		}
		return a, nil
	}

	plugins := a.getVisiblePlugins()

	// Keybindings
	switch msg.String() {
	case "esc":
		a.showPluginManager = false
	case "up", "k":
		if a.pluginManagerState.selection.selectedPluginIdx > 0 {
			a.pluginManagerState.selection.selectedPluginIdx--
		}
	case "down", "j":
		if a.pluginManagerState.selection.selectedPluginIdx < len(plugins)-1 {
			a.pluginManagerState.selection.selectedPluginIdx++
		}
	case "enter":
		if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			a.pluginManagerState.detailsModal.visible = true
			a.pluginManagerState.detailsModal.plugin = &plugins[a.pluginManagerState.selection.selectedPluginIdx]
		}
	case "tab":
		modes := []string{"installed", "curated", "official", "automatic", "manual", "custom", "all"}
		for i, mode := range modes {
			if mode == a.pluginManagerState.selection.viewMode {
				a.pluginManagerState.selection.viewMode = modes[(i+1)%len(modes)]
				a.pluginManagerState.selection.selectedPluginIdx = 0
				a.pluginManagerState.selection.scrollOffset = 0
				break
			}
		}
	case "h", "left", "shift+tab":
		modes := []string{"installed", "curated", "official", "automatic", "manual", "custom", "all"}
		for i, mode := range modes {
			if mode == a.pluginManagerState.selection.viewMode {
				a.pluginManagerState.selection.viewMode = modes[(i-1+len(modes))%len(modes)]
				a.pluginManagerState.selection.selectedPluginIdx = 0
				a.pluginManagerState.selection.scrollOffset = 0
				break
			}
		}
	case "l", "right":
		modes := []string{"installed", "curated", "official", "automatic", "manual", "custom", "all"}
		for i, mode := range modes {
			if mode == a.pluginManagerState.selection.viewMode {
				a.pluginManagerState.selection.viewMode = modes[(i+1)%len(modes)]
				a.pluginManagerState.selection.selectedPluginIdx = 0
				a.pluginManagerState.selection.scrollOffset = 0
				break
			}
		}
	case "/":
		a.pluginManagerState.selection.filterMode = true
		a.pluginManagerState.selection.filterInput.Focus()
	case "alt+r":
		// Show modal with spinner
		a.pluginManagerState.registryRefresh.visible = true
		a.pluginManagerState.registryRefresh.phase = "fetching"
		a.pluginManagerState.registryRefresh.error = ""
		a.pluginManagerState.registryRefresh.spinner = spinner.New()
		a.pluginManagerState.registryRefresh.spinner.Spinner = spinner.Dot

		// Start the refresh command
		return a, tea.Batch(
			a.pluginManagerState.registryRefresh.spinner.Tick,
			a.dataModel.RefreshRegistry(),
		)
	case "i":
		if len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := &plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if !a.pluginManagerState.pluginState.Installer.IsInstalled(plugin.ID) {
				a.pluginManagerState.warnings.showSecurity = true
				a.pluginManagerState.warnings.securityPlugin = plugin
				return a, nil
			}
		}
	case "a":
		if a.pluginManagerState.selection.viewMode == "custom" {
			a.pluginManagerState.addCustomModal.visible = true
			a.pluginManagerState.addCustomModal.fieldIdx = 0

			// Focus first field (repository)
			field := a.pluginManagerState.addCustomModal.fields["repository"]
			field.Focus()
			a.pluginManagerState.addCustomModal.fields["repository"] = field

			return a, textinput.Blink
		}
	case "e":
		if a.pluginManagerState.selection.viewMode == "installed" && len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := &plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if a.pluginManagerState.pluginState.Config != nil {
				// Check if plugin is already enabled
				if a.pluginManagerState.pluginState.Config.GetPluginEnabled(plugin.ID) {
					// Show info modal - plugin already enabled
					a.showAcknowledgeModal = true
					a.acknowledgeModalTitle = "Plugin Already Enabled"
					a.acknowledgeModalMsg = plugin.Name + " is already enabled."
					a.acknowledgeModalType = ModalTypeInfo
					return a, nil
				}

				// Check if manual/docker/binary plugin has command configured
				if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
					existingConfig := a.pluginManagerState.pluginState.Config.GetPluginConfig(plugin.ID)
					cmdValue, hasCmdInConfig := existingConfig["__command__"]
					// Warn if no command in config AND no default command
					if (!hasCmdInConfig || cmdValue == "") && plugin.Command == "" {
						a.showAcknowledgeModal = true
						a.acknowledgeModalTitle = "Command Not Configured"
						a.acknowledgeModalMsg = "This plugin may not start properly without a startup command.\n\nPress 'c' to configure, or ESC to cancel."
						a.acknowledgeModalType = ModalTypeWarning
						return a, nil
					}
				}

				// Show modal with spinner (DON'T save config yet - wait for success)
				a.showPluginOperationModal = true
				a.pluginOperationPhase = "enabling"
				a.pluginOperationName = plugin.Name
				a.pluginOperationError = ""
				a.pluginOperationSpinner = spinner.New()
				a.pluginOperationSpinner.Spinner = spinner.Dot
				a.pluginOperationSpinner.Style = lipgloss.NewStyle().Foreground(successColor) // GREEN for enabling

				// Start the enable command
				return a, tea.Batch(
					a.pluginOperationSpinner.Tick,
					a.dataModel.EnablePlugin(plugin.ID),
				)
			}
		}
	case "d":
		if a.pluginManagerState.selection.viewMode == "custom" && len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := &plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if plugin.Custom {
				a.pluginManagerState.confirmations.deleteCustomPlugin = plugin
			}
		} else if a.pluginManagerState.selection.viewMode == "installed" && len(plugins) > 0 && a.pluginManagerState.selection.selectedPluginIdx < len(plugins) {
			plugin := &plugins[a.pluginManagerState.selection.selectedPluginIdx]
			if a.pluginManagerState.pluginState.Config != nil {
				// Check if plugin is already disabled
				if !a.pluginManagerState.pluginState.Config.GetPluginEnabled(plugin.ID) {
					// Show info modal - plugin already disabled
					a.showAcknowledgeModal = true
					a.acknowledgeModalTitle = "Plugin Already Disabled"
					a.acknowledgeModalMsg = plugin.Name + " is already disabled."
					a.acknowledgeModalType = ModalTypeInfo
					return a, nil
				}

				// Save config as disabled IMMEDIATELY (user intent to disable)
				a.pluginManagerState.pluginState.Config.SetPluginEnabled(plugin.ID, false)
				_ = config.SavePluginsConfig(a.dataModel.Config.DataDir(), a.pluginManagerState.pluginState.Config)

				// Show modal with spinner
				a.showPluginOperationModal = true
				a.pluginOperationPhase = "disabling"
				a.pluginOperationName = plugin.Name
				a.pluginOperationError = ""
				a.pluginOperationSpinner = spinner.New()
				a.pluginOperationSpinner.Spinner = spinner.Dot

				// Start the disable command
				return a, tea.Batch(
					a.pluginOperationSpinner.Tick,
					a.dataModel.DisablePlugin(plugin.ID),
				)
			}
		}
	}

	return a, nil
}

// startPluginInstall initiates plugin installation
func (a AppView) startPluginInstall(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize install spinner
	a.pluginManagerState.installModal.spinner = spinner.New()
	a.pluginManagerState.installModal.spinner.Spinner = spinner.Dot
	a.pluginManagerState.installModal.spinner.Style = lipgloss.NewStyle().Foreground(successColor)

	a.pluginManagerState.installModal.visible = true
	a.pluginManagerState.installModal.plugin = plugin
	a.pluginManagerState.installModal.progress = mcp.InstallProgress{Stage: "checking", Percent: 0, Message: "Checking requirements..."}
	a.pluginManagerState.installModal.error = ""
	a.pluginManagerState.installModal.cancelFunc = cancel
	a.pluginManagerState.installModal.progressCh = make(chan mcp.InstallProgress, 10)

	go func() {
		err := a.pluginManagerState.pluginState.Installer.InstallWithContext(ctx, plugin, a.pluginManagerState.installModal.progressCh)
		if err != nil {
			a.pluginManagerState.installModal.progressCh <- mcp.InstallProgress{Stage: "error", Percent: 0, Message: err.Error()}
		}
		close(a.pluginManagerState.installModal.progressCh)
	}()

	return a, tea.Batch(
		a.pluginManagerState.installModal.spinner.Tick,
		a.waitForInstallProgress,
	)
}

// Message types for install/uninstall progress
type installProgressMsg mcp.InstallProgress
type installErrorMsg struct{ err string }
type uninstallProgressMsg mcp.InstallProgress
type githubMetadataMsg struct {
	repoURL     string
	author      string
	description string
	language    string
	stars       int
	err         error
}

// waitForInstallProgress waits for installation progress updates
func (a AppView) waitForInstallProgress() tea.Msg {
	if a.pluginManagerState.installModal.progressCh == nil {
		return nil
	}

	progress, ok := <-a.pluginManagerState.installModal.progressCh
	if !ok {
		return nil
	}

	if progress.Stage == "error" {
		return installErrorMsg{err: progress.Message}
	}

	return installProgressMsg(progress)
}

// waitForUninstallProgress waits for uninstallation progress updates
func (a AppView) waitForUninstallProgress() tea.Msg {
	if a.pluginManagerState.uninstallModal.progressCh == nil {
		return nil
	}

	progress, ok := <-a.pluginManagerState.uninstallModal.progressCh
	if !ok {
		return nil
	}

	return uninstallProgressMsg(progress)
}

// checkRuntimeAndProceed checks for required runtime and proceeds with installation
func (a AppView) checkRuntimeAndProceed(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	// For manual and docker installs, show config before manual instructions
	if plugin.InstallType == "manual" || plugin.InstallType == "docker" {
		return a.showPreInstallConfig(plugin)
	}

	if plugin.InstallType == "binary" {
		return a.checkApiKeyAndProceed(plugin)
	}

	var runtimeToCheck string

	switch plugin.Language {
	case "typescript", "javascript":
		runtimeToCheck = "node"
	case "python":
		runtimeToCheck = "python"
	case "go":
		runtimeToCheck = "go"
	default:
		return a.checkApiKeyAndProceed(plugin)
	}

	runtimeChecker := mcp.NewRuntimeChecker()
	_, err := runtimeChecker.CheckRuntime(runtimeToCheck)

	if err != nil {
		a.pluginManagerState.warnings.showRuntimeMissing = true
		a.pluginManagerState.warnings.runtimeType = runtimeToCheck
		a.pluginManagerState.warnings.runtimePlugin = plugin
		return a, nil
	}

	if plugin.InstallType == "npm" {
		_, err := runtimeChecker.CheckRuntime("npx")
		if err != nil {
			a.pluginManagerState.warnings.showRuntimeMissing = true
			a.pluginManagerState.warnings.runtimeType = "npm"
			a.pluginManagerState.warnings.runtimePlugin = plugin
			return a, nil
		}
	}

	return a.checkApiKeyAndProceed(plugin)
}

// checkApiKeyAndProceed checks for required API key and proceeds with installation
func (a AppView) checkApiKeyAndProceed(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	if plugin.RequiresKey {
		a.pluginManagerState.warnings.showApiKey = true
		a.pluginManagerState.warnings.apiKeyPlugin = plugin
		return a, nil
	}

	// Show configuration screen before installation
	return a.showPreInstallConfig(plugin)
}

// showPreInstallConfig opens the configuration modal before installation
func (a AppView) showPreInstallConfig(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	// Initialize config modal state (same pattern as regular configure)
	a.pluginManagerState.configModal.visible = true
	a.pluginManagerState.configModal.plugin = plugin
	a.pluginManagerState.configModal.selectedIdx = 0
	a.pluginManagerState.configModal.editMode = false
	a.pluginManagerState.configModal.addingCustomEnv = false
	a.pluginManagerState.configModal.customEnvFocusField = 0
	a.pluginManagerState.configModal.preInstallMode = true // Flag for pre-install mode

	// Initialize config fields with defaults
	a.pluginManagerState.configModal.fields = make(map[string]string)

	// Build full config schema (includes Environment and Args fields)
	fullSchema := mcp.BuildFullConfigSchema(plugin)

	// Load default values from schema
	for _, field := range fullSchema {
		a.pluginManagerState.configModal.fields[field.Key] = field.DefaultValue
	}

	// Initialize command for manual/docker/binary plugins
	if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
		// Use plugin's default command if available
		if plugin.Command != "" {
			a.pluginManagerState.configModal.fields["__command__"] = plugin.Command
		} else {
			a.pluginManagerState.configModal.fields["__command__"] = ""
		}
	}

	// Initialize empty custom env vars
	a.pluginManagerState.configModal.customEnvVars = []EnvVarPair{}

	return a, nil
}

// startManualInstall shows manual installation instructions
func (a AppView) startManualInstall(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	a.pluginManagerState.warnings.showManual = true
	a.pluginManagerState.warnings.manualPlugin = plugin
	return a, nil
}

// completeManualInstall marks a manual plugin as installed
func (a AppView) completeManualInstall(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	ctx := context.Background()
	progressCh := make(chan mcp.InstallProgress, 10)

	go func() {
		err := a.pluginManagerState.pluginState.Installer.InstallWithContext(ctx, plugin, progressCh)
		if err != nil {
			progressCh <- mcp.InstallProgress{Stage: "error", Percent: 0, Message: err.Error()}
		}
		close(progressCh)
	}()

	a.pluginManagerState.installModal.visible = true
	a.pluginManagerState.installModal.plugin = plugin
	a.pluginManagerState.installModal.progress = mcp.InstallProgress{Stage: "complete", Percent: 100, Message: "Plugin marked as installed"}
	a.pluginManagerState.installModal.progressCh = progressCh

	return a, a.waitForInstallProgress
}

// cancelManualInstall cancels manual installation
func (a AppView) cancelManualInstall(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	return a, nil
}

// startPluginUninstall initiates plugin uninstallation
func (a AppView) startPluginUninstall(plugin *mcp.Plugin) (AppView, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize uninstall spinner
	a.pluginManagerState.uninstallModal.spinner = spinner.New()
	a.pluginManagerState.uninstallModal.spinner.Spinner = spinner.Dot
	a.pluginManagerState.uninstallModal.spinner.Style = lipgloss.NewStyle().Foreground(accentColor)

	a.pluginManagerState.uninstallModal.visible = true
	a.pluginManagerState.uninstallModal.plugin = plugin
	a.pluginManagerState.uninstallModal.progress = mcp.InstallProgress{Stage: "starting", Percent: 0, Message: "Starting uninstall..."}
	a.pluginManagerState.uninstallModal.error = ""
	a.pluginManagerState.uninstallModal.cancelFunc = cancel
	a.pluginManagerState.uninstallModal.progressCh = make(chan mcp.InstallProgress, 10)

	go func() {
		err := a.pluginManagerState.pluginState.Installer.UninstallWithContext(ctx, plugin.ID, a.pluginManagerState.uninstallModal.progressCh)
		if err != nil {
			a.pluginManagerState.uninstallModal.progressCh <- mcp.InstallProgress{Stage: "error", Percent: 0, Message: err.Error()}
		}
		close(a.pluginManagerState.uninstallModal.progressCh)
	}()

	return a, tea.Batch(
		a.pluginManagerState.uninstallModal.spinner.Tick,
		a.waitForUninstallProgress,
	)
}

// fetchGitHubMetadata fetches metadata from GitHub for a repository
func fetchGitHubMetadata(repoURL string) tea.Cmd {
	return func() tea.Msg {
		// Validate URL
		if !strings.HasPrefix(repoURL, "https://github.com/") {
			return githubMetadataMsg{repoURL: repoURL, err: fmt.Errorf("invalid GitHub URL")}
		}

		// Extract owner/repo from URL
		repoPath := strings.TrimPrefix(repoURL, "https://github.com/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		parts := strings.Split(repoPath, "/")
		if len(parts) < 2 {
			return githubMetadataMsg{repoURL: repoURL, err: fmt.Errorf("invalid repository URL")}
		}

		// Use first two parts (owner/repo)
		repoPath = parts[0] + "/" + parts[1]

		// Fetch from GitHub API
		info, err := mcp.FetchGitHubRepoInfo(repoPath)
		if err != nil {
			return githubMetadataMsg{repoURL: repoURL, err: err}
		}

		return githubMetadataMsg{
			repoURL:     repoURL,
			author:      parts[0],
			description: info.Description,
			language:    info.Language,
			stars:       info.StargazersCount,
			err:         nil,
		}
	}
}
