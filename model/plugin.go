package model

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"

	"otui/config"
	"otui/mcp"
)

// PluginState holds the core plugin data and business logic state
type PluginState struct {
	Registry  *mcp.Registry
	Installer *mcp.Installer
	Config    *config.PluginsConfig
}

// PluginSystemState encapsulates plugin system startup/shutdown state
// Shared by BOTH app quit and Settings toggle (disable plugins)
type PluginSystemState struct {
	Active              bool   // Is operation in progress?
	Operation           string // "starting" or "stopping"
	Phase               string // "waiting", "unresponsive", "error"
	Spinner             spinner.Model
	UnresponsivePlugins []string
	ErrorMsg            string
	StartTime           time.Time // When operation started (for tracking)
}
