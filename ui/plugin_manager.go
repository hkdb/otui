package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"otui/mcp"
	"otui/model"
)

// EnvVarPair represents a key-value environment variable
type EnvVarPair struct {
	Key   string
	Value string
}

// PluginManagerState holds all UI state for the plugin manager
type PluginManagerState struct {
	// Reference to plugin data (stored in model)
	pluginState *model.PluginState

	// Grouped state (Phase 5: State Management)
	selection       SelectionState
	installModal    InstallModalState
	uninstallModal  UninstallModalState
	configModal     ConfigModalState
	detailsModal    DetailsModalState
	warnings        WarningModalStates
	confirmations   ConfirmationModalStates
	addCustomModal  AddCustomModalState
	registryRefresh RegistryRefreshState
}

// initPluginManager initializes the plugin manager state
func (a *AppView) initPluginManager() {
	if a.pluginManagerState.selection.filterInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "Filter plugins..."
		ti.CharLimit = 50
		a.pluginManagerState.selection.filterInput = ti
	}

	if a.pluginManagerState.configModal.editInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "Enter value..."
		ti.CharLimit = 200
		a.pluginManagerState.configModal.editInput = ti
	}

	if a.pluginManagerState.addCustomModal.urlInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "https://github.com/user/repo"
		ti.CharLimit = 200
		a.pluginManagerState.addCustomModal.urlInput = ti
	}

	if a.pluginManagerState.configModal.customEnvKeyInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "VARIABLE_NAME"
		ti.CharLimit = 100
		a.pluginManagerState.configModal.customEnvKeyInput = ti
	}

	if a.pluginManagerState.configModal.customEnvValInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "value"
		ti.CharLimit = 500
		a.pluginManagerState.configModal.customEnvValInput = ti
	}

	// Initialize custom plugin form inputs
	if a.pluginManagerState.addCustomModal.fields == nil {
		a.pluginManagerState.addCustomModal.fields = make(map[string]textinput.Model)

		fields := []struct {
			key         string
			placeholder string
			charLimit   int
		}{
			{"repository", "https://github.com/user/repo", 200},
			{"id", "plugin-id", 100},
			{"name", "Plugin Name", 100},
			{"install_type", "docker", 50},
			{"package", "@org/package-name or container-name", 150},
			{"server_url", "http://localhost:8080", 200},
			{"auth_type", "none", 50},
			{"transport", "sse", 50},
			{"command", "docker run -it container-name", 300},
			{"description", "Brief description", 300},
			{"category", "utility", 50},
			{"language", "TypeScript", 50},
		}

		for _, f := range fields {
			ti := textinput.New()
			ti.Placeholder = f.placeholder
			ti.CharLimit = f.charLimit
			a.pluginManagerState.addCustomModal.fields[f.key] = ti
		}
	}

	if a.pluginManagerState.addCustomModal.argFlagInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "--flag or -f"
		ti.CharLimit = 100
		a.pluginManagerState.addCustomModal.argFlagInput = ti
	}

	if a.pluginManagerState.addCustomModal.argValueInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "value"
		ti.CharLimit = 200
		a.pluginManagerState.addCustomModal.argValueInput = ti
	}

	if a.pluginManagerState.addCustomModal.argLabelInput.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = "User-friendly label"
		ti.CharLimit = 100
		a.pluginManagerState.addCustomModal.argLabelInput = ti
	}

	if a.pluginManagerState.selection.viewMode == "" {
		a.pluginManagerState.selection.viewMode = "installed"
	}
}

// getVisiblePlugins returns the list of plugins based on current view and filter
func (a *AppView) getVisiblePlugins() []mcp.Plugin {
	if a.pluginManagerState.selection.filterMode && a.pluginManagerState.selection.filterInput.Value() != "" {
		return a.pluginManagerState.selection.filteredPlugins
	}

	allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()

	switch a.pluginManagerState.selection.viewMode {
	case "installed":
		var installed []mcp.Plugin
		for _, p := range allPlugins {
			if a.pluginManagerState.pluginState.Installer.IsInstalled(p.ID) {
				installed = append(installed, p)
			}
		}
		return installed
	case "curated":
		var curated []mcp.Plugin
		for _, p := range allPlugins {
			if p.Verified {
				curated = append(curated, p)
			}
		}
		return curated
	case "official":
		var official []mcp.Plugin
		for _, p := range allPlugins {
			if p.Official {
				official = append(official, p)
			}
		}
		return official
	case "automatic":
		var automatic []mcp.Plugin
		for _, p := range allPlugins {
			if p.InstallType == "npm" || p.InstallType == "pip" || p.InstallType == "go" {
				automatic = append(automatic, p)
			}
		}
		return automatic
	case "manual":
		var manual []mcp.Plugin
		for _, p := range allPlugins {
			// Show all plugins that aren't automatically installable (npm/pip/go)
			if p.InstallType != "npm" && p.InstallType != "pip" && p.InstallType != "go" {
				manual = append(manual, p)
			}
		}
		return manual
	case "custom":
		var custom []mcp.Plugin
		for _, p := range allPlugins {
			if p.Custom {
				custom = append(custom, p)
			}
		}
		return custom
	default:
		return allPlugins
	}
}

// renderPluginManager is the main orchestrator for plugin manager rendering
func (a *AppView) renderPluginManager() string {
	if !a.showPluginManager {
		return ""
	}

	if a.pluginManagerState.warnings.showSecurity {
		return a.renderSecurityWarning()
	}

	if a.pluginManagerState.warnings.showRuntimeMissing {
		return a.renderRuntimeMissing()
	}

	if a.pluginManagerState.warnings.showApiKey {
		return a.renderApiKeyWarning()
	}

	if a.pluginManagerState.warnings.showManual {
		return a.renderManualInstructions()
	}

	if a.pluginManagerState.registryRefresh.visible {
		return a.renderRegistryRefreshModal()
	}

	if a.pluginManagerState.confirmations.deletePlugin != nil {
		return RenderConfirmationModal(ConfirmationState{
			Active:  true,
			Title:   "Uninstall Plugin",
			Message: fmt.Sprintf("Are you sure you want to uninstall:\n\n%s\n\nThis will remove all plugin files.", a.pluginManagerState.confirmations.deletePlugin.Name),
		}, a.width, a.height)
	}

	if a.pluginManagerState.installModal.visible {
		return a.renderInstallModal()
	}

	if a.pluginManagerState.uninstallModal.visible {
		return a.renderUninstallModal()
	}

	if a.pluginManagerState.configModal.visible {
		return renderConfigModal(
			a.pluginManagerState.configModal.plugin,
			a.pluginManagerState.configModal.fields,
			a.pluginManagerState.configModal.customEnvVars,
			a.pluginManagerState.configModal.selectedIdx,
			a.pluginManagerState.configModal.editMode,
			a.pluginManagerState.configModal.editInput,
			a.pluginManagerState.configModal.addingCustomEnv,
			a.pluginManagerState.configModal.customEnvKeyInput,
			a.pluginManagerState.configModal.customEnvValInput,
			a.pluginManagerState.configModal.customEnvFocusField,
			a.width,
			a.height,
		)
	}

	if a.pluginManagerState.detailsModal.visible {
		return a.renderPluginDetails()
	}

	if a.pluginManagerState.addCustomModal.visible {
		// Check for sub-modals first
		if a.pluginManagerState.addCustomModal.showEnvModal {
			return a.renderEnvEditModal()
		}
		if a.pluginManagerState.addCustomModal.showArgsModal {
			if a.pluginManagerState.addCustomModal.argsEditMode {
				return a.renderArgEditForm()
			}
			return a.renderArgsEditModal()
		}
		return a.renderAddCustomModal()
	}

	if a.pluginManagerState.warnings.showTrust {
		return a.renderTrustWarningModal()
	}

	if a.pluginManagerState.confirmations.deleteCustomPlugin != nil {
		return RenderConfirmationModal(ConfirmationState{
			Active:  true,
			Title:   "Delete Custom Plugin",
			Message: fmt.Sprintf("Are you sure you want to delete:\n\n%s\n\nThis will remove it from your custom plugins list.", a.pluginManagerState.confirmations.deleteCustomPlugin.Name),
		}, a.width, a.height)
	}

	plugins := a.getVisiblePlugins()

	maxWidth := a.width - 4
	if maxWidth > 120 {
		maxWidth = 120
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 2, 1, 2).
		Render("Plugin Manager")

	tabs := a.renderPluginTabs()

	header := lipgloss.NewStyle().
		Width(maxWidth).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(title + "\n" + tabs)

	var filterBar string
	if a.pluginManagerState.selection.filterMode {
		filterBar = lipgloss.NewStyle().
			Foreground(accentColor).
			Padding(0, 2).
			Render("Filter: " + a.pluginManagerState.selection.filterInput.View())
	} else {
		filterBar = lipgloss.NewStyle().
			Foreground(dimColor).
			Padding(0, 2).
			Render("Press / to filter, alt+r to refresh registry")
	}

	footer := a.renderPluginFooter()

	// Calculate fixed heights
	headerHeight := lipgloss.Height(header)
	filterBarHeight := lipgloss.Height(filterBar)
	footerHeight := lipgloss.Height(footer)

	// Calculate available height for plugin list
	// Reserve space for: header + filterBar + 1 blank line + footer
	listHeight := a.height - headerHeight - filterBarHeight - footerHeight - 3
	if listHeight < 10 {
		listHeight = 10
	}

	pluginList := a.renderPluginList(plugins, listHeight, maxWidth)

	// Place plugin list in fixed-height container to ensure footer stays at bottom
	pluginListContainer := lipgloss.Place(
		maxWidth,
		listHeight,
		lipgloss.Left,
		lipgloss.Top,
		pluginList,
	)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		filterBar,
		"",
		pluginListContainer,
		footer,
	)

	return lipgloss.NewStyle().
		Width(a.width).
		Height(a.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(content)
}
