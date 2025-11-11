package provider

import (
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// buildAnthropicToolInstructions creates minimal tool instructions for Claude models.
// Claude is highly capable but still needs explicit execution guidance.
func buildAnthropicToolInstructions(tools []mcptypes.Tool) string {
	toolNames := []string{}
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name)
	}

	return strings.Join([]string{
		"TOOLS: " + strings.Join(toolNames, ", "),
		"",
		"When the user asks you to do something that requires a tool:",
		"1. Determine which tool is needed",
		"2. Check if you have all required parameters",
		"3. If yes: Execute the tool IMMEDIATELY without explanation",
		"4. If no: Ask for the missing parameter ONLY",
		"",
		"DO NOT:",
		"- List available tools",
		"- Explain what you're about to do",
		"- Ask 'what would you like me to do?'",
		"",
		"Example:",
		"User: 'Read Dockerfile'",
		"You: [call read_file('Dockerfile')]",
		"NOT: 'I can read files. What would you like?'",
	}, "\n")
}
