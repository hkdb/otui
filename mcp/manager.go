package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"otui/config"
	"otui/storage"
)

type MCPManager struct {
	mu             sync.RWMutex
	config         *config.Config
	pluginStorage  *storage.PluginStorage
	pluginsConfig  *config.PluginsConfig
	registry       *Registry
	client         *Client
	currentSession *storage.Session
	activePlugins  map[string]bool
	failedPlugins  map[string]error // Tracks plugins that failed to start (zombies, errors)
	dataDir        string
}

func NewMCPManager(cfg *config.Config, pluginStorage *storage.PluginStorage, pluginsConfig *config.PluginsConfig, registry *Registry, dataDir string) *MCPManager {
	return &MCPManager{
		config:        cfg,
		pluginStorage: pluginStorage,
		pluginsConfig: pluginsConfig,
		registry:      registry,
		client:        NewClient(registry, dataDir, cfg),
		activePlugins: make(map[string]bool),
		failedPlugins: make(map[string]error),
		dataDir:       dataDir,
	}
}

func (m *MCPManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.PluginsEnabled
}

func (m *MCPManager) SetSession(session *storage.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simply update the current session reference
	// Plugins remain running; GetTools() filters by session.EnabledPlugins
	m.currentSession = session

	return nil
}

// detectNpmBinary reads the package.json from an installed NPM package
// and extracts the binary name from the "bin" field.
// Returns empty string if detection fails.
// Handles both string and object formats:
//   - Object: { "bin": { "mcp-server-filesystem": "dist/index.js" } }
//   - String: { "bin": "dist/index.js" } (returns empty, falls back to package name)
func detectNpmBinary(installPath, packageName string) string {
	// Construct path to package.json
	// For @scope/package: node_modules/@scope/package/package.json
	// For package: node_modules/package/package.json
	packageJsonPath := filepath.Join(installPath, "node_modules", packageName, "package.json")

	// Read package.json
	data, err := os.ReadFile(packageJsonPath)
	if err != nil {
		return "" // File doesn't exist or can't read
	}

	// Parse JSON
	var packageJson struct {
		Bin any `json:"bin"`
	}
	if err := json.Unmarshal(data, &packageJson); err != nil {
		return "" // Invalid JSON
	}

	// Handle different bin field formats
	switch v := packageJson.Bin.(type) {
	case string:
		// Simple string: "bin": "dist/index.js"
		// Binary name defaults to package name (handled by fallback)
		return ""
	case map[string]any:
		// Object: "bin": { "mcp-server-filesystem": "dist/index.js" }
		// Return the first binary name
		for binName := range v {
			return binName
		}
	}

	return ""
}

func (m *MCPManager) buildPluginCommand(plugin *Plugin, installed *storage.InstalledPlugin, pluginConfig map[string]string) (string, []string) {
	switch plugin.InstallType {
	case "remote":
		return "", nil // No command for remote plugins
	case "npm":
		// Use locally installed binary from node_modules/.bin
		var binaryPath string

		// Priority 1: Check for explicit Command override (from registry or custom plugins)
		if plugin.Command != "" {
			// Command might be just binary name or full path
			if filepath.IsAbs(plugin.Command) {
				binaryPath = plugin.Command
			} else {
				// Relative to .bin directory
				binaryPath = filepath.Join(installed.InstallPath, "node_modules", ".bin", plugin.Command)
			}
		} else if binaryName := detectNpmBinary(installed.InstallPath, plugin.Package); binaryName != "" {
			// Priority 2: Auto-detect from package.json bin field
			binaryPath = filepath.Join(installed.InstallPath, "node_modules", ".bin", binaryName)
		} else {
			// Priority 3: Fall back to extracting from package name
			// Extract the package name from the full path (e.g., "ihor-sokoliuk/mcp-searxng" -> "mcp-searxng")
			packageName := plugin.Package
			if strings.Contains(packageName, "/") {
				parts := strings.Split(packageName, "/")
				packageName = parts[len(parts)-1]
			}
			binaryPath = filepath.Join(installed.InstallPath, "node_modules", ".bin", packageName)
		}

		// Build args array from plugin.Args and user config
		args := SubstituteArgs(plugin.Args, pluginConfig)
		return binaryPath, args
	case "pip":
		venvPath := filepath.Join(installed.InstallPath, "venv", "bin", "python")
		modulePath := filepath.Join(installed.InstallPath, "venv", "bin", plugin.Package)
		// Build args array: module path + plugin args
		args := append([]string{modulePath}, SubstituteArgs(plugin.Args, pluginConfig)...)
		return venvPath, args
	case "go":
		binaryPath := filepath.Join(installed.InstallPath, plugin.Package)
		args := SubstituteArgs(plugin.Args, pluginConfig)
		return binaryPath, args
	case "binary":
		binaryPath := filepath.Join(installed.InstallPath, plugin.Package)
		args := SubstituteArgs(plugin.Args, pluginConfig)
		return binaryPath, args
	case "manual", "docker":
		// Priority 1: Check for command in config (from plugins.toml)
		if cmdValue, ok := pluginConfig["__command__"]; ok && cmdValue != "" {
			parts := strings.Fields(cmdValue)
			if len(parts) > 0 {
				return parts[0], parts[1:]
			}
		}
		// Priority 2: Check plugin's default Command field (from custom_plugins.json)
		if plugin.Command != "" {
			parts := strings.Fields(plugin.Command)
			if len(parts) > 0 {
				return parts[0], parts[1:]
			}
		}
		// Priority 3: Fallback to InstallPath/Package
		return filepath.Join(installed.InstallPath, plugin.Package), []string{}
	default:
		return "", nil
	}
}

func (m *MCPManager) GetTools(ctx context.Context) ([]mcptypes.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] AUDIT: GetTools called while plugins disabled")
		}
		return nil, nil
	}

	if m.currentSession == nil {
		return nil, nil
	}

	pluginMap := m.getEnabledPluginsWithNamesLocked()
	if len(pluginMap) == 0 {
		return nil, nil
	}

	return m.client.GetTools(ctx, pluginMap)
}

func (m *MCPManager) getEnabledPluginsWithNamesLocked() map[string]string {
	if m.currentSession == nil {
		return nil
	}

	pluginMap := make(map[string]string)
	for _, pluginID := range m.currentSession.EnabledPlugins {
		if !m.pluginStorage.IsInstalled(pluginID) {
			continue
		}

		if !m.activePlugins[pluginID] {
			continue
		}

		// Lookup plugin in registry to get short name
		shortName := pluginID // Default fallback
		plugin := m.registry.GetByID(pluginID)
		if plugin != nil {
			shortName = GetShortPluginName(plugin.Name)
		}

		pluginMap[pluginID] = shortName
	}

	return pluginMap
}

func (m *MCPManager) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcptypes.CallToolResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] SECURITY: CallTool(%s) rejected - plugins disabled", toolName)
		}
		return nil, fmt.Errorf("plugin system is disabled")
	}

	return m.client.CallTool(ctx, toolName, args)
}

func (m *MCPManager) Shutdown(ctx context.Context) error {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] Shutdown: Called - beginning shutdown process")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] Shutdown: Calling m.client.Shutdown")
		}
		if err := m.client.Shutdown(ctx); err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("[MCP] Shutdown: ERROR in m.client.Shutdown: %v", err)
			}
			return err
		}
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] Shutdown: m.client.Shutdown completed successfully")
		}
	}

	m.activePlugins = make(map[string]bool)
	m.failedPlugins = make(map[string]error)
	m.currentSession = nil

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] Shutdown: Shutdown process completed")
	}

	return nil
}

// ShutdownWithTracking attempts to shut down plugins and returns the names of plugins that didn't respond
func (m *MCPManager) ShutdownWithTracking(ctx context.Context) ([]string, error) {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] ShutdownWithTracking: ========== FUNCTION CALLED ==========")
		if deadline, ok := ctx.Deadline(); ok {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Context deadline: %v (timeout in %v)", deadline, time.Until(deadline))
		}
	}

	m.mu.Lock()

	// Get list of currently active plugin display names
	activePluginNames := []string{}
	for pluginID := range m.activePlugins {
		plugin := m.registry.GetByID(pluginID)
		if plugin != nil {
			activePluginNames = append(activePluginNames, plugin.Name)
			if config.DebugLog != nil {
				config.DebugLog.Printf("[MCP] ShutdownWithTracking: Active plugin: %s (ID: %s)", plugin.Name, pluginID)
			}
		} else {
			activePluginNames = append(activePluginNames, pluginID)
			if config.DebugLog != nil {
				config.DebugLog.Printf("[MCP] ShutdownWithTracking: Active plugin: %s (no metadata)", pluginID)
			}
		}
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] ShutdownWithTracking: Total %d active plugins before shutdown: %v", len(activePluginNames), activePluginNames)
	}

	m.mu.Unlock()

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] ShutdownWithTracking: Starting shutdown with timeout enforcement...")
	}

	// Run shutdown in goroutine with timeout enforcement
	// This prevents m.Shutdown from blocking forever on zombie/unresponsive processes
	type shutdownResult struct {
		err error
	}
	resultChan := make(chan shutdownResult, 1)

	go func() {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Goroutine started - calling m.Shutdown(ctx)")
		}
		err := m.Shutdown(ctx)
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Goroutine - m.Shutdown returned with err=%v", err)
		}
		resultChan <- shutdownResult{err: err}
	}()

	// Wait for either shutdown to complete OR context timeout
	var shutdownErr error
	select {
	case result := <-resultChan:
		// Shutdown completed before timeout
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Shutdown completed successfully, err=%v", result.err)
		}
		shutdownErr = result.err

		// Shutdown succeeded - return empty list
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: RETURNING empty list (all plugins shut down)")
		}
		return []string{}, shutdownErr

	case <-ctx.Done():
		// Context timed out - shutdown didn't complete in time
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Context timeout reached: %v", ctx.Err())
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: Shutdown goroutine still running (will be abandoned)")
			config.DebugLog.Printf("[MCP] ShutdownWithTracking: RETURNING unresponsive plugins: %v", activePluginNames)
		}

		// All active plugins are considered unresponsive since shutdown timed out
		return activePluginNames, ctx.Err()
	}
}

func (m *MCPManager) Restart(ctx context.Context) error {
	if err := m.Shutdown(ctx); err != nil {
		return err
	}

	return m.StartAllEnabledPlugins(ctx)
}

// GetActivePluginNames returns the display names of currently active plugins
func (m *MCPManager) GetActivePluginNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.PluginsEnabled {
		return nil
	}

	var names []string
	for pluginID := range m.activePlugins {
		if m.activePlugins[pluginID] {
			plugin := m.registry.GetByID(pluginID)
			if plugin != nil {
				names = append(names, plugin.Name)
			}
		}
	}

	return names
}

// StartAllEnabledPlugins starts all plugins that are enabled in Plugins Manager (Layer 2)
// This is called once on OTUI startup to initialize all available plugins
func (m *MCPManager) StartAllEnabledPlugins(ctx context.Context) error {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Starting plugin startup process")
	}

	if !m.config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Plugins disabled in config, skipping")
		}
		return nil
	}

	// Get all installed plugins
	installedPlugins, err := m.pluginStorage.List()
	if err != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: ERROR listing plugins: %v", err)
		}
		return fmt.Errorf("failed to list installed plugins: %w", err)
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Found %d installed plugins", len(installedPlugins))
	}

	// Collect plugins to start (with mutex held briefly)
	type pluginToStart struct {
		installed storage.InstalledPlugin
		plugin    *Plugin
		command   string
		args      []string
	}
	var pluginsToStart []pluginToStart

	m.mu.Lock()
	for _, installed := range installedPlugins {
		// Check if plugin is enabled in Plugins Manager (Layer 2)
		if !m.pluginsConfig.GetPluginEnabled(installed.ID) {
			continue
		}

		// Skip if already running
		if m.activePlugins[installed.ID] {
			continue
		}

		// Get plugin metadata from registry
		plugin := m.registry.GetByID(installed.ID)
		if plugin == nil {
			continue
		}

		// Build command for this plugin
		pluginConfig := m.pluginsConfig.GetPluginConfig(installed.ID)
		command, args := m.buildPluginCommand(plugin, &installed, pluginConfig)

		// Skip if no command AND not a remote plugin
		switch {
		case command == "" && plugin.InstallType != "remote":
			continue
		}

		pluginsToStart = append(pluginsToStart, pluginToStart{
			installed: installed,
			plugin:    plugin,
			command:   command,
			args:      args,
		})
	}
	m.mu.Unlock()

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Collected %d plugins to start", len(pluginsToStart))
	}

	// Start plugins WITHOUT holding the mutex (slow operation)
	for _, pts := range pluginsToStart {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Starting plugin '%s'", pts.installed.ID)
		}
		// Configure the plugin
		mcpConfig := PluginConfig{
			ID:         pts.installed.ID,
			Runtime:    pts.plugin.InstallType,
			EntryPoint: pts.command,
			Args:       pts.args,
			Env:        make(map[string]string),
			Config:     m.pluginsConfig.GetPluginConfig(pts.installed.ID),
			ServerURL:  pts.installed.ServerURL,
			AuthType:   pts.installed.AuthType,
			Transport:  pts.installed.Transport,
		}

		// Check for server_url override in user config
		userConfig := m.pluginsConfig.GetPluginConfig(pts.installed.ID)
		if serverURLOverride := userConfig["server_url"]; serverURLOverride != "" {
			mcpConfig.ServerURL = serverURLOverride
			if config.DebugLog != nil {
				config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Overriding server_url for '%s': %s → %s", pts.installed.ID, pts.installed.ServerURL, serverURLOverride)
			}
		}

		// For remote plugins, merge env vars from config into Env
		// (Phase 0 will handle loading encrypted credentials here)
		switch pts.plugin.InstallType {
		case "remote":
			// Get config which may contain auth headers
			for k, v := range m.pluginsConfig.GetPluginConfig(pts.installed.ID) {
				mcpConfig.Env[k] = v
			}
		}

		// Substitute session template variables in environment
		if m.currentSession != nil {
			mcpConfig.Env = SubstituteSessionVars(mcpConfig.Env, m.currentSession.ID, m.currentSession.Name, m.dataDir)
		}

		// Start the plugin (this can be slow, mutex is NOT held)
		if err := m.client.Start(ctx, mcpConfig); err != nil {
			// Plugin failed to start - mark as failed so it shows up in shutdown
			// This includes zombie processes, crashed plugins, etc.
			fmt.Printf("Warning: Failed to start plugin %s: %v\n", pts.installed.ID, err)
			if config.DebugLog != nil {
				config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: ERROR starting plugin '%s': %v", pts.installed.ID, err)
				config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Marking '%s' as failed (zombie/crashed)", pts.installed.ID)
			}

			// Mark as both active AND failed - so it shows up in shutdown as unresponsive
			m.mu.Lock()
			m.activePlugins[pts.installed.ID] = true
			m.failedPlugins[pts.installed.ID] = err
			m.mu.Unlock()
			continue
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Successfully started plugin '%s'", pts.installed.ID)
		}

		// Mark as active (acquire mutex just for this write)
		m.mu.Lock()
		m.activePlugins[pts.installed.ID] = true
		m.mu.Unlock()
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartAllEnabledPlugins: Completed plugin startup process")
	}
	return nil
}

// GetShortPluginName extracts the short display name from a full plugin name.
// This is used for compact display in the title bar and tool execution indicators.
// Examples:
//
//	"ihor-sokoliuk/mcp-searxng" -> "mcp-searxng"
//	"brave-labs/brave-search" -> "brave-search"
//	"simple-plugin" -> "simple-plugin"
func GetShortPluginName(fullName string) string {
	if idx := strings.LastIndex(fullName, "/"); idx != -1 {
		return fullName[idx+1:]
	}
	return fullName
}

// GetSessionEnabledPluginNames returns the display names of plugins enabled for a specific session.
// Used for displaying in the title bar. Returns short names (e.g., "mcp-searxng" instead of "ihor-sokoliuk/mcp-searxng").
func (m *MCPManager) GetSessionEnabledPluginNames(session *storage.Session) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.PluginsEnabled || session == nil {
		return nil
	}

	var names []string
	for _, pluginID := range session.EnabledPlugins {
		plugin := m.registry.GetByID(pluginID)
		if plugin != nil {
			// Use short name for compact display in title bar
			names = append(names, GetShortPluginName(plugin.Name))
		}
	}

	return names
}

// HasUnavailableSessionPlugins returns true if any plugins enabled in the session
// are not actually available (disabled in Plugin Manager or not running).
// Used to show warning indicator (⚠️) in title bar when plugins are configured but unavailable.
func (m *MCPManager) HasUnavailableSessionPlugins(session *storage.Session) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session == nil || len(session.EnabledPlugins) == 0 {
		return false // No plugins enabled, nothing to warn about
	}

	// If plugin system is disabled, all session plugins are unavailable
	if !m.config.PluginsEnabled {
		return true
	}

	for _, pluginID := range session.EnabledPlugins {
		// Check Layer 2: Plugin Manager enabled state
		if !m.pluginsConfig.GetPluginEnabled(pluginID) {
			return true // Plugin disabled in manager
		}

		// Check if plugin is actually running
		if !m.activePlugins[pluginID] {
			return true // Plugin not running
		}
	}

	return false // All enabled plugins are available
}

// GetPluginShortName returns the short display name for a plugin ID.
// This looks up the plugin in the registry and extracts the short name.
// Returns empty string if plugin not found.
// Example: "ihor-sokoliuk-mcp-searxng" -> "mcp-searxng"
func (m *MCPManager) GetPluginShortName(pluginID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin := m.registry.GetByID(pluginID)
	if plugin != nil {
		return GetShortPluginName(plugin.Name)
	}
	return ""
}

// StartPlugin starts a specific plugin (called when user enables it in Plugins Manager)
func (m *MCPManager) StartPlugin(ctx context.Context, pluginID string) error {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartPlugin: Called for plugin '%s'", pluginID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartPlugin: Plugins disabled in config")
		}
		return nil
	}

	// If plugin previously failed, clear its state to allow retry
	// Note: This leaves zombie processes until OTUI exits, but zombies are harmless
	// (they only consume a PID table entry and are reaped when parent exits)
	if m.failedPlugins[pluginID] != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' previously failed (%v), clearing state for retry", pluginID, m.failedPlugins[pluginID])
		}
		// Clear failed state to allow retry
		delete(m.activePlugins, pluginID)
		delete(m.failedPlugins, pluginID)
		// Continue with normal start logic below...
	}

	// Skip if already running (successfully, not failed)
	if m.activePlugins[pluginID] {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' already running, skipping", pluginID)
		}
		return nil
	}

	// Load installed plugin metadata
	installed, err := m.pluginStorage.Load(pluginID)
	if err != nil || installed == nil {
		return fmt.Errorf("plugin not installed: %w", err)
	}

	// Get plugin from registry
	plugin := m.registry.GetByID(pluginID)
	if plugin == nil {
		return fmt.Errorf("plugin not found in registry: %s", pluginID)
	}

	// Build command
	pluginConfig := m.pluginsConfig.GetPluginConfig(pluginID)
	command, args := m.buildPluginCommand(plugin, installed, pluginConfig)

	// Skip if no command AND not a remote plugin
	switch {
	case command == "" && plugin.InstallType != "remote":
		return fmt.Errorf("failed to build command for plugin: %s", pluginID)
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartPlugin: Built command for '%s': command='%s', args=%v", pluginID, command, args)
	}

	// Configure plugin
	mcpConfig := PluginConfig{
		ID:         pluginID,
		Runtime:    plugin.InstallType,
		EntryPoint: command,
		Args:       args,
		Env:        make(map[string]string),
		Config:     m.pluginsConfig.GetPluginConfig(pluginID),
		ServerURL:  installed.ServerURL,
		AuthType:   installed.AuthType,
		Transport:  installed.Transport,
	}

	// Check for server_url override in user config
	userConfig := m.pluginsConfig.GetPluginConfig(pluginID)
	if serverURLOverride := userConfig["server_url"]; serverURLOverride != "" {
		mcpConfig.ServerURL = serverURLOverride
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartPlugin: Overriding server_url for '%s': %s → %s", pluginID, installed.ServerURL, serverURLOverride)
		}
	}

	// For remote plugins, merge env vars from config into Env
	switch plugin.InstallType {
	case "remote":
		// Get config which may contain auth headers
		for k, v := range m.pluginsConfig.GetPluginConfig(pluginID) {
			mcpConfig.Env[k] = v
		}
	}

	// Substitute session template variables in environment
	if m.currentSession != nil {
		mcpConfig.Env = SubstituteSessionVars(mcpConfig.Env, m.currentSession.ID, m.currentSession.Name, m.dataDir)
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartPlugin: Plugin config for '%s': Runtime=%s, Config=%v", pluginID, mcpConfig.Runtime, mcpConfig.Config)
	}

	// Start the plugin
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartPlugin: Calling m.client.Start for plugin '%s'", pluginID)
	}
	if err := m.client.Start(ctx, mcpConfig); err != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StartPlugin: ERROR starting plugin '%s': %v", pluginID, err)
			config.DebugLog.Printf("[MCP] StartPlugin: Marking '%s' as failed (zombie/crashed)", pluginID)
		}
		// Mark as both active AND failed - so it shows up in shutdown as unresponsive
		m.activePlugins[pluginID] = true
		m.failedPlugins[pluginID] = err
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	// Mark as active
	m.activePlugins[pluginID] = true

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StartPlugin: Successfully started plugin '%s'", pluginID)
	}

	return nil
}

// StopPlugin stops a specific plugin (called when user disables it in Plugins Manager)
func (m *MCPManager) StopPlugin(ctx context.Context, pluginID string) error {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StopPlugin: Called for plugin '%s'", pluginID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.activePlugins[pluginID] {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StopPlugin: Plugin '%s' not running, skipping", pluginID)
		}
		return nil
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StopPlugin: Calling m.client.Stop for plugin '%s'", pluginID)
	}
	if err := m.client.Stop(ctx, pluginID); err != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] StopPlugin: ERROR stopping plugin '%s': %v", pluginID, err)
		}
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	delete(m.activePlugins, pluginID)

	if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] StopPlugin: Successfully stopped plugin '%s'", pluginID)
	}

	return nil
}

// ExecuteTool executes a tool by name with the given arguments
// Tool names should be in the format "pluginID.toolName" (namespaced)
func (m *MCPManager) ExecuteTool(ctx context.Context, toolName string, args map[string]any) (*mcptypes.CallToolResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.PluginsEnabled {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[MCP] SECURITY: ExecuteTool(%s) rejected - plugins disabled", toolName)
		}
		return nil, fmt.Errorf("plugins are disabled")
	}

	return m.client.CallTool(ctx, toolName, args)
}

// GetFailedPlugins returns a copy of the failed plugins map
func (m *MCPManager) GetFailedPlugins() map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	failures := make(map[string]error, len(m.failedPlugins))
	for k, v := range m.failedPlugins {
		failures[k] = v
	}
	return failures
}
