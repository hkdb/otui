package model

import (
	"otui/ollama"
	"otui/storage"
)

type StreamChunkMsg struct {
	Chunk string
}

type StreamDoneMsg struct {
	FullResponse string
}

type StreamErrorMsg struct {
	Err error
}

type StreamChunksCollectedMsg struct {
	Chunks       []string
	FullResponse string
}

type DisplayChunkTickMsg struct{}

// Tool execution messages (Phase 6)
type ToolCallsDetectedMsg struct {
	ToolCalls       []ToolCall
	InitialResponse string
	ContextMessages []Message
}

type ToolExecutionCompleteMsg struct {
	Chunks       []string
	FullResponse string
}

type ToolExecutionErrorMsg struct {
	Err error
}

type MarkdownRenderedMsg struct {
	MessageIndex int
	Rendered     string
}

type ModelsListMsg struct {
	Models       []ollama.ModelInfo
	Err          error
	ShowSelector bool // Whether to auto-show model selector (user-initiated vs background fetch)
}

type SessionsListMsg struct {
	Sessions []storage.SessionMetadata
	Err      error
}

type SessionLoadedMsg struct {
	Session *storage.Session
	Err     error
}

type SessionSavedMsg struct {
	Err error
}

type SessionRenamedMsg struct {
	Err error
}

type SessionExportedMsg struct {
	Path      string
	Err       error
	Cancelled bool
}

type SessionImportedMsg struct {
	Session   *storage.Session
	Err       error
	Cancelled bool
}

type ExportCleanupDoneMsg struct{}

type DataExportedMsg struct {
	Path      string
	Err       error
	Cancelled bool
}

type DataExportCleanupDoneMsg struct{}

type FlashTickMsg struct{}

type PluginOperationCompleteMsg struct {
	Operation string // "enable" or "disable"
	PluginID  string
	Success   bool
	Err       error
}

type PluginStartupCompleteMsg struct {
	Failures map[string]error // pluginID → error
}

type RegistryRefreshCompleteMsg struct {
	Success bool
	Err     error
}

type EditorContentMsg struct {
	Content string
}

type EditorErrorMsg struct {
	Err error
}

// ProviderPingMsg is sent when a provider ping completes (wizard setup)
type ProviderPingMsg struct {
	ProviderID string
	Valid      bool
	Err        error
}

// SingleProviderModelsMsg is sent when models are fetched from a single provider (wizard setup)
type SingleProviderModelsMsg struct {
	ProviderID string
	Models     []ollama.ModelInfo
	Err        error
}
