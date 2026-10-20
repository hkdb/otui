package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	globalconfig "otui/config"
)

type ProcessManager struct {
	processes  map[string]*PluginProcess
	starting   map[string]bool      // Plugins whose StartPlugin is in progress
	generation int                  // Incremented by Shutdown - starts in progress are discarded
	dataDir    string               // For FileTokenStore
	config     *globalconfig.Config // For security settings
	mu         sync.RWMutex
}

// pluginCmds tracks running local plugin processes (across all
// ProcessManagers) so they can be stopped if OTUI is killed by SIGHUP
var pluginCmds sync.Map // *exec.Cmd -> struct{}

// TerminatePluginProcesses stops every local plugin process group without
// closing clients. For signal handlers - use Shutdown otherwise.
func TerminatePluginProcesses() {
	var wg sync.WaitGroup
	pluginCmds.Range(func(key, _ any) bool {
		wg.Add(1)
		go func(cmd *exec.Cmd) {
			defer wg.Done()
			_ = terminateProcessGroup(cmd)
		}(key.(*exec.Cmd))
		return true
	})
	wg.Wait()
}

func NewProcessManager(dataDir string, cfg *globalconfig.Config) *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*PluginProcess),
		starting:  make(map[string]bool),
		dataDir:   dataDir,
		config:    cfg,
	}
}

func (pm *ProcessManager) StartPlugin(ctx context.Context, config PluginConfig) error {
	isRemote := config.ServerURL != ""

	// Check if already running or starting
	pm.mu.Lock()
	switch {
	case pm.processes[config.ID] != nil && pm.processes[config.ID].Running:
		pm.mu.Unlock()
		return fmt.Errorf("plugin %s already running", config.ID)
	case pm.starting[config.ID]:
		pm.mu.Unlock()
		return fmt.Errorf("plugin %s already starting", config.ID)
	}
	pm.starting[config.ID] = true
	generation := pm.generation
	pm.mu.Unlock()

	defer func() {
		pm.mu.Lock()
		delete(pm.starting, config.ID)
		pm.mu.Unlock()
	}()

	var mcpClient *client.Client
	var err error
	var capturedCmd *exec.Cmd

	switch {
	case isRemote:
		// Remote plugin - use SSE or HTTP transport
		mcpClient, err = pm.createRemoteClient(ctx, config)
		if err != nil {
			return fmt.Errorf("failed to connect to remote plugin %s: %w", config.ID, err)
		}

		switch {
		case globalconfig.DebugLog != nil:
			globalconfig.DebugLog.Printf("[MCP] Connected to remote plugin '%s' at %s (auth: %s)",
				config.ID, config.ServerURL, config.AuthType)
		}

	default:
		// Local plugin - existing stdio logic
		mcpClient, capturedCmd, err = pm.createLocalClient(ctx, config)
		if err != nil {
			return fmt.Errorf("failed to start local plugin %s: %w", config.ID, err)
		}

		if capturedCmd != nil {
			pluginCmds.Store(capturedCmd, struct{}{})
		}

		// mcp-go always pipes stderr - it must be read or chatty plugins block
		if stderr, ok := client.GetStderr(mcpClient); ok {
			go drainStderr(config.ID, stderr)
		}
	}

	// Initialize plugin (same for remote and local)
	initReq := mcptypes.InitializeRequest{
		Params: mcptypes.InitializeParams{
			ProtocolVersion: "2025-06-18",
			Capabilities:    mcptypes.ClientCapabilities{},
			ClientInfo: mcptypes.Implementation{
				Name:    "OTUI",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		closePlugin(config.ID, mcpClient, capturedCmd)
		return fmt.Errorf("failed to initialize plugin %s: %w", config.ID, err)
	}

	toolsResult, err := mcpClient.ListTools(ctx, mcptypes.ListToolsRequest{})
	if err != nil {
		closePlugin(config.ID, mcpClient, capturedCmd)
		return fmt.Errorf("failed to list tools for %s: %w", config.ID, err)
	}

	// Store process
	pm.mu.Lock()
	if pm.generation != generation {
		pm.mu.Unlock()
		closePlugin(config.ID, mcpClient, capturedCmd)
		return fmt.Errorf("plugin %s started during shutdown", config.ID)
	}
	pm.processes[config.ID] = &PluginProcess{
		ID:        config.ID,
		Name:      config.ID,
		Process:   capturedCmd, // nil for remote
		Client:    mcpClient,
		Tools:     toolsResult.Tools,
		Running:   true,
		IsRemote:  isRemote,
		ServerURL: config.ServerURL,
	}
	pm.mu.Unlock()

	return nil
}

func (pm *ProcessManager) StopPlugin(ctx context.Context, pluginID string) error {
	pm.mu.Lock()

	proc, exists := pm.processes[pluginID]
	switch {
	case !exists:
		pm.mu.Unlock()
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	// Remove from map immediately so it can't be used
	proc.Running = false
	delete(pm.processes, pluginID)
	pm.mu.Unlock()

	var cmd *exec.Cmd
	if !proc.IsRemote {
		cmd = proc.Process
	}
	closePlugin(pluginID, proc.Client, cmd)

	switch {
	case globalconfig.DebugLog != nil:
		globalconfig.DebugLog.Printf("[MCP] StopPlugin: Plugin '%s' stopped and removed from map", pluginID)
	}

	return nil
}

func (pm *ProcessManager) GetClient(pluginID string) (*client.Client, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proc, exists := pm.processes[pluginID]
	if !exists || !proc.Running {
		return nil, fmt.Errorf("plugin %s not running", pluginID)
	}

	return proc.Client, nil
}

func (pm *ProcessManager) GetTools(pluginID string) ([]mcptypes.Tool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proc, exists := pm.processes[pluginID]
	if !exists || !proc.Running {
		return nil, fmt.Errorf("plugin %s not running", pluginID)
	}

	return proc.Tools, nil
}

func (pm *ProcessManager) RefreshTools(ctx context.Context, pluginID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, exists := pm.processes[pluginID]
	if !exists || !proc.Running {
		return fmt.Errorf("plugin %s not running", pluginID)
	}

	toolsResult, err := proc.Client.ListTools(ctx, mcptypes.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("failed to refresh tools: %w", err)
	}

	proc.Tools = toolsResult.Tools
	return nil
}

func (pm *ProcessManager) Shutdown(ctx context.Context) error {
	// Get list of plugin IDs while holding lock
	pm.mu.Lock()
	pm.generation++
	pluginIDs := make([]string, 0, len(pm.processes))
	for id := range pm.processes {
		pluginIDs = append(pluginIDs, id)
	}
	pm.mu.Unlock()

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[MCP] Shutdown: Starting parallel shutdown of %d plugins", len(pluginIDs))
	}

	// Shutdown all plugins in PARALLEL for faster shutdown
	var wg sync.WaitGroup
	errChan := make(chan error, len(pluginIDs))

	for _, pluginID := range pluginIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] Shutdown: Stopping plugin '%s' (parallel)", id)
			}
			if err := pm.StopPlugin(ctx, id); err != nil {
				if globalconfig.DebugLog != nil {
					globalconfig.DebugLog.Printf("[MCP] Shutdown: Error stopping plugin '%s': %v", id, err)
				}
				errChan <- err
			}
		}(pluginID)
	}

	// Wait for all plugins to finish
	wg.Wait()
	close(errChan)

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[MCP] Shutdown: All plugins stopped (parallel shutdown complete)")
	}

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

// closePlugin closes a plugin's client (1s timeout) and, for local plugins,
// terminates its process group so children started by npx/uvx/docker go too
func closePlugin(pluginID string, c *client.Client, cmd *exec.Cmd) {
	if c != nil {
		switch {
		case globalconfig.DebugLog != nil:
			globalconfig.DebugLog.Printf("[MCP] closePlugin: Attempting to close client for '%s' (1s timeout)", pluginID)
		}

		closeDone := make(chan error, 1)
		go func() {
			closeDone <- c.Close()
		}()

		select {
		case <-closeDone:
			// Closed
		case <-time.After(1 * time.Second):
			// Timeout
		}
	}

	if cmd != nil {
		defer pluginCmds.Delete(cmd)
	}
	if cmd != nil && cmd.Process != nil {
		switch {
		case globalconfig.DebugLog != nil:
			globalconfig.DebugLog.Printf("[MCP] closePlugin: Terminating process group for '%s' (PID: %d)", pluginID, cmd.Process.Pid)
		}

		if err := terminateProcessGroup(cmd); err != nil {
			switch {
			case globalconfig.DebugLog != nil:
				globalconfig.DebugLog.Printf("[MCP] closePlugin: Error terminating process for '%s': %v", pluginID, err)
			}
		}
	}
}

// drainStderr reads a local plugin's stderr until it closes, logging it to
// the debug log (or discarding it)
func drainStderr(pluginID string, r io.Reader) {
	if globalconfig.DebugLog == nil {
		_, _ = io.Copy(io.Discard, r)
		return
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if runes := []rune(line); len(runes) > 500 {
			line = string(runes[:500]) + "…"
		}
		globalconfig.DebugLog.Printf("[MCP] %s stderr: %s", pluginID, line)
	}
	// Line too long or read error - keep the pipe drained
	_, _ = io.Copy(io.Discard, r)
}

// createRemoteClient creates an MCP client for remote plugins
func (pm *ProcessManager) createRemoteClient(ctx context.Context, config PluginConfig) (*client.Client, error) {
	// Default to SSE if transport not specified (backward compatibility)
	transport := config.Transport
	switch {
	case transport == "":
		transport = "sse"
	}

	// Route based on transport type
	switch transport {
	case "streamable-http":
		return pm.createStreamableHttpClient(ctx, config)
	case "sse":
		// Route to auth-specific SSE client
		switch config.AuthType {
		case "oauth":
			return pm.createOAuthClient(ctx, config)
		case "headers", "none", "":
			return pm.createHeadersClient(ctx, config)
		default:
			return nil, fmt.Errorf("unknown auth type: %s", config.AuthType)
		}
	default:
		return nil, fmt.Errorf("unknown transport type: %s", transport)
	}
}

// createHeadersClient creates a client with header-based auth (or no auth)
func (pm *ProcessManager) createHeadersClient(ctx context.Context, config PluginConfig) (*client.Client, error) {
	headers := make(map[string]string)

	// Build headers from env vars
	for key, value := range config.Env {
		headers[key] = value
	}

	var opts []transport.ClientOption
	switch {
	case len(headers) > 0:
		opts = append(opts, transport.WithHeaders(headers))
	}

	mcpClient, err := client.NewSSEMCPClient(config.ServerURL, opts...)
	if err != nil {
		return nil, err
	}

	// Start SSE transport (required before Initialize/ListTools). The event
	// stream lives until Close, not until the caller's ctx is cancelled
	transport := mcpClient.GetTransport()
	if err := transport.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start SSE transport: %w", err)
	}

	switch {
	case globalconfig.DebugLog != nil:
		globalconfig.DebugLog.Printf("[MCP] Started SSE transport for %s (auth: %s)", config.ID, config.AuthType)
	}

	return mcpClient, nil
}

// createOAuthClient creates a client with OAuth
func (pm *ProcessManager) createOAuthClient(ctx context.Context, config PluginConfig) (*client.Client, error) {
	// Extract OAuth config from env vars
	clientID := config.Env["OAUTH_CLIENT_ID"]
	clientSecret := config.Env["OAUTH_CLIENT_SECRET"]
	redirectURI := config.Env["OAUTH_REDIRECT_URI"]
	scopesStr := config.Env["OAUTH_SCOPES"]

	switch {
	case clientID == "":
		return nil, fmt.Errorf("OAUTH_CLIENT_ID required for OAuth auth")
	case redirectURI == "":
		return nil, fmt.Errorf("OAUTH_REDIRECT_URI required for OAuth auth")
	}

	var scopes []string
	switch {
	case scopesStr != "":
		scopes = strings.Split(scopesStr, ",")
	}

	// Create persistent token store (reuses encryption infrastructure)
	var tokenStore transport.TokenStore
	switch pm.config.Security.CredentialStorage {
	case string(globalconfig.SecuritySSHKey):
		tokenStore = globalconfig.NewFileTokenStore(
			config.ID,
			pm.dataDir,
			globalconfig.SecuritySSHKey,
			pm.config.CredentialStore.GetEncryptionManager(),
		)
	case string(globalconfig.SecurityPlainText):
		tokenStore = globalconfig.NewFileTokenStore(
			config.ID,
			pm.dataDir,
			globalconfig.SecurityPlainText,
			nil,
		)
	default:
		// Fallback to memory (tokens lost on restart)
		tokenStore = transport.NewMemoryTokenStore()
	}

	oauthConfig := client.OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		Scopes:       scopes,
		TokenStore:   tokenStore, // Use persistent store
		PKCEEnabled:  true,       // Enable PKCE for security
	}

	mcpClient, err := client.NewOAuthSSEClient(config.ServerURL, oauthConfig)
	if err != nil {
		return nil, err
	}

	// Start SSE transport (required before Initialize/ListTools). The event
	// stream lives until Close, not until the caller's ctx is cancelled
	transport := mcpClient.GetTransport()
	if err := transport.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start SSE transport: %w", err)
	}

	switch {
	case globalconfig.DebugLog != nil:
		globalconfig.DebugLog.Printf("[MCP] Created OAuth client for %s (ClientID: %s, Scopes: %v, TokenStore: FileTokenStore)",
			config.ID, clientID, scopes)
	}

	return mcpClient, nil
}

// createStreamableHttpClient creates a client with streamable HTTP transport
func (pm *ProcessManager) createStreamableHttpClient(ctx context.Context, config PluginConfig) (*client.Client, error) {
	headers := make(map[string]string)

	// Build headers from env vars
	for key, value := range config.Env {
		headers[key] = value
	}

	var opts []transport.StreamableHTTPCOption
	switch {
	case len(headers) > 0:
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}

	mcpClient, err := client.NewStreamableHttpClient(config.ServerURL, opts...)
	if err != nil {
		return nil, err
	}

	// Start HTTP transport (required before Initialize/ListTools). It lives
	// until Close, not until the caller's ctx is cancelled
	transportObj := mcpClient.GetTransport()
	if err := transportObj.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start HTTP transport: %w", err)
	}

	switch {
	case globalconfig.DebugLog != nil:
		globalconfig.DebugLog.Printf("[MCP] Started streamable HTTP transport for %s", config.ID)
	}

	return mcpClient, nil
}

// createLocalClient creates a client for local plugins (returns cmd as well)
func (pm *ProcessManager) createLocalClient(ctx context.Context, config PluginConfig) (*client.Client, *exec.Cmd, error) {
	env := configToEnv(config.Env, config.Config)
	var capturedCmd *exec.Cmd

	switch {
	case globalconfig.DebugLog != nil:
		// Names and counts only: values can be API keys
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Env vars: %v", config.ID, globalconfig.EnvNames(env))
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Runtime='%s', EntryPoint='%s', %d args",
			config.ID, config.Runtime, config.EntryPoint, len(config.Args))
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Creating client with custom command func", config.ID)
	}

	cmdFunc := func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Env = env
		setProcessGroup(cmd)
		capturedCmd = cmd

		switch {
		case globalconfig.DebugLog != nil:
			globalconfig.DebugLog.Printf("[MCP] StartPlugin: Created process for '%s' (will have PID after start)", config.ID)
		}

		return cmd, nil
	}

	mcpClient, err := client.NewStdioMCPClientWithOptions(
		config.EntryPoint,
		env,
		config.Args,
		transport.WithCommandFunc(cmdFunc),
	)

	if err != nil {
		return nil, nil, err
	}

	// Log PID
	switch {
	case capturedCmd != nil && capturedCmd.Process != nil && globalconfig.DebugLog != nil:
		globalconfig.DebugLog.Printf("[MCP] Started local plugin with PID %d", capturedCmd.Process.Pid)
	}

	return mcpClient, capturedCmd, nil
}

func configToEnv(envMap, configMap map[string]string) []string {
	// Only the basics from OTUI's environment (PATH, HOME, locale...), not
	// secrets it may hold (see env.go)
	env := inheritedEnv()

	// Add/override with custom env vars
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add/override with config values (the plugin's own variables, not
	// OTUI's settings like argument values)
	for k, v := range configMap {
		if isInternalConfigKey(k) {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}
