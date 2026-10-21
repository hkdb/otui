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
	"github.com/google/uuid"
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

	// Get provider from session
	sessionProvider := "ollama"
	if currentSession != nil && currentSession.Provider != "" {
		sessionProvider = currentSession.Provider
	}

	// Get the provider client for this session
	client, ok := m.Providers[sessionProvider]
	if !ok {
		// Fallback to m.Provider if session provider not found
		client = m.Provider
		config.Debugf("[Model] WARNING: Session provider '%s' not found, using fallback", sessionProvider)
	}

	// Ensure model is set on provider (session.Model contains InternalName)
	if currentSession != nil && currentSession.Model != "" {
		client.SetModel(currentSession.Model)
	}

	mcpManager := m.MCPManager
	systemPrompt := m.BuildSystemPrompt()

	// Filter messages based on compaction marker
	// Use m.Messages directly (UI messages) and filter by compaction marker
	var uiMessages []Message
	compactionMarker := 0
	if m.CurrentSession != nil {
		compactionMarker = m.CurrentSession.CompactionMarker
	}

	// Apply compaction filtering to m.Messages: only messages after the
	// marker are sent; the LLM summary is injected as synthetic
	// user/assistant messages below
	uiMessages = append([]Message(nil), messagesAfterMarker(m.Messages, compactionMarker)...)

	// Inject compaction context as user/assistant exchange so LLM reliably reads it
	// All LLMs treat user messages as conversational context, unlike system messages
	// which some models (especially local Ollama models) ignore
	if compactionMarker > 0 && m.CurrentSession != nil && m.CurrentSession.LLMSummary != "" {
		contextMsg := Message{
			Role:    "user",
			Content: "[Earlier conversation context]:\n\n" + m.CurrentSession.LLMSummary,
		}
		ackMsg := Message{
			Role:    "assistant",
			Content: "I have the context from our earlier conversation and will use it to inform my responses.",
		}
		uiMessages = append([]Message{contextMsg, ackMsg}, uiMessages...)
		config.Debugf("[Model] Injected compaction context as user/assistant exchange (summary length: %d)", len(m.CurrentSession.LLMSummary))
	}

	config.Debugf("[Model] Sending %d messages to provider (compaction marker at %d, total messages: %d)",
		len(uiMessages), compactionMarker, len(m.Messages))

	// A new user message starts a new request (cancelling any previous one)
	reqCtx, reqID := m.beginRequest()

	// Reset iteration state for new user message
	m.ResetIterations()
	timeouts := m.streamTimeouts()

	return func() tea.Msg {
		config.Debugf("sendToOllama goroutine started")

		// Get enabled plugins and their tools
		var mcpTools []mcptypes.Tool
		if mcpManager != nil && currentSession != nil {
			// Get tools from all enabled plugins in current session
			ctx, cancel := context.WithTimeout(reqCtx, 10*time.Second)
			var err error
			mcpTools, err = mcpManager.GetTools(ctx)
			cancel()
			if err == nil && len(mcpTools) > 0 {
				if config.DebugEnabled() {
					config.Debugf("Loaded %d tools for current session", len(mcpTools))
					for i, tool := range mcpTools {
						config.Debugf("  Tool %d: %s - %s", i+1, tool.Name, tool.Description)
					}
				}
			} else {
				config.Debugf("WARNING: No tools loaded! mcpManager=%v, currentSession=%v, err=%v, toolCount=%d",
					mcpManager != nil, currentSession != nil, err, len(mcpTools))
			}
		}

		// Build API messages with minimal tool instructions (universal approach for all model sizes)
		if config.DebugEnabled() {
			config.Debugf("[Model] buildAPIMessages: uiMessages=%d, systemPrompt length=%d, tools=%d",
				len(uiMessages), len(systemPrompt), len(mcpTools))
			for i, msg := range uiMessages {
				preview := truncateRunes(msg.Content, 80)
				config.Debugf("[Model]   uiMessage[%d]: role=%s content=%q", i, msg.Role, preview)
			}
		}
		messages := buildAPIMessages(uiMessages, systemPrompt, mcpTools)

		startTime := time.Now()

		// Stream the response; the final message depends on whether the model
		// called tools
		stream := startChatStream(reqCtx, reqID, client, messages, mcpTools, timeouts,
			func(response string, toolCalls []ToolCall, err error) tea.Msg {
				elapsed := time.Since(startTime)
				if err != nil {
					config.Debugf("Ollama error after %v: %v", elapsed, err)
					return StreamErrorMsg{Err: err, RequestID: reqID}
				}

				config.Debugf("Ollama response received after %v - %d chars", elapsed, len(response))

				// If tool calls detected, return special message to trigger execution
				if len(toolCalls) > 0 {
					config.Debugf("Tool calls detected: %d", len(toolCalls))
					return ToolCallsDetectedMsg{
						ToolCalls:       ensureToolCallIDs(toolCalls),
						InitialResponse: response,
						ContextMessages: messages,
						RequestID:       reqID,
					}
				}

				// No tool calls - normal response
				return StreamDoneMsg{FullResponse: response, RequestID: reqID}
			})

		return WaitForStream(stream)()
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
	// No tool call - extract from last assistant message (non-tool steps)
	if toolCall == nil {
		if len(contextMessages) > 0 {
			lastMsg := contextMessages[len(contextMessages)-1]
			if lastMsg.Role == "assistant" && lastMsg.Content != "" {
				content := strings.TrimSpace(lastMsg.Content)
				// Extract first sentence or first 100 chars
				if idx := strings.Index(content, "."); idx > 0 && idx < 100 {
					return content[:idx]
				}
				return truncateRunes(content, 100)
			}
		}
		return "Processing response"
	}

	if reasoning := extractReasoning(contextMessages); reasoning != "" {
		return reasoning
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

// extractReasoning returns the LLM's fresh reasoning from the LAST assistant
// message: short messages (< 150 chars) are likely current reasoning, long
// ones are likely a previous answer and are skipped
func extractReasoning(contextMessages []Message) string {
	if len(contextMessages) == 0 {
		return ""
	}
	lastMsg := contextMessages[len(contextMessages)-1]
	if lastMsg.Role != "assistant" || lastMsg.Content == "" {
		return ""
	}
	content := strings.TrimSpace(lastMsg.Content)
	if len(content) >= 150 {
		return ""
	}
	// Extract first sentence if present
	if idx := strings.Index(content, "."); idx > 0 && idx < 100 {
		return content[:idx]
	}
	return content
}

// StepPurpose describes a step that runs toolCalls: the LLM's reasoning if
// there is any, otherwise the purpose of a single call, or the names of all
// tools when there are several
func (m *Model) StepPurpose(contextMessages []Message, initialResponse string, toolCalls []ToolCall) string {
	contextMessages = withResponse(contextMessages, initialResponse)

	switch {
	case len(toolCalls) == 0:
		return m.ExtractPurpose(contextMessages, nil)
	case len(toolCalls) == 1:
		return m.ExtractPurpose(contextMessages, &toolCalls[0])
	}

	if reasoning := extractReasoning(contextMessages); reasoning != "" {
		return reasoning
	}
	var names []string
	for _, tc := range toolCalls {
		name := tc.Name
		if idx := strings.Index(name, "."); idx != -1 {
			name = name[idx+1:]
		}
		names = append(names, name)
	}
	return "Execute " + strings.Join(names, ", ")
}

// ExecuteToolsAndContinue executes detected tool calls and sends results back to LLM.
//
// Called from Update: permission checks and iteration state happen here. The
// returned Cmd only does I/O (tools, LLM) and reports the step in its result,
// which Update applies with ApplyToolStep.
func (m *Model) ExecuteToolsAndContinue(msg ToolCallsDetectedMsg) tea.Cmd {
	// Tool steps belong to the request that detected the tool calls
	reqID := msg.RequestID
	reqCtx := m.requestContext(reqID)
	if reqCtx == nil {
		return nil // Cancelled
	}

	// Check permissions BEFORE incrementing iteration
	// Only check if RequireApproval is enabled
	if m.Config.RequireApproval {
		for _, toolCall := range msg.ToolCalls {
			// Check if tool is whitelisted (global config or session-specific)
			if m.isToolAllowed(toolCall.Name) {
				continue
			}

			// Tool not allowed - request permission (don't increment iteration yet)
			purpose := m.ExtractPurpose(withResponse(msg.ContextMessages, msg.InitialResponse), &toolCall)
			config.Debugf("Permission required for tool: %s (purpose: %s)", toolCall.Name, purpose)

			// Carry the whole batch so approving this tool re-runs
			// all calls (asking about any other unapproved ones next)
			request := ToolPermissionRequestMsg{
				ToolName:        toolCall.Name,
				Purpose:         purpose,
				ToolCall:        toolCall,
				ToolCalls:       msg.ToolCalls,
				InitialResponse: msg.InitialResponse,
				ContextMessages: msg.ContextMessages,
				RequestID:       reqID,
			}
			return func() tea.Msg { return request }
		}
	}

	// Increment iteration AFTER permission check passes
	m.CurrentIteration++
	iteration := m.CurrentIteration
	maxReached := iteration >= m.MaxIterations

	config.Debugf("Starting iteration %d", iteration)

	// Everything the Cmd needs, captured now (the Cmd must not read m)
	mcpManager := m.MCPManager
	client := m.Provider
	hasSession := m.CurrentSession != nil
	enableMultiStep := m.Config.EnableMultiStep
	purpose := m.StepPurpose(msg.ContextMessages, msg.InitialResponse, msg.ToolCalls)

	// Disable tools if multi-step disabled OR max iterations reached
	disableNextTools := !enableMultiStep || maxReached
	timeouts := m.streamTimeouts()

	return func() tea.Msg {
		// Track step start time
		stepStartTime := time.Now()

		// Tools get 120s; the model's response streams without a total limit
		ctx, cancel := context.WithTimeout(reqCtx, 120*time.Second)
		defer cancel()

		config.Debugf("Executing %d tool calls", len(msg.ToolCalls))

		// Execute each tool call and collect results (one per call, in order)
		results := make([]string, len(msg.ToolCalls))

		for i, toolCall := range msg.ToolCalls {
			// Don't start tools once the request is cancelled
			if err := reqCtx.Err(); err != nil {
				return ToolExecutionErrorMsg{Err: err, RequestID: reqID}
			}

			config.Debugf("Executing tool call %d: %s", i+1, toolCall.Name)

			// ToolCall is already in provider-agnostic format (model.ToolCall)
			// Extract name and arguments directly
			toolName := toolCall.Name
			args := toolCall.Arguments

			// Execute tool via MCP manager
			result, err := mcpManager.ExecuteTool(ctx, toolName, args)
			if err != nil {
				config.Debugf("Error executing tool %s: %v", toolName, err)
				results[i] = fmt.Sprintf("Error executing %s: %v", toolName, err)
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

			config.Debugf("Tool %s result: %d chars", toolName, len(resultContent))

			results[i] = resultContent
		}

		// Build complete message history for LLM: the assistant turn with its
		// tool calls, then one result per call tied to the call's ID
		fullMessages := buildToolStepMessages(msg.ContextMessages, msg.InitialResponse, msg.ToolCalls, results)

		// Record step (all steps, not just tool executions).
		// Update adds it to IterationHistory (ApplyToolStep).
		var step *IterationStep
		if enableMultiStep {
			step = &IterationStep{
				StepNumber: iteration,
				Purpose:    purpose,
				StartTime:  stepStartTime,
				EndTime:    time.Now(),
				Success:    true,
			}
			step.Duration = step.EndTime.Sub(step.StartTime)

			// Store tool info (all tools of this step when there are several)
			if len(msg.ToolCalls) > 0 {
				step.ToolName = msg.ToolCalls[0].Name
				var shortNames []string
				for _, tc := range msg.ToolCalls {
					shortName := tc.Name
					if idx := strings.LastIndex(shortName, "."); idx != -1 {
						shortName = shortName[idx+1:]
					}
					shortNames = append(shortNames, shortName)
				}
				step.ShortName = strings.Join(shortNames, ", ")
			}

			config.Debugf("Recorded step %d: %s (%.1fs)",
				step.StepNumber, step.Purpose, step.Duration.Seconds())
		}

		// Get tools for next iteration
		var nextTools []mcptypes.Tool
		if mcpManager != nil && hasSession && !disableNextTools {
			ctx2, cancel2 := context.WithTimeout(reqCtx, 10*time.Second)
			defer cancel2()
			mcpTools, err := mcpManager.GetTools(ctx2)
			if err == nil {
				nextTools = mcpTools
			}
		}

		config.Debugf("Sending %d messages back to LLM (including %d tool results)", len(fullMessages), len(results))

		// Send back to LLM, streaming the response; the step ends with
		// ToolExecutionCompleteMsg
		stream := startChatStream(reqCtx, reqID, client, fullMessages, nextTools, timeouts,
			func(finalResponse string, detectedToolCalls []ToolCall, err error) tea.Msg {
				if err != nil {
					config.Debugf("Error getting response after tools: %v", err)
					return ToolExecutionErrorMsg{Err: err, RequestID: reqID}
				}

				if config.DebugEnabled() {
					config.Debugf("Post-tool response: %d chars, hasToolCalls=%v",
						len(finalResponse), len(detectedToolCalls) > 0)
					if len(finalResponse) > 0 {
						config.Debugf("Post-tool response content (first 200): %s", truncateRunes(finalResponse, 200))
					}
				}

				// Clean leaked tool calls from response (for both context and display)
				// This prevents leaked JSON/XML from polluting LLM context and user display
				if cleaned := cleanLeakedToolCalls(finalResponse); cleaned != finalResponse {
					config.Debugf("Cleaned leaked tool calls from response: %d -> %d chars",
						len(finalResponse), len(cleaned))
					finalResponse = cleaned
				}

				result := ToolExecutionCompleteMsg{
					FullResponse: finalResponse,
					Step:         step,
					MaxReached:   maxReached && len(detectedToolCalls) > 0,
					RequestID:    reqID,
				}

				// Complete (no more tool calls) or max iterations reached: Update
				// builds the summary (ApplyToolStep)
				if len(detectedToolCalls) == 0 || maxReached {
					if maxReached && len(detectedToolCalls) > 0 {
						config.Debugf("Max iterations (%d) reached", iteration)
					}
					return result
				}

				// Continue iteration
				config.Debugf("Next iteration will be %d: %d tool calls detected",
					iteration+1, len(detectedToolCalls))

				// Context for the next step. The LLM's current response becomes the
				// next step's assistant turn together with its tool calls (not a
				// separate turn), so it's passed as NextInitialResponse.
				result.HasMoreSteps = true
				result.NextToolCalls = ensureToolCallIDs(detectedToolCalls)
				result.NextContext = fullMessages
				result.NextInitialResponse = finalResponse
				return result
			})

		return ToolResultsReadyMsg{RequestID: reqID, Stream: stream}
	}
}

// ApplyToolStep records a finished tool step (from ToolExecutionCompleteMsg)
// in the iteration state. When the response is complete it fills in the
// step summary and resets the iteration state. Call from Update.
func (m *Model) ApplyToolStep(msg *ToolExecutionCompleteMsg) {
	if msg.Step != nil {
		m.IterationHistory = append(m.IterationHistory, *msg.Step)
	}
	if msg.HasMoreSteps {
		return
	}

	if len(m.IterationHistory) > 0 || msg.MaxReached {
		msg.IterationSummary = IterationSummaryMsg{
			Steps:      m.IterationHistory,
			TotalSteps: len(m.IterationHistory),
			MaxReached: msg.MaxReached,
		}
		config.Debugf("Multi-step complete: %d steps (max reached: %v)", len(m.IterationHistory), msg.MaxReached)
	}
	m.ResetIterations()
}

// ResetIterations clears the multi-step iteration state. Call from Update.
func (m *Model) ResetIterations() {
	m.CurrentIteration = 0
	m.IterationHistory = nil
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

// appendToolCalls adds newly reported tool calls to those already collected
// for a response. Exact duplicates (same name and arguments) are skipped:
// text-based tool call parsing can report the same call twice.
func appendToolCalls(existing, calls []ToolCall) []ToolCall {
	for _, call := range calls {
		duplicate := false
		for _, have := range existing {
			if sameToolCall(have, call) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			existing = append(existing, call)
		}
	}
	return existing
}

// sameToolCall reports whether two tool calls have the same name and arguments
func sameToolCall(a, b ToolCall) bool {
	if a.Name != b.Name {
		return false
	}
	argsA, errA := json.Marshal(a.Arguments)
	argsB, errB := json.Marshal(b.Arguments)
	return errA == nil && errB == nil && string(argsA) == string(argsB)
}

// ensureToolCallIDs gives every tool call an ID. Providers without call IDs
// (Ollama, tool calls parsed from text) get generated ones, which tie the
// call to its result in the next request.
func ensureToolCallIDs(calls []ToolCall) []ToolCall {
	for i := range calls {
		if calls[i].ID == "" {
			calls[i].ID = "call_" + uuid.NewString()[:8]
		}
	}
	return calls
}

// buildToolStepMessages appends a tool step to the context: the assistant
// turn (its text and the tool calls it made), then one tool result per call
// in the same order, tied to the call by ID
func buildToolStepMessages(context []Message, initialResponse string, calls []ToolCall, results []string) []Message {
	messages := append([]Message(nil), context...)
	messages = append(messages, Message{
		Role:      "assistant",
		Content:   initialResponse,
		ToolCalls: calls,
	})
	for i, call := range calls {
		result := ""
		if i < len(results) {
			result = results[i]
		}
		messages = append(messages, Message{
			Role:       "tool",
			Content:    result,
			ToolCallID: call.ID,
			ToolName:   call.Name,
		})
	}
	return messages
}

// withResponse returns the context with the LLM's response for this step
// appended, so purpose extraction can find its reasoning
func withResponse(context []Message, response string) []Message {
	if strings.TrimSpace(response) == "" {
		return context
	}
	return append(append([]Message(nil), context...), Message{Role: "assistant", Content: response})
}
