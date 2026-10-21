package model

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// StartAllPlugins starts all enabled plugins for the current session
func (m *Model) StartAllPlugins() tea.Cmd {
	if m.MCPManager == nil {
		return nil
	}
	manager := m.MCPManager

	return func() tea.Msg {
		ctx := context.Background()
		_ = manager.StartAllEnabledPlugins(ctx)

		// Check for failed plugins after startup
		failures := manager.GetFailedPlugins()
		if len(failures) > 0 {
			return PluginStartupCompleteMsg{Failures: failures}
		}
		return nil
	}
}

// EnablePlugin starts a plugin with timeout monitoring
func (m *Model) EnablePlugin(pluginID string) tea.Cmd {
	if m.MCPManager == nil {
		return nil
	}
	manager := m.MCPManager
	return func() tea.Msg {
		config.Debugf("[UI] enablePlugin: Starting plugin '%s'", pluginID)

		// Create timeout-monitored operation
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Run operation in goroutine to monitor timeout
		type result struct {
			err error
		}
		resultChan := make(chan result, 1)

		go func() {
			err := manager.StartPlugin(ctx, pluginID)
			resultChan <- result{err: err}
		}()

		// Wait for result OR timeout
		select {
		case res := <-resultChan:
			// Operation completed (success or error)
			config.Debugf("[UI] enablePlugin: Result for '%s': success=%v, err=%v", pluginID, res.err == nil, res.err)
			return PluginOperationCompleteMsg{
				Operation: "enable",
				PluginID:  pluginID,
				Success:   res.err == nil,
				Err:       res.err,
			}
		case <-ctx.Done():
			// Timeout reached - operation never completed
			timeoutErr := fmt.Errorf("plugin failed to start: operation timed out after 3 seconds (plugin may not exist, may be incompatible, or failed to respond)")
			config.Debugf("[UI] enablePlugin: TIMEOUT for plugin '%s' - returning error message", pluginID)
			return PluginOperationCompleteMsg{
				Operation: "enable",
				PluginID:  pluginID,
				Success:   false,
				Err:       timeoutErr,
			}
		}
	}
}

// DisablePlugin stops a plugin with timeout monitoring
func (m *Model) DisablePlugin(pluginID string) tea.Cmd {
	if m.MCPManager == nil {
		return nil
	}
	manager := m.MCPManager
	return func() tea.Msg {
		config.Debugf("[UI] disablePlugin: Stopping plugin '%s'", pluginID)

		// Create timeout-monitored operation
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Run operation in goroutine to monitor timeout
		type result struct {
			err error
		}
		resultChan := make(chan result, 1)

		go func() {
			err := manager.StopPlugin(ctx, pluginID)
			resultChan <- result{err: err}
		}()

		// Wait for result OR timeout
		select {
		case res := <-resultChan:
			// Operation completed (success or error)
			config.Debugf("[UI] disablePlugin: Result for '%s': success=%v, err=%v", pluginID, res.err == nil, res.err)
			return PluginOperationCompleteMsg{
				Operation: "disable",
				PluginID:  pluginID,
				Success:   res.err == nil,
				Err:       res.err,
			}
		case <-ctx.Done():
			// Timeout reached - operation never completed
			timeoutErr := fmt.Errorf("plugin failed to stop: operation timed out after 5 seconds (plugin may be unresponsive)")
			config.Debugf("[UI] disablePlugin: TIMEOUT for plugin '%s' - returning error message", pluginID)
			return PluginOperationCompleteMsg{
				Operation: "disable",
				PluginID:  pluginID,
				Success:   false,
				Err:       timeoutErr,
			}
		}
	}
}

// RefreshRegistry fetches the latest plugin list from the registry
func (m *Model) RefreshRegistry() tea.Cmd {
	if m.Plugins.Registry == nil {
		return nil
	}
	registry := m.Plugins.Registry
	return func() tea.Msg {
		config.Debugf("[UI] refreshRegistry: Starting registry refresh")

		err := registry.Refresh()

		if err != nil {
			config.Debugf("[UI] refreshRegistry: Failed with error: %v", err)
		} else {
			config.Debugf("[UI] refreshRegistry: Completed successfully")
		}

		return RegistryRefreshCompleteMsg{
			Success: err == nil,
			Err:     err,
		}
	}
}
