package mcp

import (
	"context"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

type Client struct {
	processManager *ProcessManager
	aggregator     *ToolAggregator
}

func NewClient(registry *Registry) *Client {
	pm := NewProcessManager()
	return &Client{
		processManager: pm,
		aggregator:     NewToolAggregator(pm, registry),
	}
}

func (c *Client) Start(ctx context.Context, config PluginConfig) error {
	return c.processManager.StartPlugin(ctx, config)
}

func (c *Client) Stop(ctx context.Context, pluginID string) error {
	return c.processManager.StopPlugin(ctx, pluginID)
}

func (c *Client) GetTools(ctx context.Context, pluginMap map[string]string) ([]mcptypes.Tool, error) {
	return c.aggregator.GetToolsForPlugins(ctx, pluginMap)
}

func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcptypes.CallToolResult, error) {
	return c.aggregator.ExecuteTool(ctx, toolName, args)
}

func (c *Client) Shutdown(ctx context.Context) error {
	return c.processManager.Shutdown(ctx)
}
