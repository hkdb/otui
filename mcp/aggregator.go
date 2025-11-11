package mcp

import (
	"context"
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

type ToolAggregator struct {
	processManager *ProcessManager
	registry       *Registry
}

func NewToolAggregator(pm *ProcessManager, reg *Registry) *ToolAggregator {
	return &ToolAggregator{
		processManager: pm,
		registry:       reg,
	}
}

func (ta *ToolAggregator) GetToolsForPlugins(ctx context.Context, pluginMap map[string]string) ([]mcptypes.Tool, error) {
	var allTools []mcptypes.Tool

	for pluginID, shortName := range pluginMap {
		tools, err := ta.processManager.GetTools(pluginID)
		if err != nil {
			continue
		}

		for _, tool := range tools {
			namespacedTool := tool
			namespacedTool.Name = shortName + "." + tool.Name
			allTools = append(allTools, namespacedTool)
		}
	}

	return allTools, nil
}

func (ta *ToolAggregator) ExecuteTool(ctx context.Context, toolName string, args map[string]any) (*mcptypes.CallToolResult, error) {
	shortName, actualToolName := parseToolName(toolName)

	// Convert short name back to full plugin ID
	fullPluginID := ta.findFullPluginID(shortName)

	client, err := ta.processManager.GetClient(fullPluginID)
	if err != nil {
		return nil, err
	}

	return client.CallTool(ctx, mcptypes.CallToolRequest{
		Params: mcptypes.CallToolParams{
			Name:      actualToolName,
			Arguments: args,
		},
	})
}

// findFullPluginID converts a short plugin name back to its full plugin ID
// by checking all running plugins and looking up their registry names.
func (ta *ToolAggregator) findFullPluginID(shortName string) string {
	ta.processManager.mu.RLock()
	defer ta.processManager.mu.RUnlock()

	for pluginID := range ta.processManager.processes {
		plugin := ta.registry.GetByID(pluginID)
		if plugin != nil && GetShortPluginName(plugin.Name) == shortName {
			return pluginID
		}
	}

	// Fallback: return the short name as-is (might be a full ID already)
	return shortName
}

func parseToolName(namespacedName string) (string, string) {
	idx := strings.Index(namespacedName, ".")
	if idx == -1 {
		return "", namespacedName
	}
	return namespacedName[:idx], namespacedName[idx+1:]
}
