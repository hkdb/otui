package model

import (
	"time"

	"otui/ollama"
	"otui/storage"
)

// IterationStep records a single step in multi-step execution (Phase 2)
// Each step = one LLM response (may have 0+ tool calls)
type IterationStep struct {
	StepNumber int    // 1, 2, 3...
	Purpose    string // "Checking directory structure" (user-facing)
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration // Computed: EndTime - StartTime
	Success    bool
	ErrorMsg   string // If failed

	// Internal fields (not displayed to users)
	ToolName  string // e.g., "mcp-filesystem.read_dir"
	ShortName string // e.g., "read_dir"
}

// IterationSummaryMsg contains summary of all steps (Phase 2)
type IterationSummaryMsg struct {
	Steps      []IterationStep
	TotalSteps int
	MaxReached bool // True if max iterations reached (warning)
}

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
	Chunks           []string
	FullResponse     string
	IterationSummary IterationSummaryMsg // Phase 2

	// Phase 2: Multi-step continuation
	HasMoreSteps  bool       // Continue iteration after typewriter?
	NextToolCalls []ToolCall // Tools to execute in next step
	NextContext   []Message  // Context for next iteration
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

// ToolPermissionRequestMsg is sent when the model wants to execute a tool that needs approval
type ToolPermissionRequestMsg struct {
	ToolName        string
	Purpose         string
	ToolCall        ToolCall
	ContextMessages []Message
}

// ToolPermissionResponseMsg is sent when the user responds to a permission request
type ToolPermissionResponseMsg struct {
	Approved        bool
	AlwaysAllow     bool   // User chose "Always Allow" for this tool
	ToolName        string // Tool that was approved/denied
	ToolCall        ToolCall
	ContextMessages []Message
}
