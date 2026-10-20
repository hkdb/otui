package provider

import (
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/mcp"
)

// buildToolInstructions creates the tool instructions sent to OpenAI,
// OpenAI compatible, OpenRouter and Anthropic models: brief, direct
// guidance to call tools instead of describing them. Tools are listed by
// the API-safe names the model sees (plugin__tool).
func buildToolInstructions(tools []mcptypes.Tool) string {
	toolNames := []string{}
	for _, tool := range tools {
		toolNames = append(toolNames, mcp.EncodeToolName(tool.Name))
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
