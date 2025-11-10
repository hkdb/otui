package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// ExecuteToolsAndContinue executes detected tool calls and sends results back to LLM
func (m *Model) ExecuteToolsAndContinue(msg ToolCallsDetectedMsg) tea.Cmd {
	mcpManager := m.MCPManager
	client := m.Provider

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		if config.DebugLog != nil {
			config.DebugLog.Printf("Executing %d tool calls", len(msg.ToolCalls))
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
		// 1. Original conversation context
		fullMessages := msg.ContextMessages

		// 2. Assistant's initial response (note: we can't include ToolCalls in Message struct currently)
		// This is okay because the provider will handle the tool call format internally
		fullMessages = append(fullMessages, Message{
			Role:    "assistant",
			Content: msg.InitialResponse,
		})

		// 3. Tool results
		fullMessages = append(fullMessages, toolResultMsgs...)

		if config.DebugLog != nil {
			config.DebugLog.Printf("Sending %d messages back to LLM (including %d tool results)", len(fullMessages), len(toolResultMsgs))
		}

		// Send back to LLM for final response (no tools this time to prevent recursion)
		var chunks []string
		var responseBuilder strings.Builder

		err := client.ChatWithTools(ctx, fullMessages, nil, func(chunk string, toolCalls []ToolCall) error {
			responseBuilder.WriteString(chunk)
			chunks = append(chunks, chunk)
			// Note: Ignoring tool calls in second phase to prevent infinite loops
			// Could be enhanced in future to support multi-step tool execution
			return nil
		})

		if err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Error getting final response: %v", err)
			}
			return ToolExecutionErrorMsg{Err: err}
		}

		finalResponse := responseBuilder.String()
		if config.DebugLog != nil {
			config.DebugLog.Printf("Final response received: %d chunks, %d chars", len(chunks), len(finalResponse))
		}

		return ToolExecutionCompleteMsg{
			Chunks:       chunks,
			FullResponse: finalResponse,
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
