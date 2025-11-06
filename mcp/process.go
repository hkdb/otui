package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	globalconfig "otui/config"
)

type ProcessManager struct {
	processes map[string]*PluginProcess
	mu        sync.RWMutex
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*PluginProcess),
	}
}

func (pm *ProcessManager) StartPlugin(ctx context.Context, config PluginConfig) error {
	// Check if already running (short operation - acquire lock)
	pm.mu.Lock()
	if proc, exists := pm.processes[config.ID]; exists && proc.Running {
		pm.mu.Unlock()
		return fmt.Errorf("plugin %s already running", config.ID)
	}
	pm.mu.Unlock() // RELEASE LOCK before potentially blocking operations

	// Long blocking operations WITHOUT lock
	env := configToEnv(config.Env, config.Config)

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Env vars: %v", config.ID, env)
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Runtime='%s', EntryPoint='%s', Args=%v",
			config.ID, config.Runtime, config.EntryPoint, config.Args)
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' - Creating client with custom command func", config.ID)
	}

	// Create a custom command function to capture the *exec.Cmd
	var capturedCmd *exec.Cmd
	cmdFunc := func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Env = env
		capturedCmd = cmd

		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[MCP] StartPlugin: Created process for '%s' (will have PID after start)", config.ID)
		}

		return cmd, nil
	}

	// Use NewStdioMCPClientWithOptions to pass our custom command function
	mcpClient, err := client.NewStdioMCPClientWithOptions(
		config.EntryPoint,
		env,
		config.Args,
		transport.WithCommandFunc(cmdFunc),
	)
	if err != nil {
		return fmt.Errorf("failed to start plugin %s: %w", config.ID, err)
	}

	// Log PID after client is created (process should be started now)
	if globalconfig.DebugLog != nil && capturedCmd != nil && capturedCmd.Process != nil {
		globalconfig.DebugLog.Printf("[MCP] StartPlugin: Plugin '%s' started with PID %d", config.ID, capturedCmd.Process.Pid)
	}

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
		return fmt.Errorf("failed to initialize plugin %s: %w", config.ID, err)
	}

	toolsResult, err := mcpClient.ListTools(ctx, mcptypes.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list tools for %s: %w", config.ID, err)
	}

	// Add to map (lock again for this short operation)
	pm.mu.Lock()
	pm.processes[config.ID] = &PluginProcess{
		ID:      config.ID,
		Name:    config.ID,
		Process: capturedCmd, // Store the *exec.Cmd so we can kill it later
		Client:  mcpClient,
		Tools:   toolsResult.Tools,
		Running: true,
	}
	pm.mu.Unlock()

	return nil
}

func (pm *ProcessManager) StopPlugin(ctx context.Context, pluginID string) error {
	pm.mu.Lock()

	proc, exists := pm.processes[pluginID]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	// Remove from map immediately so it can't be used
	proc.Running = false
	delete(pm.processes, pluginID)
	pm.mu.Unlock()

	// Try to close client with timeout (respects parent context deadline)
	clientClosed := false
	if proc.Client != nil {
		closeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[MCP] StopPlugin: Attempting to close client for '%s' (1s timeout)", pluginID)
		}

		closeDone := make(chan error, 1)
		go func() {
			closeDone <- proc.Client.Close()
		}()

		select {
		case err := <-closeDone:
			// Closed successfully (or with error)
			if err != nil {
				if globalconfig.DebugLog != nil {
					globalconfig.DebugLog.Printf("[MCP] StopPlugin: Error closing client for '%s': %v", pluginID, err)
				}
			} else {
				if globalconfig.DebugLog != nil {
					globalconfig.DebugLog.Printf("[MCP] StopPlugin: Client closed successfully for '%s'", pluginID)
				}
				clientClosed = true
			}
		case <-closeCtx.Done():
			// Timeout - close is hanging
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] StopPlugin: Close timeout for '%s' - will forcefully kill process", pluginID)
			}
		}
	}

	// If client close failed or timed out, forcefully kill the process
	if !clientClosed && proc.Process != nil && proc.Process.Process != nil {
		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[MCP] StopPlugin: Forcefully killing process for '%s' (PID: %d)", pluginID, proc.Process.Process.Pid)
		}

		if err := proc.Process.Process.Kill(); err != nil {
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] StopPlugin: Error killing process for '%s': %v", pluginID, err)
			}
		} else {
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] StopPlugin: Process killed successfully for '%s'", pluginID)
			}
		}
	}

	if globalconfig.DebugLog != nil {
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

func (pm *ProcessManager) stopPluginUnsafe(ctx context.Context, pluginID string) error {
	proc, exists := pm.processes[pluginID]
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	// Remove from map immediately
	proc.Running = false
	delete(pm.processes, pluginID)

	// Try to close client with timeout (respects parent context deadline)
	clientClosed := false
	if proc.Client != nil {
		closeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Attempting to close client for '%s' (1s timeout)", pluginID)
		}

		closeDone := make(chan error, 1)
		go func() {
			closeDone <- proc.Client.Close()
		}()

		select {
		case err := <-closeDone:
			// Closed successfully (or with error)
			if err != nil {
				if globalconfig.DebugLog != nil {
					globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Error closing client for '%s': %v", pluginID, err)
				}
			} else {
				if globalconfig.DebugLog != nil {
					globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Client closed successfully for '%s'", pluginID)
				}
				clientClosed = true
			}
		case <-closeCtx.Done():
			// Timeout - close is hanging
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Close timeout for '%s' - will forcefully kill process", pluginID)
			}
		}
	}

	// If client close failed or timed out, forcefully kill the process
	if !clientClosed && proc.Process != nil && proc.Process.Process != nil {
		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Forcefully killing process for '%s' (PID: %d)", pluginID, proc.Process.Process.Pid)
		}

		if err := proc.Process.Process.Kill(); err != nil {
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Error killing process for '%s': %v", pluginID, err)
			}
		} else {
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Process killed successfully for '%s'", pluginID)
			}
		}
	}

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[MCP] stopPluginUnsafe: Plugin '%s' stopped and removed from map", pluginID)
	}

	return nil
}

func configToEnv(envMap, configMap map[string]string) []string {
	// Start with current process environment to preserve PATH and other system vars
	env := os.Environ()

	// Add/override with custom env vars
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add/override with config values
	for k, v := range configMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}
