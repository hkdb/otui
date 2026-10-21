package model

import (
	"time"

	"otui/ollama"
	"otui/storage"
)

// IterationStep records a single step in multi-step execution
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

// IterationSummaryMsg contains summary of all steps
type IterationSummaryMsg struct {
	Steps      []IterationStep
	TotalSteps int
	MaxReached bool // True if max iterations reached (warning)
}

// StreamChunkMsg carries new response text. Update shows it and calls
// WaitForStream(Stream) for the next part.
type StreamChunkMsg struct {
	Chunk     string
	RequestID uint64
	Stream    *ChatStream
}

// StreamDoneMsg ends a response without tool calls
type StreamDoneMsg struct {
	FullResponse string
	RequestID    uint64
}

// ToolResultsReadyMsg is sent when a step's tools have run and the model's
// follow-up response starts streaming (read it with WaitForStream(Stream)).
// The step ends with ToolExecutionCompleteMsg.
type ToolResultsReadyMsg struct {
	RequestID uint64
	Stream    *ChatStream
}

type StreamErrorMsg struct {
	Err       error
	RequestID uint64
}

// Tool execution messages
type ToolCallsDetectedMsg struct {
	ToolCalls       []ToolCall
	InitialResponse string
	ContextMessages []Message
	RequestID       uint64
}

type ToolExecutionCompleteMsg struct {
	FullResponse     string
	IterationSummary IterationSummaryMsg

	// Multi-step continuation
	HasMoreSteps        bool       // Continue with another step
	NextToolCalls       []ToolCall // Tools to execute in next step
	NextContext         []Message  // Context for next iteration
	NextInitialResponse string     // LLM text that came with NextToolCalls

	Step       *IterationStep // This step (multi-step enabled), added to the history by ApplyToolStep
	MaxReached bool           // Stopped at max iterations with tool calls left

	RequestID uint64
}

type ToolExecutionErrorMsg struct {
	Err       error
	RequestID uint64
}

type MarkdownRenderedMsg struct {
	MessageIndex      int
	Rendered          string
	SessionGeneration uint64
}

type ModelsListMsg struct {
	Models       []ollama.ModelInfo
	Err          error
	ShowSelector bool // Whether to auto-show model selector (user-initiated vs background fetch)

	// Model lists fetched by FetchAllModels, per provider (for StoreModelCache)
	Fetched map[string][]ollama.ModelInfo
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
	Session     *storage.Session
	Err         error
	Cancelled   bool
	Permissions ImportedPermissions // Removed from the imported session (see ImportSessionCmd)
}

// ImportedPermissions are the plugins and pre-approved tools an imported
// session file came with. They're removed on import; the user can keep them
// (RestoreImportedPermissionsCmd).
type ImportedPermissions struct {
	EnabledPlugins []string
	AllowedTools   []string
}

// Empty reports whether the session came without permissions
func (p ImportedPermissions) Empty() bool {
	return len(p.EnabledPlugins) == 0 && len(p.AllowedTools) == 0
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
	ToolCall        ToolCall   // The call that needs approval (shown to the user)
	ToolCalls       []ToolCall // All calls of this step, re-run after approval
	InitialResponse string     // Assistant text that came with the calls
	ContextMessages []Message
	RequestID       uint64
}

// ToolPermissionResponseMsg is sent when the user responds to a permission request
type ToolPermissionResponseMsg struct {
	Approved        bool
	AlwaysAllow     bool   // User chose "Always Allow" for this tool
	ToolName        string // Tool that was approved/denied
	ToolCall        ToolCall
	ToolCalls       []ToolCall // All calls of this step (see ToolPermissionRequestMsg)
	InitialResponse string
	ContextMessages []Message
	RequestID       uint64
}

// CompactionRequestMsg is sent when compaction approval is needed
type CompactionRequestMsg struct {
	MarkerIndex      int
	CurrentTokens    int
	ContextWindow    int
	MessageCount     int
	UserMessageCount int
	AssistantCount   int
}

// CompactionResponseMsg is sent when user responds to compaction request
type CompactionResponseMsg struct {
	Approved    bool
	MarkerIndex int
}

// CompactionCompleteMsg is sent when compaction finishes
type CompactionCompleteMsg struct {
	Success    bool
	Err        error
	Summary    string // wrapped: "Compacted N messages... - Summary: ..."
	LLMSummary string // raw LLM output (just the summary text)
	SessionID  string // Session the compaction was computed for
	Marker     int    // New compaction marker (user/assistant messages compacted)
}

// TokenUsageUpdatedMsg is sent when token usage is recalculated
type TokenUsageUpdatedMsg struct {
	Usage storage.TokenUsage
}

// CompactionWarningMsg is sent when context usage exceeds warning threshold
type CompactionWarningMsg struct {
	Percentage float64
}
