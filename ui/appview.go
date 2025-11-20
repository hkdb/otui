package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	"otui/mcp"
	appmodel "otui/model"
	"otui/ollama"
	"otui/provider"
	"otui/storage"
)

type AppView struct {
	// Reference to core data model
	dataModel *appmodel.Model

	// UI Components
	viewport viewport.Model
	textarea textarea.Model

	// Window state
	width  int
	height int
	ready  bool

	// Streaming UI state
	currentResp *strings.Builder // Pointer to avoid copy panic
	showHelp    bool

	// Typewriter effect fields
	chunks     []string // Chunks to display with typewriter effect
	chunkIndex int      // Current chunk being displayed

	// Loading spinner (bubbles/spinner)
	loadingSpinner spinner.Model

	// Model selector
	showModelSelector            bool
	modelList                    []ollama.ModelInfo
	selectedModelIdx             int
	modelListCached              bool
	modelFilterMode              bool
	modelFilterInput             textinput.Model
	filteredModelList            []ollama.ModelInfo
	showToolWarningModal         bool
	pendingModelSwitch           string
	toolWarningPluginList        []string
	showSystemPromptToolWarning  bool
	systemPromptToolWarningShown bool

	// Session management UI
	showSessionManager   bool
	sessionList          []storage.SessionMetadata
	selectedSessionIdx   int
	sessionRenameMode    bool
	sessionRenameInput   textinput.Model
	sessionFilterMode    bool
	sessionFilterInput   textinput.Model
	filteredSessionList  []storage.SessionMetadata
	sessionExportMode    bool
	sessionExportInput   textinput.Model
	sessionExporting     bool
	sessionExportSuccess string // Contains export path if successful, empty otherwise

	// Import session state
	sessionImportPicker     FilePickerState
	sessionImportSuccess    *storage.Session
	sessionImportCancelCtx  context.Context
	sessionImportCancelFunc context.CancelFunc

	// Export state
	exportingSession bool
	exportSpinner    spinner.Model
	exportCancelCtx  context.Context
	exportCancelFunc context.CancelFunc
	exportTargetPath string
	exportCleaningUp bool

	// About modal
	showAbout bool

	// Settings modal
	showSettings            bool
	settingsFields          []SettingField
	selectedSettingIdx      int
	settingsEditMode        bool
	settingsEditInput       textinput.Model
	settingsHasChanges      bool
	settingsConfirmExit     bool
	settingsLoadedInfo      string
	settingsSaveError       string
	settingsDataDirNotFound bool   // Show confirmation for creating new data directory
	settingsNewDataDirPath  string // Path of new data directory to create

	// Provider Settings confirmation modal
	providerSettingsConfirmExit bool

	// Provider settings sub-screen
	providerSettingsState ProviderSettingsState

	// Data export state
	dataExportMode       bool
	dataExportInput      textinput.Model
	exportingDataDir     bool
	dataExportSpinner    spinner.Model
	dataExportCancelCtx  context.Context
	dataExportCancelFunc context.CancelFunc
	dataExportTargetPath string
	dataExportCleaningUp bool
	dataExportSuccess    string // Contains export path if successful, empty otherwise

	// Delete confirmation state
	confirmDeleteSession *storage.SessionMetadata

	// Info modal state (for simple notifications/errors)
	showInfoModal  bool
	infoModalTitle string
	infoModalMsg   string

	// Acknowledge modal (for warnings/errors requiring only acknowledgement)
	showAcknowledgeModal  bool
	acknowledgeModalTitle string
	acknowledgeModalMsg   string
	acknowledgeModalType  ModalType

	// SSH passphrase modal for data dir switch (uses shared modal helper)
	showPassphraseForDataDir bool
	passphraseForDataDir     textinput.Model
	passphraseSSHKeyPath     string
	passphraseError          string
	passphraseRetryDataDir   string // Data dir to retry after passphrase entered

	// New session modal
	showNewSessionModal      bool
	newSessionNameInput      textinput.Model
	newSessionPromptInput    textarea.Model
	newSessionFocusedField   int
	newSessionPluginIdx      int      // Selected plugin in the list
	newSessionEnabledPlugins []string // Enabled plugins for new session

	// Edit session modal (reuses newSession inputs)
	showEditSessionModal      bool
	editSessionID             string
	editSessionPluginIdx      int      // Selected plugin in the list
	editSessionEnabledPlugins []string // Temporary storage for enabled plugins during edit

	showMessageSearch      bool
	messageSearchInput     textinput.Model
	messageSearchResults   []storage.MessageMatch
	selectedSearchIdx      int
	messageSearchScrollIdx int

	showGlobalSearch      bool
	globalSearchInput     textinput.Model
	globalSearchResults   []storage.SessionMessageMatch
	selectedGlobalIdx     int
	globalSearchScrollIdx int

	highlightedMessageIdx     int
	highlightFlashCount       int
	pendingScrollToMessageIdx int

	showPluginManager  bool
	pluginManagerState PluginManagerState

	// Plugin system operation state (unified for app quit and Settings toggle)
	pluginSystemState PluginSystemState

	// Pending plugin operation callbacks (for actions after shutdown/startup completes)
	pendingPluginOperation     string      // "datadir_switch", "plugin_disable", "app_quit"
	pendingPluginOperationData interface{} // Optional data for callback (e.g., newDataDir string)

	// RestartAfterQuit indicates OTUI should restart after quit completes
	// Used for creating new data directories from Settings
	RestartAfterQuit bool

	// Tool execution state (Phase 6)
	executingTool        string        // Plugin name currently executing (e.g., "mcp-searxng")
	toolExecutionSpinner spinner.Model // Spinner for tool execution indicator

	// Permission system state (Phase 1: Permission System)
	waitingForPermission    bool
	pendingPermission       *appmodel.ToolPermissionRequestMsg
	temporarilyAllowedTools []string // Tools approved once - removed after execution

	// Plugin operation modal (enable/disable feedback for individual plugins in Plugin Manager)
	showPluginOperationModal bool
	pluginOperationPhase     string // "enabling", "disabling", "complete", "error"
	pluginOperationName      string // Plugin display name
	pluginOperationError     string // Error message if failed
	pluginOperationSpinner   spinner.Model

	// Multi-step iteration UI state (Phase 2)
	iterationCount     int  // Current step number
	maxIterations      int  // Max from config
	pendingNextStep    bool // Continue after typewriter?
	pendingToolCalls   []ToolCall
	pendingToolContext []Message
	pendingSummary     *IterationSummaryMsg // Summary to add after typewriter completes
}

// PluginSystemState is an alias to appmodel.PluginSystemState for backward compatibility
type PluginSystemState = appmodel.PluginSystemState

func NewAppView(cfg *config.Config, sessionStorage *storage.SessionStorage, lastSession *storage.Session, version, license string) AppView {
	// Initialize MCP components (allow graceful degradation - Phase 4)
	registry, err := mcp.NewRegistry(cfg.DataDir())
	if err != nil {
		// Don't panic - plugins will be disabled
		if config.DebugLog != nil {
			config.DebugLog.Printf("[AppView] Plugin registry init failed: %v (plugins disabled)", err)
		}
		registry = nil
	}

	var pluginStorage *storage.PluginStorage
	if registry != nil {
		pluginStorage, err = storage.NewPluginStorage(cfg.DataDir())
		if err != nil {
			// Don't panic - plugins will be disabled
			if config.DebugLog != nil {
				config.DebugLog.Printf("[AppView] Plugin storage init failed: %v (plugins disabled)", err)
			}
			registry = nil
			pluginStorage = nil
		}
	}

	var pluginsConfig *config.PluginsConfig
	if registry != nil {
		pluginsConfig, err = config.LoadPluginsConfig(cfg.DataDir())
		if err != nil {
			// Don't panic - plugins will be disabled
			if config.DebugLog != nil {
				config.DebugLog.Printf("[AppView] Plugins config load failed: %v (plugins disabled)", err)
			}
			registry = nil
			pluginStorage = nil
			pluginsConfig = nil
		}
	}

	var installer *mcp.Installer
	var mcpManager *mcp.MCPManager

	// Only create MCP manager if all plugin components initialized successfully
	if registry != nil && pluginStorage != nil && pluginsConfig != nil {
		installer = mcp.NewInstaller(pluginStorage, pluginsConfig, cfg.DataDir())
		mcpManager = mcp.NewMCPManager(cfg, pluginStorage, pluginsConfig, registry, cfg.DataDir())
		// Plugins will be started asynchronously in Init() only if cfg.PluginsEnabled == true
	}

	ta := textarea.New()
	ta.Placeholder = "Type your message here or press Alt+I to use your favorite text editor..."
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.SetWidth(80)

	// Custom KeyMap: Alt+Enter for newline, Enter alone does nothing (handled separately)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))

	// Set dynamic prompt: "> " for first line, "| " for subsequent lines
	ta.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "> "
		}
		return "| "
	})

	vp := viewport.New(0, 0)

	sessionImportPicker := NewFilePickerState(FilePickerConfig{
		Title:          "Import Session",
		Mode:           FilePickerModeOpen,
		AllowedTypes:   []string{".json"},
		StartDirectory: "",
		ShowHidden:     true,
		OperationType:  "Import",
	})

	sessionFilterInput := textinput.New()
	sessionFilterInput.Prompt = "Filter: "
	sessionFilterInput.CharLimit = 64

	modelFilterInput := textinput.New()
	modelFilterInput.Prompt = "Filter: "
	modelFilterInput.CharLimit = 64

	messageSearchInput := textinput.New()
	messageSearchInput.Prompt = "Search: "
	messageSearchInput.CharLimit = 100

	globalSearchInput := textinput.New()
	globalSearchInput.Prompt = "Search all: "
	globalSearchInput.CharLimit = 100

	// Initialize passphrase input for data dir switch (reuses shared helper)
	passphraseForDataDir := NewPassphraseInput("Enter passphrase for SSH key")

	// Create plugin state
	pluginState := &appmodel.PluginState{
		Registry:  registry,
		Installer: installer,
		Config:    pluginsConfig,
	}

	// Create search index
	searchIndex := storage.NewSearchIndex(sessionStorage)

	// Initialize ALL providers (Ollama + cloud providers) via provider package
	// Provider package owns provider lifecycle - zero business logic in UI
	allProviders := provider.InitializeProviders(cfg)

	// Determine which provider to use for initialization
	// Priority: session's provider → configured default → ollama fallback
	// This ensures sessions with non-default providers load correctly on app restart
	sessionProvider := cfg.DefaultProvider
	if lastSession != nil && lastSession.Provider != "" {
		sessionProvider = lastSession.Provider
	}

	initialProvider := allProviders[sessionProvider]
	if initialProvider == nil {
		// Session's provider not available - fall back to configured default
		initialProvider = allProviders[cfg.DefaultProvider]
		if initialProvider == nil {
			// Default provider not available - fall back to Ollama
			initialProvider = allProviders["ollama"]
		}
	}

	// Initialize the core data model with correct provider (session's or default)
	// This fixes bug where non-default provider sessions would use wrong provider on restart
	dataModel := appmodel.NewModel(cfg, initialProvider, sessionStorage, lastSession, pluginState, mcpManager, searchIndex, version, license)

	// Set ALL providers on the model (multi-provider support)
	dataModel.Providers = allProviders

	// Create initial session if none exists (e.g., after welcome wizard)
	if lastSession == nil {
		newSession, _ := dataModel.CreateAndSaveNewSession("New Session", "", []string{})
		dataModel.CurrentSession = newSession
		if mcpManager != nil {
			mcpManager.SetSession(newSession)
		}
	}

	return AppView{
		dataModel:                    dataModel,
		textarea:                     ta,
		viewport:                     vp,
		currentResp:                  &strings.Builder{},
		ready:                        false,
		showHelp:                     false,
		showAbout:                    false,
		sessionImportPicker:          sessionImportPicker,
		sessionFilterMode:            false,
		sessionFilterInput:           sessionFilterInput,
		filteredSessionList:          []storage.SessionMetadata{},
		modelFilterMode:              false,
		modelFilterInput:             modelFilterInput,
		filteredModelList:            []ollama.ModelInfo{},
		showToolWarningModal:         false,
		pendingModelSwitch:           "",
		toolWarningPluginList:        nil,
		showSystemPromptToolWarning:  false,
		systemPromptToolWarningShown: false,
		messageSearchInput:           messageSearchInput,
		globalSearchInput:            globalSearchInput,
		highlightedMessageIdx:        -1,
		pendingScrollToMessageIdx:    -1,
		passphraseForDataDir:         passphraseForDataDir,
		showPluginManager:            false,
		pluginManagerState: PluginManagerState{
			pluginState: pluginState,
			selection: SelectionState{
				viewMode: "installed",
			},
		},
	}
}

func (a AppView) Init() tea.Cmd {
	// Don't render markdown here - wait for WindowSizeMsg to get correct width
	// The needsInitialRender flag is set in NewModel() if messages were loaded

	cmds := []tea.Cmd{
		textarea.Blink,
		a.dataModel.FetchAllModels(false), // Background fetch on startup, don't show selector
	}

	// Start all enabled plugins asynchronously
	if a.dataModel.MCPManager != nil {
		cmds = append(cmds, a.dataModel.StartAllPlugins())
	}

	return tea.Batch(cmds...)
}

func (a AppView) View() string {
	if !a.ready {
		return "Loading OTUI..."
	}

	// Modal rendering order (top to bottom layers):
	// 0. Plugin system operation modal (absolute highest priority - startup/shutdown)
	// 1. Help (always on top - can peek while in other modals)
	// 2. Model selector
	// 3. Settings
	// 4. Session manager
	// 5. Search modals
	// 6. About

	// Show plugin system modal if active (absolute highest priority - starting/stopping plugins)
	if a.pluginSystemState.Active {
		return RenderPluginSystemModal(
			a.pluginSystemState,
			a.width,
			a.height,
		)
	}

	// Show plugin operation modal if active (high priority - plugin enable/disable feedback)
	if a.showPluginOperationModal {
		return RenderPluginOperationModal(
			a.pluginOperationPhase,
			a.pluginOperationSpinner.View(),
			a.pluginOperationName,
			a.pluginOperationError,
			a.width,
			a.height,
		)
	}

	// Show passphrase modal for data dir switch (uses shared modal helper)
	if a.showPassphraseForDataDir {
		return a.renderPassphraseForDataDirModal()
	}

	// Show info modal if active (highest priority)
	if a.showInfoModal {
		return RenderConfirmationModal(ConfirmationState{
			Active:  true,
			Title:   a.infoModalTitle,
			Message: a.infoModalMsg,
		}, a.width, a.height)
	}

	// Show acknowledge modal if active (warnings/errors requiring only acknowledgement)
	if a.showAcknowledgeModal {
		return RenderAcknowledgeModal(
			a.acknowledgeModalTitle,
			a.acknowledgeModalMsg,
			a.acknowledgeModalType,
			a.width,
			a.height,
		)
	}

	// Show help modal if toggled (top layer - can appear over other modals)
	if a.showHelp {
		return renderHelpModal(a.width, a.height)
	}

	// Show model selector if toggled
	if a.showModelSelector {
		multiProvider := len(a.dataModel.Providers) > 1
		return renderModelSelector(a.modelList, a.selectedModelIdx, a.dataModel.Provider.GetModel(), a.modelFilterMode, a.modelFilterInput, a.filteredModelList, multiProvider, a.width, a.height)
	}

	// Show settings modal if toggled
	if a.showSettings {
		// Check if provider settings sub-screen is visible
		if a.providerSettingsState.visible {
			// Check for confirmation modal first
			if a.providerSettingsConfirmExit {
				return RenderUnsavedChangesModal(a.width, a.height)
			}
			return a.renderProviderSettings(a.width, a.height)
		}
		return renderSettings(a.settingsFields, a.selectedSettingIdx, a.settingsEditMode, a.settingsEditInput, a.settingsHasChanges, a.settingsConfirmExit, a.settingsLoadedInfo, a.settingsSaveError, a.dataExportMode, a.dataExportInput, a.exportingDataDir, a.dataExportCleaningUp, a.dataExportSpinner, a.dataExportSuccess, a.settingsDataDirNotFound, a.settingsNewDataDirPath, a.width, a.height)
	}

	// Show new session modal (must be before session manager)
	if a.showNewSessionModal {
		// Gather available plugins (Layer 2 enabled) - same as Edit Session modal
		var availablePlugins []mcp.Plugin
		var unavailablePluginIDs []string

		if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
			allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()

			// Find available plugins (enabled in Layer 2)
			for _, p := range allPlugins {
				if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
					availablePlugins = append(availablePlugins, p)
				}
			}

			// Find unavailable plugins (in session but not available)
			for _, pluginID := range a.newSessionEnabledPlugins {
				found := false
				for _, p := range availablePlugins {
					if p.ID == pluginID {
						found = true
						break
					}
				}
				if !found {
					unavailablePluginIDs = append(unavailablePluginIDs, pluginID)
				}
			}
		}

		return renderSessionModal("New session", a.newSessionNameInput, a.newSessionPromptInput, a.newSessionFocusedField, a.width, a.height, availablePlugins, a.newSessionEnabledPlugins, unavailablePluginIDs, a.newSessionPluginIdx)
	}

	// Show edit session modal (must be before session manager)
	if a.showEditSessionModal {
		// Gather available plugins (Layer 2 enabled)
		var availablePlugins []mcp.Plugin
		var unavailablePluginIDs []string

		if a.pluginManagerState.pluginState.Config != nil && a.pluginManagerState.pluginState.Registry != nil {
			allPlugins := a.pluginManagerState.pluginState.Registry.GetAll()

			// Find available plugins (enabled in Layer 2)
			for _, p := range allPlugins {
				if a.pluginManagerState.pluginState.Config.GetPluginEnabled(p.ID) {
					availablePlugins = append(availablePlugins, p)
				}
			}

			// Find unavailable plugins (in session but not available)
			for _, pluginID := range a.editSessionEnabledPlugins {
				found := false
				for _, p := range availablePlugins {
					if p.ID == pluginID {
						found = true
						break
					}
				}
				if !found {
					unavailablePluginIDs = append(unavailablePluginIDs, pluginID)
				}
			}
		}

		return renderSessionModal("Edit session", a.newSessionNameInput, a.newSessionPromptInput, a.newSessionFocusedField, a.width, a.height, availablePlugins, a.editSessionEnabledPlugins, unavailablePluginIDs, a.editSessionPluginIdx)
	}

	// Show session manager if toggled
	if a.showSessionManager {
		currentSessionID := ""
		if a.dataModel.CurrentSession != nil {
			currentSessionID = a.dataModel.CurrentSession.ID
		}
		return renderSessionManager(a.sessionList, a.selectedSessionIdx, currentSessionID, a.sessionRenameMode, a.sessionRenameInput, a.sessionExportMode, a.sessionExportInput, a.exportingSession, a.exportCleaningUp, a.exportSpinner, a.sessionExportSuccess, a.sessionImportPicker, a.sessionImportSuccess, a.confirmDeleteSession, a.sessionFilterMode, a.sessionFilterInput, a.filteredSessionList, a.width, a.height)
	}

	// Show plugin manager if toggled
	if a.showPluginManager {
		return a.renderPluginManager()
	}

	if a.showGlobalSearch {
		return renderGlobalSearch(a.globalSearchInput, a.globalSearchResults, a.selectedGlobalIdx, a.globalSearchScrollIdx, a.width, a.height)
	}

	if a.showMessageSearch {
		return renderMessageSearch(a.messageSearchInput, a.messageSearchResults, a.selectedSearchIdx, a.messageSearchScrollIdx, a.width, a.height)
	}

	// Show about modal if toggled
	if a.showAbout {
		return renderAboutModal(a.width, a.height, a.dataModel.Version, a.dataModel.License)
	}

	// Show tool warning modal if triggered
	if a.showToolWarningModal {
		return RenderToolWarningModal(a.pendingModelSwitch, a.toolWarningPluginList, a.width, a.height)
	}

	// Show system prompt + tools warning modal if triggered
	if a.showSystemPromptToolWarning {
		// Get enabled plugins for current session
		var enabledPluginNames []string
		if a.dataModel.MCPManager != nil && a.dataModel.CurrentSession != nil {
			enabledPluginNames = a.dataModel.MCPManager.GetSessionEnabledPluginNames(a.dataModel.CurrentSession)
		}

		// Build message lines
		messageLines := buildSystemPromptToolWarningLines(
			a.dataModel.CurrentSession.SystemPrompt,
			enabledPluginNames,
			70, // modal width
		)

		// Format footer
		footer := FormatFooter("Enter", "Send Anyway", "Esc", "Cancel")

		return RenderThreeSectionModal(
			"⚠  System Prompts May Break Tool Usage",
			messageLines,
			footer,
			ModalTypeWarning,
			70, // desired width
			a.width,
			a.height,
		)
	}

	// Title bar - "OTUI - Model - Session Name | 🔌 plugins"
	otuiText := AssistantStyle.Render("OTUI")
	modelText := TitleStyle.Render(fmt.Sprintf(" - %s", a.dataModel.Provider.GetDisplayName()))
	sessionName := "New Session"
	if a.dataModel.CurrentSession != nil && a.dataModel.CurrentSession.Name != "" {
		sessionName = a.dataModel.CurrentSession.Name
	}
	sessionText := UserStyle.Render(fmt.Sprintf(" - %s", sessionName))

	// Add plugin indicator for session's enabled plugins
	pluginText := ""
	if a.dataModel.MCPManager != nil && a.dataModel.CurrentSession != nil {
		pluginNames := a.dataModel.MCPManager.GetSessionEnabledPluginNames(a.dataModel.CurrentSession)
		if len(pluginNames) > 0 {
			// Check if any plugins are unavailable
			hasUnavailable := a.dataModel.MCPManager.HasUnavailableSessionPlugins(a.dataModel.CurrentSession)

			var pluginIndicator string
			if hasUnavailable {
				pluginIndicator = " | ⚠️ " // Warning: some plugins unavailable
			} else {
				pluginIndicator = " | 🔌 " // All plugins available
			}

			if len(pluginNames) == 1 {
				pluginIndicator += pluginNames[0]
			} else if len(pluginNames) == 2 {
				pluginIndicator += pluginNames[0] + ", " + pluginNames[1]
			} else {
				// 3+ plugins: show first 2 and count
				pluginIndicator += pluginNames[0] + ", " + pluginNames[1] + fmt.Sprintf(", ... (%d)", len(pluginNames))
			}
			pluginText = DimStyle.Render(pluginIndicator)
		}
	}

	title := otuiText + modelText + sessionText + pluginText

	// Add tool execution indicator (Phase 6)
	if a.executingTool != "" {
		toolIndicator := fmt.Sprintf(" | 🔧: %s %s", a.executingTool, a.toolExecutionSpinner.View())
		title += TitleStyle.Render(toolIndicator)
	}

	// Separator with bottom margin for header (empty line forces spacing)
	separator := ""

	// Viewport with messages
	viewportView := a.viewport.View()

	// Input area
	inputView := a.textarea.View()

	// Status bar with bold user green descriptions (main chat uses user green)
	descStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
	statusBar := fmt.Sprintf("Alt+Q %s  Alt+E %s  Alt+S %s  Alt+M %s  Alt+F %s  Alt+Enter %s  Enter %s  Alt+Y %s",
		descStyle.Render("Quit"),
		descStyle.Render("Edit session"),
		descStyle.Render("Sessions"),
		descStyle.Render("Models"),
		descStyle.Render("Search"),
		descStyle.Render("New Line"),
		descStyle.Render("Send"),
		descStyle.Render("Copy"),
	)
	statusBar = StatusStyle.Render(statusBar)

	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		separator,
		viewportView,
		inputView,
		statusBar,
	)
}

func (a AppView) getSessionList() []storage.SessionMetadata {
	if a.sessionFilterMode && len(a.filteredSessionList) > 0 {
		return a.filteredSessionList
	}
	return a.sessionList
}

func (a AppView) getModelList() []ollama.ModelInfo {
	if a.modelFilterMode && len(a.filteredModelList) > 0 {
		return a.filteredModelList
	}
	return a.modelList
}

// setCurrentSession sets the current session and syncs it with the MCP manager
// This ensures plugin isolation and prevents security holes from stale session references
func (a *AppView) setCurrentSession(session *storage.Session) {
	a.dataModel.CurrentSession = session
	if a.dataModel.MCPManager != nil {
		a.dataModel.MCPManager.SetSession(session)
	}
	// Reset system prompt tool warning flag when switching sessions
	a.systemPromptToolWarningShown = false
}

func (a *AppView) closeAllModals() {
	a.showInfoModal = false
	a.showHelp = false
	a.showSessionManager = false
	a.showModelSelector = false
	a.showToolWarningModal = false
	a.showMessageSearch = false
	a.showGlobalSearch = false
	a.showSettings = false
	a.showAbout = false
	a.showPluginManager = false

	a.sessionRenameMode = false
	a.sessionExportMode = false
	a.sessionFilterMode = false
	a.confirmDeleteSession = nil
	a.sessionImportPicker.Active = false
	a.pluginManagerState.confirmations.deletePlugin = nil

	a.modelFilterMode = false

	a.settingsEditMode = false
	a.settingsConfirmExit = false

	a.dataExportMode = false

	if a.sessionRenameInput.Focused() {
		a.sessionRenameInput.Blur()
	}
	if a.sessionExportInput.Focused() {
		a.sessionExportInput.Blur()
	}
	if a.sessionFilterInput.Focused() {
		a.sessionFilterInput.Blur()
	}
	if a.modelFilterInput.Focused() {
		a.modelFilterInput.Blur()
	}
	if a.messageSearchInput.Focused() {
		a.messageSearchInput.Blur()
	}
	if a.globalSearchInput.Focused() {
		a.globalSearchInput.Blur()
	}
	if a.settingsEditInput.Focused() {
		a.settingsEditInput.Blur()
	}
	if a.dataExportInput.Focused() {
		a.dataExportInput.Blur()
	}
}

func (a AppView) renderPassphraseForDataDirModal() string {
	return RenderPassphraseModal(
		"SSH Key Passphrase Required",
		a.passphraseSSHKeyPath,
		a.passphraseForDataDir,
		a.passphraseError,
		a.width,
		a.height,
	)
}

// UnlockCurrentDataDir unlocks the OTUI instance in the current data directory.
// This is called on application exit to ensure the instance lock is released.
// Safe to call multiple times or with nil storage (returns nil).
//
// Used by main.go defer to unlock the CURRENT data directory, which may differ
// from the initial data directory if the user switched directories during the session.
func (a *AppView) UnlockCurrentDataDir() error {
	if a.dataModel == nil || a.dataModel.SessionStorage == nil {
		return nil
	}
	return a.dataModel.SessionStorage.UnlockOTUIInstance()
}
