package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"otui/mcp"
)

// renderInstallModal renders the plugin installation progress modal
func (a *AppView) renderInstallModal() string {
	if a.pluginManagerState.installModal.plugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.installModal.plugin
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	// Title section
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(fmt.Sprintf("Installing: %s", plugin.Name))

	// Build message section based on state
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	if a.pluginManagerState.installModal.error != "" {
		// Error state
		errorText := "Error: " + a.pluginManagerState.installModal.error
		errorLines := strings.Split(errorText, "\n")
		errorStyle := lipgloss.NewStyle().
			Foreground(dangerColor).
			Width(modalWidth)

		for _, line := range errorLines {
			wrapped := wrapText(line, modalWidth-4)
			for _, wrappedLine := range wrapped {
				messageLines = append(messageLines, errorStyle.Render(wrappedLine))
			}
		}
	} else {
		// Progress state
		progress := a.pluginManagerState.installModal.progress

		// Show spinner + message during progress (not on complete)
		var progressMessage string
		if progress.Stage == "complete" {
			progressMessage = progress.Message
		} else {
			spinnerView := a.pluginManagerState.installModal.spinner.View()
			progressMessage = spinnerView + " " + progress.Message
		}
		messageLines = append(messageLines, messageStyle.Render(progressMessage))

		progressBar := renderProgressBar(int(progress.Percent), modalWidth-4)
		messageLines = append(messageLines, messageStyle.Render(progressBar))

		if progress.Stage == "complete" {
			messageLines = append(messageLines, messageStyle.Render(""))

			successStyle := lipgloss.NewStyle().
				Foreground(accentColor).
				Align(lipgloss.Center).
				Width(modalWidth)
			messageLines = append(messageLines, successStyle.Render("Installation complete!"))
			messageLines = append(messageLines, messageStyle.Render(""))

			reminderStyle := lipgloss.NewStyle().
				Foreground(successColor).
				Align(lipgloss.Center).
				Width(modalWidth)
			messageLines = append(messageLines, reminderStyle.Render("You can enable this plugin in the Installed tab"))
		}
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Footer section - different based on state
	var footer string
	if a.pluginManagerState.installModal.error != "" {
		footer = "Press any key to close"
	} else if a.pluginManagerState.installModal.progress.Stage == "complete" {
		footer = "Press any key to continue"
	} else {
		footer = "Press Esc to cancel"
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderConfigModal is a pure function that renders the plugin configuration modal
func renderConfigModal(
	a AppView,
	plugin *mcp.Plugin,
	configFields map[string]string,
	customEnvVars []EnvVarPair,
	selectedConfigIdx int,
	configEditMode bool,
	configEditInput textinput.Model,
	addingCustomEnv bool,
	customEnvKeyInput textinput.Model,
	customEnvValInput textinput.Model,
	customEnvFocusField int,
	width, height int,
) string {
	if plugin == nil {
		return ""
	}

	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
	}

	// Title section
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(fmt.Sprintf("Configure: %s", plugin.Name))

	// Message section
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	// For remote plugins, add explanatory header about env vars and auth
	switch plugin.InstallType {
	case "remote":
		headerStyle := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Width(modalWidth)
		messageLines = append(messageLines, headerStyle.Render("Environment Variables (sent as HTTP headers):"))
		messageLines = append(messageLines, messageStyle.Render(""))

		hintStyle := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Width(modalWidth)

		// Show auth type specific hints
		switch plugin.AuthType {
		case "oauth":
			messageLines = append(messageLines,
				hintStyle.Render("  Required: OAUTH_CLIENT_ID, OAUTH_REDIRECT_URI"))
			messageLines = append(messageLines,
				hintStyle.Render("  Optional: OAUTH_CLIENT_SECRET, OAUTH_SCOPES"))
		case "headers":
			messageLines = append(messageLines,
				hintStyle.Render("  Common: Authorization, X-API-Key, X-Custom-Header"))
		case "none":
			messageLines = append(messageLines,
				hintStyle.Render("  No authentication configured"))
		}
		messageLines = append(messageLines, messageStyle.Render(""))
	}

	// Build full config schema (includes Environment and Args fields)
	fullSchema := mcp.BuildFullConfigSchema(plugin)

	// Render ConfigSchema fields (if any)
	numSchemaFields := len(fullSchema)
	if numSchemaFields > 0 {
		for i, field := range fullSchema {
			isSelected := !addingCustomEnv && i == selectedConfigIdx
			isEditing := configEditMode && isSelected

			indicator := "  "
			if isSelected && !isEditing {
				indicator = "▶ "
			}

			label := field.Label
			if field.Required {
				label += " *"
			}

			value := configFields[field.Key]
			if value == "" {
				value = lipgloss.NewStyle().Foreground(dimColor).Render("<not set>")
			}

			var fieldLine string
			if isEditing {
				fieldLine = fmt.Sprintf("%s%s: %s", indicator, label, configEditInput.View())
			} else {
				fieldLine = fmt.Sprintf("%s%s: %s", indicator, label, value)
			}

			fieldStyle := lipgloss.NewStyle().Width(modalWidth)
			if isSelected && !isEditing {
				fieldStyle = fieldStyle.Foreground(successColor).Bold(true)
			}

			messageLines = append(messageLines, fieldStyle.Render(fieldLine))

			if field.Description != "" && isSelected {
				descStyle := lipgloss.NewStyle().
					Foreground(dimColor).
					Italic(true).
					Width(modalWidth)
				messageLines = append(messageLines, descStyle.Render("  "+field.Description))
			}
		}

		messageLines = append(messageLines, messageStyle.Render(""))
	}

	// Command Field Section (for manual/docker/binary plugins only)
	commandFieldIdx := numSchemaFields
	if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
		messageLines = append(messageLines, messageStyle.Render(""))

		commandHeaderStyle := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Width(modalWidth)
		messageLines = append(messageLines, commandHeaderStyle.Render("Startup Command:"))
		messageLines = append(messageLines, messageStyle.Render(""))

		// Command field
		isSelected := selectedConfigIdx == commandFieldIdx && !configEditMode && !addingCustomEnv
		isEditing := configEditMode && selectedConfigIdx == commandFieldIdx

		indicator := "  "
		if isSelected && !isEditing {
			indicator = "▶ "
		}

		commandValue := configFields["__command__"]
		var line string
		if isEditing {
			line = fmt.Sprintf("%sCommand:  %s", indicator, configEditInput.View())
		} else {
			if commandValue == "" {
				commandValue = "(not configured - plugin may not start)"
			}
			line = fmt.Sprintf("%sCommand:  %s", indicator, commandValue)
		}

		lineStyle := lipgloss.NewStyle().Width(modalWidth)
		if isSelected && !isEditing {
			lineStyle = lineStyle.Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, lineStyle.Render(line))

		// Show hint when selected
		if isSelected && !isEditing {
			var hint string
			switch plugin.InstallType {
			case "docker":
				hint = "Example: docker run -it container-name"
			case "manual":
				hint = "Example: /path/to/executable --args"
			default:
				hint = "Example: ./binary-name --args"
			}

			hintStyle := lipgloss.NewStyle().
				Foreground(dimColor).
				Italic(true).
				Width(modalWidth)
			messageLines = append(messageLines, hintStyle.Render("           "+hint))
		}

		messageLines = append(messageLines, messageStyle.Render(""))
	}

	// Environment Variables Section (always shown)
	envHeaderStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Width(modalWidth)
	messageLines = append(messageLines, envHeaderStyle.Render("Environment Variables:"))
	messageLines = append(messageLines, messageStyle.Render(""))

	// Render existing env vars
	if len(customEnvVars) == 0 && !addingCustomEnv {
		noEnvStyle := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Width(modalWidth)
		messageLines = append(messageLines, noEnvStyle.Render("  No environment variables set"))
	} else {
		for i, envVar := range customEnvVars {
			// Calculate index: schema fields + command field (if manual/docker/binary) + env var index
			commandFieldOffset := 0
			if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
				commandFieldOffset = 1
			}
			envIdx := numSchemaFields + commandFieldOffset + i
			isSelected := !addingCustomEnv && envIdx == selectedConfigIdx
			isEditing := configEditMode && isSelected

			indicator := "  "
			if isSelected && !isEditing {
				indicator = "▶ "
			}

			// Mask sensitive values
			displayValue := envVar.Value
			if isSensitiveKey(envVar.Key) && !isEditing {
				displayValue = strings.Repeat("*", len(envVar.Value))
				if displayValue == "" {
					displayValue = "<not set>"
				}
			}

			var line string
			if isEditing {
				if customEnvFocusField == 0 {
					line = fmt.Sprintf("%s%s = %s", indicator, customEnvKeyInput.View(), displayValue)
				} else {
					line = fmt.Sprintf("%s%s = %s", indicator, envVar.Key, customEnvValInput.View())
				}
			} else {
				line = fmt.Sprintf("%s%s = %s", indicator, envVar.Key, displayValue)
			}

			lineStyle := lipgloss.NewStyle().Width(modalWidth)
			if isSelected && !isEditing {
				lineStyle = lineStyle.Foreground(successColor).Bold(true)
			}

			messageLines = append(messageLines, lineStyle.Render(line))
		}
	}

	// Add new env var section
	commandFieldOffset := 0
	if plugin.InstallType == "manual" || plugin.InstallType == "docker" || plugin.InstallType == "binary" {
		commandFieldOffset = 1
	}
	addEnvIdx := numSchemaFields + commandFieldOffset + len(customEnvVars)
	isAddSelected := !configEditMode && addEnvIdx == selectedConfigIdx

	var addLine string
	if addingCustomEnv {
		if customEnvFocusField == 0 {
			addLine = fmt.Sprintf("  %s = ", customEnvKeyInput.View())
		} else {
			addLine = fmt.Sprintf("  %s = %s", customEnvKeyInput.Value(), customEnvValInput.View())
		}
	} else {
		indicator := "  "
		if isAddSelected {
			indicator = "▶ "
		}
		addLine = indicator + "+ Add Environment Variable"
	}

	addStyle := lipgloss.NewStyle().Width(modalWidth)
	if isAddSelected || addingCustomEnv {
		addStyle = addStyle.Foreground(successColor).Bold(true)
	}

	messageLines = append(messageLines, addStyle.Render(addLine))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Dynamic footer based on state
	var footer string
	if addingCustomEnv {
		footer = FormatFooter("Tab", "Next", "Enter", "Add", "Esc", "Cancel")
	} else if configEditMode {
		footer = FormatFooter("Tab", "Next", "Enter", "Save", a.formatKeyDisplay("primary", "U"), "Clear", "Esc", "Cancel")
	} else {
		footer = FormatFooter("j/k", "Nav", "Enter", "Edit", "d", "Del", a.formatKeyDisplay("primary", "Enter"), "Save", "Esc", "Close")
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// isSensitiveKey checks if an environment variable key should be masked
func isSensitiveKey(key string) bool {
	upperKey := strings.ToUpper(key)
	sensitiveWords := []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "AUTH", "CREDENTIAL"}
	for _, word := range sensitiveWords {
		if strings.Contains(upperKey, word) {
			return true
		}
	}
	return false
}

// renderAddCustomModal renders the modal for adding a custom plugin
func (a *AppView) renderAddCustomModal() string {
	modalWidth := 80
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	// Determine title based on mode
	title := "Add Custom Plugin"
	if a.pluginManagerState.addCustomModal.editMode {
		title = "Edit Custom Plugin"
	}

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(title)

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Get current install type to conditionally show Command field
	installType := a.pluginManagerState.addCustomModal.fields["install_type"].Value()

	// Build field order dynamically
	type fieldDef struct {
		key   string
		label string
		hint  string
	}

	fieldOrder := []fieldDef{}

	// Always show these fields
	fieldOrder = append(fieldOrder,
		fieldDef{"repository", "Repository", ""},
		fieldDef{"id", "ID", ""},
		fieldDef{"name", "Name", ""},
		fieldDef{"install_type", "Install Type", "(npm, pip, npx, go, manual, docker, binary, remote)"},
		fieldDef{"package", "Package", ""},
	)

	// Conditionally show ServerURL, AuthType, and Transport for remote
	switch installType {
	case "remote":
		fieldOrder = append(fieldOrder,
			fieldDef{"server_url", "Server URL", "(http://localhost:8080 or https://mcp.example.com)"},
			fieldDef{"auth_type", "Auth Type", "(none, headers, oauth)"},
			fieldDef{"transport", "Transport", "(sse, streamable-http)"},
		)
	}

	// Conditionally show Command field for manual/docker/binary
	switch installType {
	case "manual", "docker", "binary":
		hint := "(optional - command to start plugin)"
		switch installType {
		case "docker":
			hint = "(optional - e.g., 'docker run -it container-name')"
		case "manual":
			hint = "(optional - e.g., '/path/to/executable --args')"
		case "binary":
			hint = "(optional - e.g., './binary-name --args')"
		}
		fieldOrder = append(fieldOrder, fieldDef{"command", "Command", hint})
	}

	// Always show these fields
	fieldOrder = append(fieldOrder,
		fieldDef{"description", "Description", "(optional)"},
		fieldDef{"category", "Category", "(optional)"},
		fieldDef{"language", "Language", "(optional)"},
	)

	// Render each field
	for i, f := range fieldOrder {
		isSelected := i == a.pluginManagerState.addCustomModal.fieldIdx
		indicator := "  "
		if isSelected {
			indicator = "▶ "
		}

		input := a.pluginManagerState.addCustomModal.fields[f.key]

		// Show full placeholder when field is empty and not focused
		displayValue := input.View()
		if input.Value() == "" && !isSelected {
			displayValue = lipgloss.NewStyle().Foreground(dimColor).Render(input.Placeholder)
		}

		line := fmt.Sprintf("%s%-15s %s", indicator, f.label+":", displayValue)

		lineStyle := lipgloss.NewStyle().Width(modalWidth)
		if isSelected {
			lineStyle = lineStyle.Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, lineStyle.Render(line))

		// Add hint if exists
		if f.hint != "" && isSelected {
			hintStyle := lipgloss.NewStyle().
				Foreground(dimColor).
				Italic(true).
				Width(modalWidth)
			messageLines = append(messageLines, hintStyle.Render("                "+f.hint))
		}
	}

	messageLines = append(messageLines, "")

	// Configuration requirements section
	envCount := len(a.pluginManagerState.addCustomModal.envVars)
	argsCount := len(a.pluginManagerState.addCustomModal.args)

	configHeaderStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Width(modalWidth)
	messageLines = append(messageLines, configHeaderStyle.Render("  Configuration Requirements:"))
	messageLines = append(messageLines, "")

	// Calculate dynamic indices for Environment and Arguments
	installTypeForRender := a.pluginManagerState.addCustomModal.fields["install_type"].Value()
	numTextFieldsForRender := 8 // Base fields: repository, id, name, install_type, package, description, category, language
	switch installTypeForRender {
	case "remote":
		numTextFieldsForRender = 11 // Add server_url, auth_type, and transport
	case "manual", "docker", "binary":
		numTextFieldsForRender = 9 // Add command field
	}
	envIndexForRender := numTextFieldsForRender
	argsIndexForRender := numTextFieldsForRender + 1

	// Environment Variables item (dynamic index)
	envSelected := a.pluginManagerState.addCustomModal.fieldIdx == envIndexForRender
	envIndicator := "  "
	if envSelected {
		envIndicator = "▶ "
	}
	envLine := fmt.Sprintf("%sEnvironment Variables: (%d defined)  [Enter to configure]", envIndicator, envCount)
	envStyle := lipgloss.NewStyle().Width(modalWidth)
	if envSelected {
		envStyle = envStyle.Foreground(successColor).Bold(true)
	} else {
		envStyle = envStyle.Foreground(dimColor)
	}
	messageLines = append(messageLines, envStyle.Render(envLine))

	// Arguments item (dynamic index)
	argsSelected := a.pluginManagerState.addCustomModal.fieldIdx == argsIndexForRender
	argsIndicator := "  "
	if argsSelected {
		argsIndicator = "▶ "
	}
	argsLine := fmt.Sprintf("%sArguments:             (%d defined)  [Enter to configure]", argsIndicator, argsCount)
	argsStyle := lipgloss.NewStyle().Width(modalWidth)
	if argsSelected {
		argsStyle = argsStyle.Foreground(successColor).Bold(true)
	} else {
		argsStyle = argsStyle.Foreground(dimColor)
	}
	messageLines = append(messageLines, argsStyle.Render(argsLine))

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Determine footer action based on mode
	footerAction := "Add"
	if a.pluginManagerState.addCustomModal.editMode {
		footerAction = "Save"
	}
	footer := FormatFooter("Tab/Shift+Tab", "Navigate", "Enter", "Edit", a.formatKeyDisplay("primary", "Enter"), footerAction, "Esc", "Cancel")

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderEnvEditModal renders the environment variable editing modal
func (a *AppView) renderEnvEditModal() string {
	modalWidth := 70
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Configure Environment Variables")

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Render existing env vars
	numEnvVars := len(a.pluginManagerState.addCustomModal.envVars)
	if numEnvVars == 0 && !a.pluginManagerState.configModal.addingCustomEnv {
		noEnvStyle := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Width(modalWidth)
		messageLines = append(messageLines, noEnvStyle.Render("  No environment variables defined"))
	} else {
		for i, envVar := range a.pluginManagerState.addCustomModal.envVars {
			isSelected := !a.pluginManagerState.configModal.addingCustomEnv && i == a.pluginManagerState.addCustomModal.envIdx
			isEditing := a.pluginManagerState.addCustomModal.envEditMode && isSelected

			indicator := "  "
			if isSelected && !isEditing {
				indicator = "▶ "
			}

			var line string
			if isEditing {
				if a.pluginManagerState.configModal.customEnvFocusField == 0 {
					line = fmt.Sprintf("%s%s", indicator, a.pluginManagerState.configModal.customEnvKeyInput.View())
				} else {
					line = fmt.Sprintf("%s%s", indicator, envVar.Key)
				}
			} else {
				line = fmt.Sprintf("%s%s", indicator, envVar.Key)
			}

			lineStyle := lipgloss.NewStyle().Width(modalWidth)
			if isSelected && !isEditing {
				lineStyle = lineStyle.Foreground(successColor).Bold(true)
			}

			messageLines = append(messageLines, lineStyle.Render(line))
		}
	}

	// Add new env var section
	addEnvIdx := numEnvVars
	isAddSelected := !a.pluginManagerState.addCustomModal.envEditMode && addEnvIdx == a.pluginManagerState.addCustomModal.envIdx

	var addLine string
	if a.pluginManagerState.configModal.addingCustomEnv {
		addLine = fmt.Sprintf("  %s", a.pluginManagerState.configModal.customEnvKeyInput.View())
	} else {
		indicator := "  "
		if isAddSelected {
			indicator = "▶ "
		}
		addLine = indicator + "+ Add Environment Variable"
	}

	addStyle := lipgloss.NewStyle().Width(modalWidth)
	if isAddSelected || a.pluginManagerState.configModal.addingCustomEnv {
		addStyle = addStyle.Foreground(successColor).Bold(true)
	}

	messageLines = append(messageLines, addStyle.Render(addLine))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	var footer string
	if a.pluginManagerState.configModal.addingCustomEnv {
		footer = FormatFooter("Enter", "Add", "Esc", "Cancel")
	} else if a.pluginManagerState.addCustomModal.envEditMode {
		footer = FormatFooter("Enter", "Save", a.formatKeyDisplay("primary", "U"), "Clear", "Esc", "Cancel")
	} else {
		footer = FormatFooter("j/k", "Nav", "Enter", "Edit", "d", "Del", "Esc", "Back")
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderArgsEditModal renders the arguments editing modal
func (a *AppView) renderArgsEditModal() string {
	modalWidth := 70
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Configure Arguments")

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Render existing args
	numArgs := len(a.pluginManagerState.addCustomModal.args)
	if numArgs == 0 && !a.pluginManagerState.addCustomModal.argsEditMode {
		noArgsStyle := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Width(modalWidth)
		messageLines = append(messageLines, noArgsStyle.Render("  No arguments defined"))
	} else {
		for i, arg := range a.pluginManagerState.addCustomModal.args {
			isSelected := !a.pluginManagerState.addCustomModal.argsEditMode && i == a.pluginManagerState.addCustomModal.argsIdx

			indicator := "  "
			if isSelected {
				indicator = "▶ "
			}

			var valueStr string
			switch arg.Type {
			case "none":
				valueStr = "(no value)"
			case "fixed":
				valueStr = fmt.Sprintf("= %s", arg.Value)
			case "value":
				valueStr = fmt.Sprintf("= value (%s)", arg.Label)
			}

			line := fmt.Sprintf("%s%s  %s", indicator, arg.Flag, valueStr)

			lineStyle := lipgloss.NewStyle().Width(modalWidth)
			if isSelected {
				lineStyle = lineStyle.Foreground(successColor).Bold(true)
			}

			messageLines = append(messageLines, lineStyle.Render(line))
		}
	}

	// Add new arg section
	addArgsIdx := numArgs
	isAddSelected := !a.pluginManagerState.addCustomModal.argsEditMode && addArgsIdx == a.pluginManagerState.addCustomModal.argsIdx

	var addLine string
	if a.pluginManagerState.addCustomModal.argsEditMode {
		addLine = "  [Adding new argument...]"
	} else {
		indicator := "  "
		if isAddSelected {
			indicator = "▶ "
		}
		addLine = indicator + "+ Add Argument"
	}

	addStyle := lipgloss.NewStyle().Width(modalWidth)
	if isAddSelected || a.pluginManagerState.addCustomModal.argsEditMode {
		addStyle = addStyle.Foreground(successColor).Bold(true)
	}

	messageLines = append(messageLines, addStyle.Render(addLine))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	var footer string
	if a.pluginManagerState.addCustomModal.argsEditMode {
		footer = FormatFooter(a.formatKeyDisplay("primary", "Enter"), "Add", "Esc", "Cancel")
	} else {
		footer = FormatFooter("j/k", "Nav", "Enter", "Add/Edit", "d", "Del", "Esc", "Back")
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderArgEditForm renders the argument edit form modal
func (a *AppView) renderArgEditForm() string {
	modalWidth := 70
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render("Add Argument")

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	// Field 0: Flag
	flagIndicator := "  "
	if a.pluginManagerState.addCustomModal.argFocusField == 0 {
		flagIndicator = "▶ "
	}
	flagLine := fmt.Sprintf("%sFlag:  %s", flagIndicator, a.pluginManagerState.addCustomModal.argFlagInput.View())
	flagStyle := messageStyle
	if a.pluginManagerState.addCustomModal.argFocusField == 0 {
		flagStyle = lipgloss.NewStyle().Width(modalWidth).Foreground(successColor).Bold(true)
	}
	messageLines = append(messageLines, flagStyle.Render(flagLine))
	messageLines = append(messageLines, messageStyle.Render(""))

	// Field 1: Type selector
	typeIndicator := "  "
	if a.pluginManagerState.addCustomModal.argFocusField == 1 {
		typeIndicator = "▶ "
	}
	messageLines = append(messageLines, messageStyle.Render(typeIndicator+"Type:"))

	types := []struct {
		value string
		label string
	}{
		{"none", "none"},
		{"value", "value (user provides at configure time)"},
		{"fixed", "fixed (fixed value)"},
	}

	for i, t := range types {
		isHighlighted := a.pluginManagerState.addCustomModal.argFocusField == 1 && a.pluginManagerState.addCustomModal.argTypeHighlight == i

		// Determine if selected (comparing with current type state)
		// We need to track selected type separately - for now, highlight = selected
		selectedMarker := "○"
		if isHighlighted {
			selectedMarker = "●"
		}

		typeLineStr := fmt.Sprintf("         %s %s", selectedMarker, t.label)
		typeLineStyle := messageStyle
		if isHighlighted {
			typeLineStyle = lipgloss.NewStyle().Width(modalWidth).Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, typeLineStyle.Render(typeLineStr))
	}

	messageLines = append(messageLines, messageStyle.Render(""))

	// Field 2: Value or Label (depending on type)
	selectedType := types[a.pluginManagerState.addCustomModal.argTypeHighlight].value
	switch selectedType {
	case "fixed":
		valueIndicator := "  "
		if a.pluginManagerState.addCustomModal.argFocusField == 2 {
			valueIndicator = "▶ "
		}
		valueLine := fmt.Sprintf("%sValue: %s", valueIndicator, a.pluginManagerState.addCustomModal.argValueInput.View())
		valueStyle := messageStyle
		if a.pluginManagerState.addCustomModal.argFocusField == 2 {
			valueStyle = lipgloss.NewStyle().Width(modalWidth).Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, valueStyle.Render(valueLine))
	case "value":
		labelIndicator := "  "
		if a.pluginManagerState.addCustomModal.argFocusField == 2 {
			labelIndicator = "▶ "
		}
		labelLine := fmt.Sprintf("%sLabel: %s", labelIndicator, a.pluginManagerState.addCustomModal.argLabelInput.View())
		labelStyle := messageStyle
		if a.pluginManagerState.addCustomModal.argFocusField == 2 {
			labelStyle = lipgloss.NewStyle().Width(modalWidth).Foreground(successColor).Bold(true)
		}
		messageLines = append(messageLines, labelStyle.Render(labelLine))
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	var footer string
	if a.pluginManagerState.addCustomModal.argFocusField == 1 {
		footer = FormatFooter("Tab", "Next Field", "j/k", "Cycle", "Space/Enter", "Select", a.formatKeyDisplay("primary", "Enter"), "Add", "Esc", "Cancel")
	} else {
		footer = FormatFooter("Tab", "Next Field", a.formatKeyDisplay("primary", "U"), "Clear", a.formatKeyDisplay("primary", "Enter"), "Add", "Esc", "Cancel")
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderSecurityWarning renders a security warning modal
func (a *AppView) renderSecurityWarning() string {
	if a.pluginManagerState.warnings.securityPlugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.warnings.securityPlugin

	// Build message lines
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	var messageLines []string

	// Security warning text
	warnings := []string{
		"You are about to install third-party code that will",
		"execute on your system.",
		"",
		"Plugins can:",
		"• Access the internet",
		"• Read and write files",
		"• Execute system commands",
		"• Access environment variables",
		"",
		"Only install plugins from sources you trust.",
		"",
	}

	for _, msg := range warnings {
		messageLines = append(messageLines, messageStyle.Render(msg))
	}

	// Plugin information
	pluginInfo := []string{
		fmt.Sprintf("Plugin: %s", plugin.Name),
		fmt.Sprintf("Author: %s", plugin.Author),
		fmt.Sprintf("Repository: %s", plugin.Repository),
	}

	for _, info := range pluginInfo {
		wrapped := wrapText(info, modalWidth-4)
		for _, wrappedLine := range wrapped {
			messageLines = append(messageLines, messageStyle.Render(wrappedLine))
		}
	}

	messageLines = append(messageLines, messageStyle.Render(""))
	messageLines = append(messageLines, messageStyle.Render("Have you reviewed this plugin's source code?"))

	footer := FormatFooter("y", "Install", "n", "Cancel")

	return RenderThreeSectionModal("⚠️  Security Warning  ⚠️", messageLines, footer, ModalTypeWarning, 60, a.width, a.height)
}

// renderRuntimeMissing renders a warning modal for missing runtimes
func (a *AppView) renderRuntimeMissing() string {
	if a.pluginManagerState.warnings.runtimePlugin == nil {
		return ""
	}

	var titleText, runtimeName, websiteURL string

	switch a.pluginManagerState.warnings.runtimeType {
	case "node":
		titleText = "⚠️  Node.js Required"
		runtimeName = "Node.js"
		websiteURL = "https://nodejs.org"
	case "npm":
		titleText = "⚠️  npm Required"
		runtimeName = "npm"
		websiteURL = "https://nodejs.org"
	case "python":
		titleText = "⚠️  Python Required"
		runtimeName = "Python"
		websiteURL = "https://www.python.org"
	case "go":
		titleText = "⚠️  Go Required"
		runtimeName = "Go"
		websiteURL = "https://go.dev"
	default:
		return ""
	}

	// Build message lines
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	var messages []string
	if a.pluginManagerState.warnings.runtimeType == "npm" {
		messages = []string{
			"This plugin requires npm to install.",
			"",
			"npm was not detected on your system.",
			"",
			"npm is included with Node.js.",
			"",
			fmt.Sprintf("For more information, visit: %s", websiteURL),
		}
	} else {
		messages = []string{
			fmt.Sprintf("This plugin requires %s to run.", runtimeName),
			"",
			fmt.Sprintf("%s was not detected on your system.", runtimeName),
			"",
			fmt.Sprintf("Please install %s first.", runtimeName),
			"",
			fmt.Sprintf("For more information, visit: %s", websiteURL),
		}
	}

	var messageLines []string
	for _, msg := range messages {
		messageLines = append(messageLines, messageStyle.Render(msg))
	}

	footer := "Press any key to close"

	return RenderThreeSectionModal(titleText, messageLines, footer, ModalTypeWarning, 60, a.width, a.height)
}

// renderApiKeyWarning renders a warning modal for plugins requiring API keys
func (a *AppView) renderApiKeyWarning() string {
	if a.pluginManagerState.warnings.apiKeyPlugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.warnings.apiKeyPlugin

	// Build message lines
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	messages := []string{
		fmt.Sprintf("%s requires an API key to function.", plugin.Name),
		"",
		"After installation, configure the API key in:",
		"Plugin Manager → Select Plugin → Press 'c'",
		"",
		"Continue with installation?",
	}

	var messageLines []string
	for _, msg := range messages {
		messageLines = append(messageLines, messageStyle.Render(msg))
	}

	footer := FormatFooter("y", "Continue", "n", "Cancel")

	return RenderThreeSectionModal("⚠️  API Key Required", messageLines, footer, ModalTypeWarning, 60, a.width, a.height)
}

// renderManualInstructions renders manual installation instructions modal
func (a *AppView) renderManualInstructions() string {
	if a.pluginManagerState.warnings.manualPlugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.warnings.manualPlugin

	// Build message lines
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	pluginDir := fmt.Sprintf("~/.local/share/otui/plugins/%s/bin/", plugin.ID)

	var messageLines []string
	messageLines = append(messageLines, messageStyle.Render("This plugin requires manual installation."))
	messageLines = append(messageLines, messageStyle.Render(""))
	messageLines = append(messageLines, messageStyle.Render("Steps:"))

	// Step 1 with wrapping
	step1 := fmt.Sprintf("1. Download from: %s", plugin.Repository)
	wrapped1 := wrapText(step1, modalWidth-4)
	for i, wrappedLine := range wrapped1 {
		if i > 0 {
			wrappedLine = "   " + wrappedLine
		}
		messageLines = append(messageLines, messageStyle.Render(wrappedLine))
	}

	// Step 2
	messageLines = append(messageLines, messageStyle.Render("2. Follow installation instructions in README"))

	// Step 3 with wrapping
	step3 := fmt.Sprintf("3. Place binary in: %s", pluginDir)
	wrapped3 := wrapText(step3, modalWidth-4)
	for i, wrappedLine := range wrapped3 {
		if i > 0 {
			wrappedLine = "   " + wrappedLine
		}
		messageLines = append(messageLines, messageStyle.Render(wrappedLine))
	}

	messageLines = append(messageLines, messageStyle.Render(""))
	messageLines = append(messageLines, messageStyle.Render("Have you completed the installation?"))

	footer := FormatFooter("y", "Mark as Installed", "n", "Cancel & Remove")

	return RenderThreeSectionModal("Manual Installation Required", messageLines, footer, ModalTypeInfo, 60, a.width, a.height)
}

// renderUninstallModal renders the plugin uninstall progress modal
func (a *AppView) renderUninstallModal() string {
	if a.pluginManagerState.uninstallModal.plugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.uninstallModal.plugin
	modalWidth := 60
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	// Title section
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(fmt.Sprintf("Uninstalling: %s", plugin.Name))

	// Build message section based on state
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	if a.pluginManagerState.uninstallModal.error != "" {
		// Error state
		errorText := "Error: " + a.pluginManagerState.uninstallModal.error
		lines := strings.Split(errorText, "\n")
		errorStyle := lipgloss.NewStyle().
			Foreground(dangerColor).
			Width(modalWidth)

		for _, line := range lines {
			wrapped := wrapText(line, modalWidth-4)
			for _, wrappedLine := range wrapped {
				messageLines = append(messageLines, errorStyle.Render(wrappedLine))
			}
		}
	} else {
		// Progress state
		progress := a.pluginManagerState.uninstallModal.progress

		// Show spinner + message during progress (not on complete)
		var progressMessage string
		if progress.Stage == "complete" {
			progressMessage = progress.Message
		} else {
			spinnerView := a.pluginManagerState.uninstallModal.spinner.View()
			progressMessage = spinnerView + " " + progress.Message
		}
		messageLines = append(messageLines, messageStyle.Render(progressMessage))

		progressBar := renderProgressBar(int(progress.Percent), modalWidth-4)
		messageLines = append(messageLines, messageStyle.Render(progressBar))

		if progress.Stage == "complete" {
			messageLines = append(messageLines, messageStyle.Render(""))

			completeStyle := lipgloss.NewStyle().
				Align(lipgloss.Center).
				Width(modalWidth)
			messageLines = append(messageLines, completeStyle.Render("Uninstall complete! Press any key to close"))
		}
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Footer section - different based on state
	var footer string
	if a.pluginManagerState.uninstallModal.error != "" {
		footer = "Press any key to close"
	} else if a.pluginManagerState.uninstallModal.progress.Stage == "complete" {
		footer = "" // Message already shown in content
	} else {
		footer = "Uninstalling... please wait"
	}

	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// renderTrustWarningModal renders a trust warning for custom plugins
func (a *AppView) renderTrustWarningModal() string {
	if a.pluginManagerState.warnings.trustPlugin == nil {
		return ""
	}

	plugin := a.pluginManagerState.warnings.trustPlugin

	// Build message lines
	modalWidth := 70
	if a.width < modalWidth+10 {
		modalWidth = a.width - 10
	}

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	var messageLines []string

	// Plugin info
	pluginInfo := []string{
		fmt.Sprintf("Plugin: %s", plugin.Name),
		fmt.Sprintf("Repository: %s", plugin.Repository),
		fmt.Sprintf("Author: %s", plugin.Author),
		fmt.Sprintf("Stars: %d", plugin.Stars),
	}

	if plugin.License != "" {
		pluginInfo = append(pluginInfo, fmt.Sprintf("License: %s", plugin.License))
	}

	pluginInfo = append(pluginInfo, "")

	for _, info := range pluginInfo {
		wrapped := wrapText(info, modalWidth-4)
		for _, wrappedLine := range wrapped {
			messageLines = append(messageLines, messageStyle.Render(wrappedLine))
		}
	}

	// Trust warnings (if any)
	if len(a.pluginManagerState.warnings.trustWarnings) > 0 {
		warningTitleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(warningColor).
			Width(modalWidth)

		messageLines = append(messageLines, warningTitleStyle.Render("Trust Warnings:"))

		warningStyle := lipgloss.NewStyle().
			Foreground(warningColor).
			Width(modalWidth)

		for _, warning := range a.pluginManagerState.warnings.trustWarnings {
			wrapped := wrapText(warning, modalWidth-4)
			for _, wrappedLine := range wrapped {
				messageLines = append(messageLines, warningStyle.Render(wrappedLine))
			}
		}
		messageLines = append(messageLines, messageStyle.Render(""))
	}

	// Question
	questionStyle := lipgloss.NewStyle().
		Bold(true).
		Width(modalWidth)
	messageLines = append(messageLines, questionStyle.Render("Do you want to add this plugin?"))

	footer := FormatFooter("y", "Add Plugin", "n", "Cancel")

	return RenderThreeSectionModal("⚠️  Custom Plugin Warning", messageLines, footer, ModalTypeWarning, 70, a.width, a.height)
}

// renderRegistryRefreshModal renders the registry refresh modal
func (a *AppView) renderRegistryRefreshModal() string {
	if a.pluginManagerState.registryRefresh.phase == "fetching" {
		// Show spinner while fetching
		modalWidth := 50
		if a.width < modalWidth+10 {
			modalWidth = a.width - 10
		}

		content := a.pluginManagerState.registryRefresh.spinner.View() + " Fetching plugins from registry..."

		paddedContent := lipgloss.NewStyle().
			Width(modalWidth).
			Align(lipgloss.Center).
			Render(content)

		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, paddedContent)
	}

	if a.pluginManagerState.registryRefresh.phase == "success" {
		// Show success modal
		return RenderAcknowledgeModal(
			"Registry Updated",
			"Plugin list updated successfully!",
			ModalTypeInfo,
			a.width,
			a.height,
		)
	}

	if a.pluginManagerState.registryRefresh.phase == "error" {
		// Show error modal
		return RenderAcknowledgeModal(
			"Registry Update Failed",
			"Failed to fetch plugins from registry:\n\n"+a.pluginManagerState.registryRefresh.error,
			ModalTypeError,
			a.width,
			a.height,
		)
	}

	return ""
}
