package ui

import (
	"fmt"
	"os"

	"otui/config"
	"otui/mcp"
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
