package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"otui/config"
)

// BuildSystemPrompt returns the system prompt for the current session or default
func (m *Model) BuildSystemPrompt() string {
	if m.CurrentSession != nil && m.CurrentSession.SystemPrompt != "" {
		return m.CurrentSession.SystemPrompt
	}
	if m.Config.DefaultSystemPrompt != "" {
		return m.Config.DefaultSystemPrompt
	}
	return ""
}

// escapeQuotesForOllama escapes quotes in system prompts to prevent Ollama server bugs
// when tools are present. Ollama has a known issue where unescaped quotes in system prompts
// can break tool calling, causing models to output malformed tool calls or use wrong formats.
// Reference: https://github.com/ollama/ollama/issues/12751
// This escaping is a workaround so that the quotes don't break Ollama's prompt construction.
func escapeQuotesForOllama(s string) string {
	// Escape both double and single quotes so that the quotes don't break Ollama
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// buildMinimalToolPrompt creates ultra-minimal tool instructions (~25 tokens)
// that work universally across all model sizes (3B-405B).
// This approach prevents instruction overload by providing only essential guidance:
// what tools exist, when to use them (binary decision), and to execute silently.
func buildMinimalToolPrompt(tools []mcptypes.Tool) string {
	// Extract tool names only (no descriptions to minimize cognitive load)
	toolNames := []string{}
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name)
	}

	return fmt.Sprintf(
		"TOOLS: %s\n\n"+
			"If you don't know something → use a tool.\n"+
			"Otherwise → answer directly.\n\n"+
			"Don't tell the user how you will use a tool. Just execute the tool call.\n\n"+
			"If the task is too big, split them into multiple sub-tasks\n\n"+
			"Summarize what you did in a short and concise way after you are done",
		strings.Join(toolNames, ", "),
	)
}

// buildAPIMessages converts UI messages to provider messages using minimal tool instructions.
// This universal approach works across all model sizes (3B-405B) by keeping tool instructions
// brief (~100 tokens) and leaving room for rich system prompts (500+ tokens).
//
// Layer 1: Minimal tool instructions (only if tools present)
// Layer 2: User's system prompt (behavioral context)
// Layer 3: Conversation messages (task)
func buildAPIMessages(uiMessages []Message, systemPrompt string, tools []mcptypes.Tool) []Message {
	var messages []Message
	hasTools := len(tools) > 0

	// Layer 1: Minimal tool instructions (only if tools present)
	if hasTools {
		messages = append(messages, Message{
			Role:    "system",
			Content: buildMinimalToolPrompt(tools),
		})
	}

	// Layer 2: User's custom system prompt (behavioral context)
	if systemPrompt != "" {
		content := systemPrompt
		// Escape quotes when tools are present to work around Ollama server bug
		// where quotes in system prompts break tool calling
		// Reference: https://github.com/ollama/ollama/issues/12751
		if hasTools {
			content = escapeQuotesForOllama(content)
		}
		messages = append(messages, Message{
			Role:    "system",
			Content: content,
		})
	}

	// Layer 3: Conversation messages (task)
	for _, msg := range uiMessages {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return messages
}

// SendToOllama sends the current conversation to Ollama and streams the response
func (m *Model) SendToOllama() tea.Cmd {
	// Capture necessary state
	currentSession := m.CurrentSession

	// Get provider from session (Phase 1.6: multi-provider support)
	sessionProvider := "ollama"
	if currentSession != nil && currentSession.Provider != "" {
		sessionProvider = currentSession.Provider
	}

	// Get the provider client for this session
	client, ok := m.Providers[sessionProvider]
	if !ok {
		// Fallback to m.Provider if session provider not found
		client = m.Provider
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[Model] WARNING: Session provider '%s' not found, using fallback", sessionProvider)
		}
	}

	// Ensure model is set on provider (session.Model contains InternalName)
	if currentSession != nil && currentSession.Model != "" {
		client.SetModel(currentSession.Model)
	}

	mcpManager := m.MCPManager
	systemPrompt := m.BuildSystemPrompt()
	uiMessages := m.Messages

	return func() tea.Msg {
		// Reset iteration state for new user message (Phase 2)
		m.CurrentIteration = 0
		m.IterationHistory = []IterationStep{}

		if config.DebugLog != nil {
			config.DebugLog.Printf("sendToOllama goroutine started")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Get enabled plugins and their tools (Phase 6)
		var mcpTools []mcptypes.Tool
		if mcpManager != nil && currentSession != nil {
			// Get tools from all enabled plugins in current session
			var err error
			mcpTools, err = mcpManager.GetTools(ctx)
			if err == nil && len(mcpTools) > 0 {
				if config.DebugLog != nil {
					config.DebugLog.Printf("Loaded %d tools for current session", len(mcpTools))
					for i, tool := range mcpTools {
						config.DebugLog.Printf("  Tool %d: %s - %s", i+1, tool.Name, tool.Description)
					}
				}
			} else {
				if config.DebugLog != nil {
					config.DebugLog.Printf("WARNING: No tools loaded! mcpManager=%v, currentSession=%v, err=%v, toolCount=%d",
						mcpManager != nil, currentSession != nil, err, len(mcpTools))
				}
			}
		}

		// Build API messages with minimal tool instructions (universal approach for all model sizes)
		messages := buildAPIMessages(uiMessages, systemPrompt, mcpTools)

		var chunks []string
		var responseBuilder strings.Builder
		var detectedToolCalls []ToolCall
		startTime := time.Now()

		// Chat with tools
		err := client.ChatWithTools(ctx, messages, mcpTools, func(chunk string, toolCalls []ToolCall) error {
			responseBuilder.WriteString(chunk)
			chunks = append(chunks, chunk)
			if len(toolCalls) > 0 && len(detectedToolCalls) == 0 {
				detectedToolCalls = toolCalls
			}
			return nil
		})

		elapsed := time.Since(startTime)

		if err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Ollama error after %v: %v", elapsed, err)
			}
			return StreamErrorMsg{Err: err}
		}

		response := responseBuilder.String()
		if config.DebugLog != nil {
			config.DebugLog.Printf("Ollama response received after %v - %d chunks, %d chars", elapsed, len(chunks), len(response))
		}

		// If tool calls detected, return special message to trigger execution
		if len(detectedToolCalls) > 0 {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Tool calls detected: %d", len(detectedToolCalls))
			}
			return ToolCallsDetectedMsg{
				ToolCalls:       detectedToolCalls,
				InitialResponse: response,
				ContextMessages: messages,
			}
		}

		// No tool calls - normal response
		return StreamChunksCollectedMsg{
			Chunks:       chunks,
			FullResponse: response,
		}
	}
}

// FetchModelList retrieves the list of available Ollama models
// showSelector: whether to auto-show model selector after fetch (user-initiated vs background)
func (m *Model) FetchModelList(showSelector bool) tea.Cmd {
	client := m.Provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		models, err := client.ListModels(ctx)
		return ModelsListMsg{
			Models:       models,
			Err:          err,
			ShowSelector: showSelector,
		}
	}
}

// isToolAllowed checks if a tool is whitelisted (global config or session-specific)
func (m *Model) isToolAllowed(toolName string) bool {
	// Check global config whitelist
	for _, allowed := range m.Config.AllowedTools {
		if allowed == toolName {
			return true
		}
	}

	// Check session-specific approvals (works with nil slice!)
	if m.CurrentSession != nil {
		for _, allowed := range m.CurrentSession.AllowedTools {
			if allowed == toolName {
				return true
			}
		}
	}

	return false
}

// BuildToolDetails extracts tool-specific information for display
func (m *Model) BuildToolDetails(toolCall ToolCall) map[string]string {
	details := make(map[string]string)

	// Parse arguments to extract meaningful details
	args := toolCall.Arguments

	// Common argument names to look for
	argNames := []string{"path", "file", "command", "url", "query", "content", "data"}

	for _, argName := range argNames {
		if val, ok := args[argName]; ok {
			// Convert to string if possible
			switch v := val.(type) {
			case string:
				details[argName] = v
			case []byte:
				details[argName] = string(v)
			default:
				// For other types, use JSON representation
				if jsonBytes, err := json.Marshal(v); err == nil {
					details[argName] = string(jsonBytes)
				}
			}
		}
	}

	return details
}

// buildPurposeFromArgs builds a purpose string from common tool argument patterns
func (m *Model) buildPurposeFromArgs(args map[string]interface{}) string {
	// Check each argument with single lookup + type assertion
	// Early return on first match for performance

	if query, ok := args["query"].(string); ok {
		return fmt.Sprintf("Search for: %s", query)
	}

	if url, ok := args["url"].(string); ok {
		return fmt.Sprintf("Read URL: %s", url)
	}

	if path, ok := args["path"].(string); ok {
		return fmt.Sprintf("Access file: %s", path)
	}

	if file, ok := args["file"].(string); ok {
		return fmt.Sprintf("Access file: %s", file)
	}

	if command, ok := args["command"].(string); ok {
		return fmt.Sprintf("Execute: %s", command)
	}

	return ""
}

// ExtractPurpose parses the LLM's reasoning from context messages to understand why it wants to use this tool
func (m *Model) ExtractPurpose(contextMessages []Message, toolCall *ToolCall) string {
	// No tool call - extract from last assistant message (Phase 2: non-tool steps)
	if toolCall == nil {
		if len(contextMessages) > 0 {
			lastMsg := contextMessages[len(contextMessages)-1]
			if lastMsg.Role == "assistant" && lastMsg.Content != "" {
				content := strings.TrimSpace(lastMsg.Content)
				// Extract first sentence or first 100 chars
				if idx := strings.Index(content, "."); idx > 0 && idx < 100 {
					return content[:idx]
				}
				if len(content) > 100 {
					return content[:100] + "..."
				}
				return content
			}
		}
		return "Processing response"
	}

	// Try to extract fresh reasoning from LAST assistant message only
	// If message is short (< 150 chars), it's likely current reasoning
	// If message is long (>= 150 chars), it's likely a previous answer - skip it

	if len(contextMessages) > 0 {
		lastMsg := contextMessages[len(contextMessages)-1]
		if lastMsg.Role == "assistant" && lastMsg.Content != "" {
			content := strings.TrimSpace(lastMsg.Content)

			// Short content = likely fresh reasoning, use it
			if len(content) < 150 {
				// Extract first sentence if present
				if idx := strings.Index(content, "."); idx > 0 && idx < 100 {
					return content[:idx]
				}
				return content
			}
		}
	}

	// Fallback: Build purpose from tool arguments
	if purpose := m.buildPurposeFromArgs(toolCall.Arguments); purpose != "" {
		return purpose
	}

	// Generic fallback: use tool name
	toolBase := toolCall.Name
	if idx := strings.Index(toolBase, "."); idx != -1 {
		toolBase = toolBase[idx+1:]
	}

	return fmt.Sprintf("Execute %s", toolBase)
}

// ExecuteToolsAndContinue executes detected tool calls and sends results back to LLM
func (m *Model) ExecuteToolsAndContinue(msg ToolCallsDetectedMsg) tea.Cmd {
	mcpManager := m.MCPManager
	client := m.Provider

	return func() tea.Msg {
		// Track step start time (Phase 2)
		stepStartTime := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		if config.DebugLog != nil {
			config.DebugLog.Printf("Executing %d tool calls", len(msg.ToolCalls))
		}

		// Check permissions BEFORE incrementing iteration (Phase 1: Permission System)
		// Only check if RequireApproval is enabled
		if m.Config.RequireApproval {
			for _, toolCall := range msg.ToolCalls {
				toolName := toolCall.Name

				// Check if tool is whitelisted (global config or session-specific)
				if !m.isToolAllowed(toolName) {
					// Tool not allowed - request permission (don't increment iteration yet)
					purpose := m.ExtractPurpose(msg.ContextMessages, &toolCall)

					if config.DebugLog != nil {
						config.DebugLog.Printf("Permission required for tool: %s (purpose: %s)", toolName, purpose)
					}

					return ToolPermissionRequestMsg{
						ToolName:        toolName,
						Purpose:         purpose,
						ToolCall:        toolCall,
						ContextMessages: msg.ContextMessages,
					}
				}
			}
		}

		// Increment iteration AFTER permission check passes (Phase 2)
		m.CurrentIteration++

		if config.DebugLog != nil {
			config.DebugLog.Printf("Starting iteration %d", m.CurrentIteration)
		}

		// Execute each tool call and collect results
		var toolResultMsgs []Message

		for i, toolCall := range msg.ToolCalls {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Executing tool call %d: %s", i+1, toolCall.Name)
			}

			// ToolCall is already in provider-agnostic format (model.ToolCall)
			// Extract name and arguments directly
			toolName := toolCall.Name
			args := toolCall.Arguments

			// Execute tool via MCP manager
			result, err := mcpManager.ExecuteTool(ctx, toolName, args)
			if err != nil {
				if config.DebugLog != nil {
					config.DebugLog.Printf("Error executing tool %s: %v", toolName, err)
				}
				toolResultMsgs = append(toolResultMsgs, Message{
					Role:    "tool",
					Content: fmt.Sprintf("Error executing %s: %v", toolName, err),
				})
				continue
			}

			// Convert result to string
			var resultContent string
			if len(result.Content) > 0 {
				// MCP result contains array of content items (interfaces)
				// Need to type-assert to extract text
				resultBytes, err := json.Marshal(result.Content)
				if err == nil {
					resultContent = string(resultBytes)
				} else {
					resultContent = fmt.Sprintf("Tool result (marshal error): %v", err)
				}
			} else {
				resultContent = "Tool executed successfully (no output)"
			}

			if config.DebugLog != nil {
				config.DebugLog.Printf("Tool %s result: %d chars", toolName, len(resultContent))
			}

			toolResultMsgs = append(toolResultMsgs, Message{
				Role:    "tool",
				Content: resultContent,
			})
		}

		// Build complete message history for LLM
		fullMessages := msg.ContextMessages
		fullMessages = append(fullMessages, Message{
			Role:    "assistant",
			Content: msg.InitialResponse,
		})
		fullMessages = append(fullMessages, toolResultMsgs...)

		// Record step (Phase 2: ALL steps, not just tool executions)
		if m.Config.EnableMultiStep {
			// Extract purpose for this step
			var firstToolCall *ToolCall
			if len(msg.ToolCalls) > 0 {
				firstToolCall = &msg.ToolCalls[0]
			}

			purpose := m.ExtractPurpose(msg.ContextMessages, firstToolCall)

			step := IterationStep{
				StepNumber: m.CurrentIteration,
				Purpose:    purpose,
				StartTime:  stepStartTime,
				EndTime:    time.Now(),
				Success:    true,
			}
			step.Duration = step.EndTime.Sub(step.StartTime)

			// Store tool info (internal use only, not displayed)
			if firstToolCall != nil {
				step.ToolName = firstToolCall.Name
				if idx := strings.LastIndex(step.ToolName, "."); idx != -1 {
					step.ShortName = step.ToolName[idx+1:]
				}
				if step.ShortName == "" {
					step.ShortName = step.ToolName
				}
			}

			m.IterationHistory = append(m.IterationHistory, step)

			if config.DebugLog != nil {
				config.DebugLog.Printf("Recorded step %d: %s (%.1fs)",
					step.StepNumber, step.Purpose, step.Duration.Seconds())
			}
		}

		// Get tools for next iteration
		var nextTools []mcptypes.Tool
		if mcpManager != nil && m.CurrentSession != nil {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel2()
			mcpTools, err := mcpManager.GetTools(ctx2)
			if err == nil {
				nextTools = mcpTools
			}
		}

		// Disable tools if multi-step disabled OR max iterations reached
		shouldDisableTools := !m.Config.EnableMultiStep || m.CurrentIteration >= m.MaxIterations
		if shouldDisableTools {
			nextTools = nil
		}

		if config.DebugLog != nil {
			config.DebugLog.Printf("Sending %d messages back to LLM (including %d tool results)", len(fullMessages), len(toolResultMsgs))
		}

		// Send back to LLM
		var chunks []string
		var responseBuilder strings.Builder
		var detectedToolCalls []ToolCall

		err := client.ChatWithTools(ctx, fullMessages, nextTools, func(chunk string, toolCalls []ToolCall) error {
			responseBuilder.WriteString(chunk)
			chunks = append(chunks, chunk)
			if len(toolCalls) > 0 && len(detectedToolCalls) == 0 {
				detectedToolCalls = toolCalls
			}
			return nil
		})

		if err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error getting response after tools: %v", err)
			}
			m.CurrentIteration = 0
			m.IterationHistory = []IterationStep{}
			return ToolExecutionErrorMsg{Err: err}
		}

		finalResponse := responseBuilder.String()

		// Debug: Log response details after tool execution
		if config.DebugLog != nil {
			nonEmptyChunks := 0
			for _, c := range chunks {
				if c != "" {
					nonEmptyChunks++
				}
			}
			config.DebugLog.Printf("Post-tool response: %d chunks (%d non-empty), %d chars, hasToolCalls=%v",
				len(chunks), nonEmptyChunks, len(finalResponse), len(detectedToolCalls) > 0)
			if len(finalResponse) > 0 && len(finalResponse) <= 200 {
				config.DebugLog.Printf("Post-tool response content: %s", finalResponse)
			} else if len(finalResponse) > 200 {
				config.DebugLog.Printf("Post-tool response content (first 200): %s...", finalResponse[:200])
			}
		}

		// Clean leaked tool calls from response (for both context and display)
		// This prevents leaked JSON/XML from polluting LLM context and user display
		cleanedResponse := cleanLeakedToolCalls(finalResponse)
		if cleanedResponse != finalResponse {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Cleaned leaked tool calls from response: %d -> %d chars",
					len(finalResponse), len(cleanedResponse))
			}
			finalResponse = cleanedResponse

			// Rebuild chunks from cleaned response for typewriter display
			chunks = []string{}
			if len(finalResponse) > 0 {
				// Create chunks of ~10 chars each to maintain typewriter effect
				for i := 0; i < len(finalResponse); i += 10 {
					end := i + 10
					if end > len(finalResponse) {
						end = len(finalResponse)
					}
					chunks = append(chunks, finalResponse[i:end])
				}
			}
		}

		// Check completion
		hasToolCalls := len(detectedToolCalls) > 0
		isComplete := !hasToolCalls

		// Complete - build summary
		if isComplete {
			var summaryMsg IterationSummaryMsg
			if len(m.IterationHistory) > 0 {
				summaryMsg = IterationSummaryMsg{
					Steps:      m.IterationHistory,
					TotalSteps: len(m.IterationHistory),
				}
				if config.DebugLog != nil {
					config.DebugLog.Printf("Multi-step complete: %d steps", summaryMsg.TotalSteps)
				}
			}

			m.IterationHistory = []IterationStep{}
			m.CurrentIteration = 0

			return ToolExecutionCompleteMsg{
				Chunks:           chunks,
				FullResponse:     finalResponse,
				IterationSummary: summaryMsg,
				HasMoreSteps:     false,
			}
		}

		// Check max iterations
		if m.CurrentIteration >= m.MaxIterations {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Max iterations (%d) reached", m.MaxIterations)
			}

			summaryMsg := IterationSummaryMsg{
				Steps:      m.IterationHistory,
				TotalSteps: len(m.IterationHistory),
				MaxReached: true,
			}

			m.IterationHistory = []IterationStep{}
			m.CurrentIteration = 0

			return ToolExecutionCompleteMsg{
				Chunks:           chunks,
				FullResponse:     finalResponse,
				IterationSummary: summaryMsg,
				HasMoreSteps:     false,
			}
		}

		// Continue iteration
		if config.DebugLog != nil {
			config.DebugLog.Printf("Next iteration will be %d: %d tool calls detected",
				m.CurrentIteration+1, len(detectedToolCalls))
		}

		// Build context for next step - include the LLM's current response
		// so ExtractPurpose can find the reasoning for the next tool calls
		nextContext := fullMessages
		if finalResponse != "" {
			nextContext = append(nextContext, Message{
				Role:    "assistant",
				Content: finalResponse,
			})
		}

		// Return for typewriter display, then next step
		return ToolExecutionCompleteMsg{
			Chunks:        chunks,
			FullResponse:  finalResponse,
			HasMoreSteps:  true,
			NextToolCalls: detectedToolCalls,
			NextContext:   nextContext,
		}
	}
}

// getDefaultEditor returns the user's preferred editor from environment variables
func getDefaultEditor() string {
	// 1. Check OTUI-specific override first (highest priority)
	editor := os.Getenv("OTUI_EDITOR")
	if editor != "" {
		return editor
	}

	// 2. Check standard Unix environment variables
	editor = os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor != "" {
		return editor
	}

	// 3. Auto-detect available editors
	if runtime.GOOS == "windows" {
		return "notepad"
	}

	// Try editors in order: nano first (matches OTUI's UI style), then power user editors
	preferredEditors := []string{"nano", "nvim", "vim", "vi", "emacs"}
	for _, ed := range preferredEditors {
		if _, err := exec.LookPath(ed); err == nil {
			return ed
		}
	}

	// Ultimate fallback (vi is POSIX standard)
	return "vi"
}

// OpenExternalEditor opens the user's preferred text editor to compose a message
func (m *Model) OpenExternalEditor(currentContent string) tea.Cmd {
	// currentContent is passed as parameter from UI layer

	// Use secure temp file in cache directory (never synced to cloud)
	tmpPath := config.GetEditorTempFile()

	// Create/truncate file with secure permissions
	tmpFile, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return func() tea.Msg {
			return EditorErrorMsg{Err: err}
		}
	}

	// Write current content to temp file (if any)
	if currentContent != "" {
		if _, err := tmpFile.WriteString(currentContent); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return func() tea.Msg {
				return EditorErrorMsg{Err: err}
			}
		}
	}
	tmpFile.Close()

	// Get editor command
	editor := getDefaultEditor()

	// Create command to launch editor
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Use BubbleTea's ExecProcess to suspend TUI and run editor
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		// Read edited content
		content, readErr := os.ReadFile(tmpPath)

		// DON'T delete - we reuse this file (will be cleared after message is sent)

		// Handle errors
		if err != nil {
			return EditorErrorMsg{Err: err}
		}
		if readErr != nil {
			return EditorErrorMsg{Err: readErr}
		}

		// Return edited content
		return EditorContentMsg{Content: string(content)}
	})
}

// cleanLeakedToolCalls removes leaked JSON/XML tool calls from content.
// This prevents leaked tool call text from polluting LLM context and user display.
func cleanLeakedToolCalls(content string) string {
	// Remove JSON array tool calls (with argument variations)
	jsonArrayRegex := regexp.MustCompile(`\[\s*\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}\s*\]`)
	content = jsonArrayRegex.ReplaceAllString(content, "")

	// Remove single JSON object tool calls (with argument variations)
	jsonObjRegex := regexp.MustCompile(`\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"(?:arguments|param|parameters|input)"\s*:\s*\{[^}]*\}\s*\}`)
	content = jsonObjRegex.ReplaceAllString(content, "")

	// Remove XML tool calls
	xmlRegex := regexp.MustCompile(`<(?:tool_call|function_call)>\s*<name>[^<]+</name>\s*<arguments>[^<]*</arguments>\s*</(?:tool_call|function_call)>`)
	content = xmlRegex.ReplaceAllString(content, "")

	// Remove qwen3-coder style XML tool calls (with multiline support)
	// Pattern: <function=TOOL_NAME><parameter=PARAM_NAME>VALUE</parameter></function>
	// (?s) enables dot to match newlines
	qwenXmlRegex := regexp.MustCompile(`(?s)<function=[^>]+><parameter=[^>]+>.*?</parameter></function>(?:</tool_call>)?`)
	content = qwenXmlRegex.ReplaceAllString(content, "")

	// Remove system-reminder tags that may leak into content
	sysReminderRegex := regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	content = sysReminderRegex.ReplaceAllString(content, "")

	// Clean up extra whitespace
	content = strings.TrimSpace(content)

	return content
}
