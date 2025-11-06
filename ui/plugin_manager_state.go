package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"

	"otui/mcp"
)

// SelectionState manages plugin list navigation and filtering
type SelectionState struct {
	selectedPluginIdx int
	scrollOffset      int
	filterMode        bool
	filterInput       textinput.Model
	filteredPlugins   []mcp.Plugin
	viewMode          string // "all", "installed", "available"
}

// InstallModalState manages plugin installation modal and progress
type InstallModalState struct {
	visible    bool
	plugin     *mcp.Plugin
	progress   mcp.InstallProgress
	error      string
	cancelFunc context.CancelFunc
	progressCh chan mcp.InstallProgress
	spinner    spinner.Model
}

// UninstallModalState manages plugin uninstallation modal and progress
type UninstallModalState struct {
	visible    bool
	plugin     *mcp.Plugin
	progress   mcp.InstallProgress
	error      string
	cancelFunc context.CancelFunc
	progressCh chan mcp.InstallProgress
	spinner    spinner.Model
}

// ConfigModalState manages plugin environment variable configuration
type ConfigModalState struct {
	visible        bool
	plugin         *mcp.Plugin
	fields         map[string]string
	selectedIdx    int
	editMode       bool
	editInput      textinput.Model
	preInstallMode bool // true if config modal opened from install flow

	// Custom env vars editing
	customEnvVars       []EnvVarPair
	addingCustomEnv     bool
	customEnvKeyInput   textinput.Model
	customEnvValInput   textinput.Model
	customEnvFocusField int // 0=key, 1=value
}

// DetailsModalState manages plugin details modal
type DetailsModalState struct {
	visible bool
	plugin  *mcp.Plugin
}

// WarningModalStates groups all warning/info modals
type WarningModalStates struct {
	// Security warning
	showSecurity   bool
	securityPlugin *mcp.Plugin

	// Runtime missing warning
	showRuntimeMissing bool
	runtimeType        string
	runtimePlugin      *mcp.Plugin

	// API key warning
	showApiKey   bool
	apiKeyPlugin *mcp.Plugin

	// Manual instructions
	showManual   bool
	manualPlugin *mcp.Plugin

	// Trust warning
	showTrust     bool
	trustPlugin   *mcp.Plugin
	trustWarnings []string
}

// ConfirmationModalStates groups all confirmation prompts
type ConfirmationModalStates struct {
	deletePlugin       *mcp.Plugin // Confirm delete installed plugin
	deleteCustomPlugin *mcp.Plugin // Confirm delete custom plugin
}

// AddCustomModalState manages the add custom plugin form with sub-modals
type AddCustomModalState struct {
	visible  bool
	urlInput textinput.Model

	// Form fields
	fields   map[string]textinput.Model
	fieldIdx int

	// Env vars editing
	envVars      []EnvVarPair
	envIdx       int
	envEditMode  bool
	showEnvModal bool

	// Args editing
	args             []mcp.ArgPair
	argsIdx          int
	argsEditMode     bool
	showArgsModal    bool
	argFocusField    int // 0=flag, 1=type, 2=value/label
	argTypeHighlight int // 0=none, 1=value, 2=fixed
	argFlagInput     textinput.Model
	argValueInput    textinput.Model
	argLabelInput    textinput.Model

	lastFetchedURL string // Track last URL we fetched metadata for
}

// RegistryRefreshState manages registry refresh modal
type RegistryRefreshState struct {
	visible bool
	phase   string // "fetching", "success", "error"
	error   string
	spinner spinner.Model
}
