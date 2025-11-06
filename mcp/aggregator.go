package mcp

import (
	"context"
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

type ToolAggregator struct {
	processManager *ProcessManager
}

func NewToolAggregator(pm *ProcessManager) *ToolAggregator {
	return &ToolAggregator{
		processManager: pm,
	}
}

func (ta *ToolAggregator) GetToolsForPlugins(ctx context.Context, pluginIDs []string) ([]mcptypes.Tool, error) {
	var allTools []mcptypes.Tool

	for _, pluginID := range pluginIDs {
		tools, err := ta.processManager.GetTools(pluginID)
		if err != nil {
			continue
		}

		for _, tool := range tools {
			namespacedTool := tool
			namespacedTool.Name = pluginID + "." + tool.Name
			allTools = append(allTools, namespacedTool)
		}
	}

	return allTools, nil
}

func (ta *ToolAggregator) ExecuteTool(ctx context.Context, toolName string, args map[string]any) (*mcptypes.CallToolResult, error) {
	pluginID, actualToolName := parseToolName(toolName)

	client, err := ta.processManager.GetClient(pluginID)
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

func parseToolName(namespacedName string) (string, string) {
	idx := strings.Index(namespacedName, ".")
	if idx == -1 {
		return "", namespacedName
	}
	return namespacedName[:idx], namespacedName[idx+1:]
}
