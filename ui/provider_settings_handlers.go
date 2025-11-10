package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// handleProviderSettingsInput handles keyboard input for provider settings sub-screen
func (a AppView) handleProviderSettingsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle confirmation modal if active (takes priority over everything)
	if a.providerSettingsConfirmExit {
		switch msg.String() {
		case "y", "Y":
			// Discard changes and close
			a.providerSettingsConfirmExit = false
			a.providerSettingsState.visible = false
			a.providerSettingsState.hasChanges = false
			return a, nil
		case "n", "N", "esc":
			// Return to provider settings
			a.providerSettingsConfirmExit = false
			return a, nil
		}
		return a, nil
	}

	// Edit mode handling
	if a.providerSettingsState.editMode {
		switch msg.String() {
		case "enter":
			// Save field value
			return a.saveProviderField()
		case "esc":
			// Cancel edit
			a.providerSettingsState.editMode = false
			a.providerSettingsState.editInput.Blur()
			return a, nil
		default:
			// Update textinput
			var cmd tea.Cmd
			a.providerSettingsState.editInput, cmd = a.providerSettingsState.editInput.Update(msg)
			return a, cmd
		}
	}

	// Navigation mode handling
	switch msg.String() {
	case "h", "left", "shift+tab":
		// Previous tab (with wrap) - DON'T reload cache, preserves unsaved changes
		for i, id := range providerTabs {
			if id == a.providerSettingsState.selectedProviderID {
				a.providerSettingsState.selectedProviderID = providerTabs[(i-1+len(providerTabs))%len(providerTabs)]
				a.providerSettingsState.selectedFieldIdx = 0
				break
			}
		}
		return a, nil

	case "l", "right", "tab":
		// Next tab (with wrap) - DON'T reload cache, preserves unsaved changes
		for i, id := range providerTabs {
			if id == a.providerSettingsState.selectedProviderID {
				a.providerSettingsState.selectedProviderID = providerTabs[(i+1)%len(providerTabs)]
				a.providerSettingsState.selectedFieldIdx = 0
				break
			}
		}
		return a, nil

	case "j", "down":
		// Next field (with wrap)
		currentFields := a.providerSettingsState.currentFieldsMap[a.providerSettingsState.selectedProviderID]
		if len(currentFields) > 0 {
			a.providerSettingsState.selectedFieldIdx = (a.providerSettingsState.selectedFieldIdx + 1) % len(currentFields)
		}
		return a, nil

	case "k", "up":
		// Previous field (with wrap)
		currentFields := a.providerSettingsState.currentFieldsMap[a.providerSettingsState.selectedProviderID]
		if len(currentFields) > 0 {
			a.providerSettingsState.selectedFieldIdx = (a.providerSettingsState.selectedFieldIdx - 1 + len(currentFields)) % len(currentFields)
		}
		return a, nil

	case "enter", " ":
		// Enter edit mode (or toggle for boolean fields)
		if config.DebugLog != nil {
			config.DebugLog.Printf("[ProviderSettings] Enter pressed, selectedIdx=%d", a.providerSettingsState.selectedFieldIdx)
		}

		providerID := a.providerSettingsState.selectedProviderID
		currentFields := a.providerSettingsState.currentFieldsMap[providerID]
		if a.providerSettingsState.selectedFieldIdx < len(currentFields) {
			field := currentFields[a.providerSettingsState.selectedFieldIdx]

			if config.DebugLog != nil {
				config.DebugLog.Printf("[ProviderSettings] Field: label=%s, type=%d, value=%s", field.Label, field.Type, field.Value)
				config.DebugLog.Printf("[ProviderSettings] ProviderFieldTypeEnabled=%d, match=%v", ProviderFieldTypeEnabled, field.Type == ProviderFieldTypeEnabled)
			}

			// Special handling: Toggle Enabled field in cache (don't save yet)
			if field.Type == ProviderFieldTypeEnabled {
				if config.DebugLog != nil {
					config.DebugLog.Printf("[ProviderSettings] TOGGLE ENABLED FIELD - current=%s", field.Value)
				}

				// Determine new value
				var newValue string
				switch field.Value {
				case "true":
					newValue = "false"
				case "false":
					newValue = "true"
				}

				if config.DebugLog != nil {
					config.DebugLog.Printf("[ProviderSettings] Toggle: %s -> %s (in cache only, wait for Alt+Enter)", field.Value, newValue)
				}

				// Modify map directly (not copy!)
				a.providerSettingsState.currentFieldsMap[providerID][a.providerSettingsState.selectedFieldIdx].Value = newValue

				// DEBUG: Verify map was actually updated
				if config.DebugLog != nil {
					verifyValue := a.providerSettingsState.currentFieldsMap[providerID][a.providerSettingsState.selectedFieldIdx].Value
					config.DebugLog.Printf("[ProviderSettings] ✅ VERIFY: Map[%s][%d].Value is now: %s", providerID, a.providerSettingsState.selectedFieldIdx, verifyValue)
				}

				// Mark as changed but don't save yet
				a.providerSettingsState.hasChanges = true
				return a, nil
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("[ProviderSettings] NOT enabled field type, entering edit mode")
			}

			// Normal text edit mode for other fields
			actualValue := a.getProviderFieldActualValue(field)

			a.providerSettingsState.editInput.SetValue(actualValue)
			a.providerSettingsState.editInput.Focus()
			a.providerSettingsState.editMode = true
			return a, textinput.Blink
		}
		return a, nil

	case "alt+enter":
		// Close modal
		a.providerSettingsState.visible = false
		a.providerSettingsState.hasChanges = false

		// Save all providers' fields synchronously (Settings screen pattern)
		return a, a.saveAllProviderFieldsCmd()

	case "esc":
		// Check for unsaved changes
		if a.providerSettingsState.hasChanges {
			a.providerSettingsConfirmExit = true
			return a, nil
		}

		// Close without confirmation if no changes
		a.providerSettingsState.visible = false
		return a, nil
	}

	return a, nil
}

// saveProviderField updates the cache with edited field value (doesn't save to disk yet)
func (a AppView) saveProviderField() (tea.Model, tea.Cmd) {
	providerID := a.providerSettingsState.selectedProviderID
	currentFields := a.providerSettingsState.currentFieldsMap[providerID]

	if a.providerSettingsState.selectedFieldIdx >= len(currentFields) {
		a.providerSettingsState.editMode = false
		return a, nil
	}

	newValue := a.providerSettingsState.editInput.Value()

	// Modify map directly (not copy!)
	a.providerSettingsState.currentFieldsMap[providerID][a.providerSettingsState.selectedFieldIdx].Value = newValue

	// Exit edit mode
	a.providerSettingsState.editMode = false
	a.providerSettingsState.editInput.Blur()
	a.providerSettingsState.hasChanges = true

	// Don't save yet - wait for Alt+Enter
	return a, nil
}

// getProviderFieldActualValue gets unmasked field value
func (a AppView) getProviderFieldActualValue(field ProviderField) string {
	switch field.Type {
	case ProviderFieldTypeHost:
		return a.dataModel.Config.OllamaHost
	case ProviderFieldTypeAPIKey:
		if a.dataModel.Config.CredentialStore != nil {
			key := a.dataModel.Config.CredentialStore.Get(a.providerSettingsState.selectedProviderID)
			return key
		}
		return ""
	case ProviderFieldTypeEnabled:
		return field.Value // "true" or "false"
	default:
		return field.Value
	}
}

type providerFieldSavedMsg struct {
	success bool
	err     error
	cfg     *config.Config
}

// saveAllProviderFieldsCmd saves all providers' fields from cache (Settings screen pattern)
func (a AppView) saveAllProviderFieldsCmd() tea.Cmd {
	return func() tea.Msg {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[SaveCmd] ========== STARTING BATCH SAVE ==========")
		}

		// Loop through all providers and save their fields
		for providerID, fields := range a.providerSettingsState.currentFieldsMap {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[SaveCmd] Processing provider: %s (has %d fields)", providerID, len(fields))
			}

			for i, field := range fields {
				var fieldName string
				switch field.Type {
				case ProviderFieldTypeHost:
					fieldName = "host"
				case ProviderFieldTypeAPIKey:
					fieldName = "apikey"
				case ProviderFieldTypeEnabled:
					fieldName = "enabled"
				}

				if config.DebugLog != nil {
					config.DebugLog.Printf("[SaveCmd]   Field[%d]: %s (%s) = '%s'", i, field.Label, fieldName, field.Value)
				}

				// Get actual value (unmask API keys if not edited)
				actualValue := field.Value
				if field.Type == ProviderFieldTypeAPIKey {
					// If value looks masked, get actual value from config
					if field.Value == "(not set)" || strings.Contains(field.Value, "***") {
						if a.dataModel.Config.CredentialStore != nil {
							actualValue = a.dataModel.Config.CredentialStore.Get(providerID)
							if config.DebugLog != nil {
								config.DebugLog.Printf("[SaveCmd]     API Key was masked, using actual value from config")
							}
						}
					}
					// Otherwise user edited it, use new value
				}

				if config.DebugLog != nil {
					config.DebugLog.Printf("[SaveCmd]   💾 Calling config.UpdateProviderField(%s, %s, %s)", providerID, fieldName, actualValue)
				}

				// Save this field
				err := config.UpdateProviderField(
					a.dataModel.Config.DataDir(),
					providerID,
					fieldName,
					actualValue,
				)

				if err != nil {
					return providerFieldsSavedMsg{success: false, err: err}
				}
			}
		}

		// All fields saved successfully - reload config
		newCfg, err := config.Load()
		if err != nil {
			return providerFieldsSavedMsg{success: false, err: err}
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("[SaveCmd] ========== RETURNING providerFieldsSavedMsg ==========")
			config.DebugLog.Printf("[SaveCmd] Reloaded config has %d providers", len(newCfg.Providers))
			for _, p := range newCfg.Providers {
				config.DebugLog.Printf("[SaveCmd]   Provider: %s, Enabled: %v", p.ID, p.Enabled)
			}
		}

		return providerFieldsSavedMsg{success: true, cfg: newCfg}
	}
}

// providerFieldsSavedMsg is returned after saving ALL providers' fields
type providerFieldsSavedMsg struct {
	success bool
	err     error
	cfg     *config.Config
}
