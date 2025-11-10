package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"otui/config"
	"otui/mcp"
	"otui/ollama"
	"otui/storage"
)

// FetchSessionList retrieves the list of saved sessions
func (m *Model) FetchSessionList() tea.Cmd {
	if m.SessionStorage == nil {
		return nil
	}
	storage := m.SessionStorage
	return func() tea.Msg {
		sessions, err := storage.List()
		return SessionsListMsg{
			Sessions: sessions,
			Err:      err,
		}
	}
}

// LoadSession loads a session by ID
func (m *Model) LoadSession(sessionID string) tea.Cmd {
	if m.SessionStorage == nil {
		return nil
	}

	// Skip if reloading current session (no need to check our own lock)
	if m.CurrentSession != nil && m.CurrentSession.ID == sessionID {
		// Already loaded, just close session manager
		return func() tea.Msg {
			return SessionLoadedMsg{
				Session: m.CurrentSession,
				Err:     nil,
			}
		}
	}

	storage := m.SessionStorage
	return func() tea.Msg {
		// Check if session is locked by another OTUI instance
		isLocked, err := storage.CheckSessionLock(sessionID)
		if err != nil {
			return SessionLoadedMsg{Session: nil, Err: err}
		}
		if isLocked {
			return SessionLoadedMsg{Session: nil, Err: fmt.Errorf("session_locked")}
		}

		session, err := storage.Load(sessionID)
		if err != nil {
			return SessionLoadedMsg{Session: nil, Err: err}
		}

		// Create lock file for this session
		_ = storage.LockSession(sessionID)

		return SessionLoadedMsg{
			Session: session,
			Err:     err,
		}
	}
}

// SaveCurrentSession saves the current session to storage
func (m *Model) SaveCurrentSession() tea.Cmd {
	if m.SessionStorage == nil || m.CurrentSession == nil {
		return nil
	}

	// Convert UI messages to storage messages
	var sessionMessages []storage.Message
	for _, msg := range m.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			sessionMessages = append(sessionMessages, storage.Message{
				Role:      msg.Role,
				Content:   msg.Content,
				Rendered:  msg.Rendered,
				Timestamp: msg.Timestamp,
			})
		}
	}

	m.CurrentSession.Messages = sessionMessages
	m.CurrentSession.UpdatedAt = time.Now()
	m.CurrentSession.Model = m.Provider.GetModel()

	session := m.CurrentSession
	storage := m.SessionStorage

	return func() tea.Msg {
		err := storage.Save(session)
		if err == nil {
			// Save as current session ID
			storage.SaveCurrentSessionID(session.ID)
		}
		return SessionSavedMsg{Err: err}
	}
}

// AutoSaveSession automatically saves the current session with an auto-generated name if needed
func (m *Model) AutoSaveSession() tea.Cmd {
	if m.SessionStorage == nil {
		return nil
	}

	// Create new session if none exists (fallback - should rarely happen now)
	if m.CurrentSession == nil {
		// Generate name from first user message
		var firstUserMsg string
		for _, msg := range m.Messages {
			if msg.Role == "user" {
				firstUserMsg = msg.Content
				break
			}
		}

		m.CurrentSession = &storage.Session{
			ID:             "", // Let Save() generate UUID
			Name:           storage.GenerateSessionName(firstUserMsg),
			Model:          m.Config.DefaultModel,    // Use configured default model
			Provider:       m.Config.DefaultProvider, // Use configured default provider
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			EnabledPlugins: []string{},
			SystemPrompt:   "",
		}

		// Switch active provider to match session
		m.SwitchToDefaultProvider()

		// Sync with MCP manager (security fix)
		if m.MCPManager != nil {
			m.MCPManager.SetSession(m.CurrentSession)
		}
	} else if m.CurrentSession.Name == "New Session" && len(m.Messages) > 0 {
		// Auto-rename if still "New Session" and has messages
		var firstUserMsg string
		for _, msg := range m.Messages {
			if msg.Role == "user" {
				firstUserMsg = msg.Content
				break
			}
		}

		if firstUserMsg != "" {
			m.CurrentSession.Name = storage.GenerateSessionName(firstUserMsg)
		}
	}

	return m.SaveCurrentSession()
}

// RenameSessionCmd renames a session and refreshes the session list
func (m *Model) RenameSessionCmd(sessionID, newName string) tea.Cmd {
	return func() tea.Msg {
		if m.SessionStorage == nil {
			return SessionRenamedMsg{Err: fmt.Errorf("session storage not initialized")}
		}

		if err := m.SessionStorage.RenameSession(sessionID, newName); err != nil {
			return SessionRenamedMsg{Err: err}
		}

		sessions, err := m.SessionStorage.List()
		if err != nil {
			return SessionRenamedMsg{Err: err}
		}

		return SessionsListMsg{Sessions: sessions, Err: nil}
	}
}

// ExportSessionCmd exports a session to a JSON file
func (m *Model) ExportSessionCmd(ctx context.Context, sessionID, exportPath string) tea.Cmd {
	return func() tea.Msg {
		// Cancellation point 1: Before loading
		select {
		case <-ctx.Done():
			return SessionExportedMsg{Cancelled: true}
		default:
		}

		if m.SessionStorage == nil {
			return SessionExportedMsg{Err: fmt.Errorf("session storage not initialized")}
		}

		// Load session
		session, err := m.SessionStorage.Load(sessionID)
		if err != nil {
			return SessionExportedMsg{Err: err}
		}

		// Cancellation point 2: Before marshaling
		select {
		case <-ctx.Done():
			return SessionExportedMsg{Cancelled: true}
		default:
		}

		// Marshal JSON (potentially slow for large sessions)
		data, err := json.MarshalIndent(session, "", "  ")
		if err != nil {
			return SessionExportedMsg{Err: err}
		}

		// Cancellation point 3: Before creating directory
		select {
		case <-ctx.Done():
			return SessionExportedMsg{Cancelled: true}
		default:
		}

		// Ensure directory exists (0700 - user-only access)
		dir := filepath.Dir(exportPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return SessionExportedMsg{Err: err}
		}

		// Cancellation point 4: Before writing file
		select {
		case <-ctx.Done():
			return SessionExportedMsg{Cancelled: true}
		default:
		}

		// Write file (0600 - session exports contain sensitive conversation data)
		if err := os.WriteFile(exportPath, data, 0600); err != nil {
			return SessionExportedMsg{Err: err}
		}

		return SessionExportedMsg{Path: exportPath}
	}
}

// ImportSessionCmd imports a session from a JSON file
func (m *Model) ImportSessionCmd(ctx context.Context, filePath string) tea.Cmd {
	return func() tea.Msg {
		// Cancellation point 1: Start
		select {
		case <-ctx.Done():
			return SessionImportedMsg{Cancelled: true}
		default:
		}

		if m.SessionStorage == nil {
			return SessionImportedMsg{Err: fmt.Errorf("session storage not initialized")}
		}

		// Expand path
		expandedPath := config.ExpandPath(filePath)

		// Read JSON file
		data, err := os.ReadFile(expandedPath)
		if err != nil {
			return SessionImportedMsg{Err: fmt.Errorf("failed to read file: %w", err)}
		}

		// Cancellation point 2: After read
		select {
		case <-ctx.Done():
			return SessionImportedMsg{Cancelled: true}
		default:
		}

		// Parse JSON
		var session storage.Session
		if err := json.Unmarshal(data, &session); err != nil {
			return SessionImportedMsg{Err: fmt.Errorf("invalid session file: %w", err)}
		}

		// Validate required fields
		if session.Name == "" {
			return SessionImportedMsg{Err: fmt.Errorf("invalid Session: missing name")}
		}
		if len(session.Messages) == 0 {
			return SessionImportedMsg{Err: fmt.Errorf("invalid Session: no messages")}
		}

		// Generate new UUID and timestamps
		session.ID = uuid.New().String()
		session.CreatedAt = time.Now()
		session.UpdatedAt = time.Now()

		// Cancellation point 3: Before save
		select {
		case <-ctx.Done():
			return SessionImportedMsg{Cancelled: true}
		default:
		}

		// Save to storage
		if err := m.SessionStorage.Save(&session); err != nil {
			return SessionImportedMsg{Err: fmt.Errorf("failed to save Session: %w", err)}
		}

		return SessionImportedMsg{Session: &session}
	}
}

// CleanupPartialFileCmd removes a partial export file
func (m *Model) CleanupPartialFileCmd(filePath string) tea.Cmd {
	return func() tea.Msg {
		// Delete the partial file
		if err := os.Remove(filePath); err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Failed to cleanup partial file: %v", err)
			}
		}
		return ExportCleanupDoneMsg{}
	}
}

// CleanupPartialDataExportCmd removes a partial data export file
func (m *Model) CleanupPartialDataExportCmd(filePath string) tea.Cmd {
	return func() tea.Msg {
		// Delete the partial data export file
		if err := os.Remove(filePath); err != nil {
			if config.DebugLog != nil {
				config.DebugLog.Printf("Failed to cleanup partial data export: %v", err)
			}
		}
		return DataExportCleanupDoneMsg{}
	}
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// UpdateSessionPropertiesCmd updates session properties and refreshes the session list
func (m *Model) UpdateSessionPropertiesCmd(sessionID, newName, newSystemPrompt string, enabledPlugins []string) tea.Cmd {
	return func() tea.Msg {
		if m.SessionStorage == nil {
			return SessionsListMsg{Err: fmt.Errorf("session storage not initialized")}
		}

		// Load full session
		session, err := m.SessionStorage.Load(sessionID)
		if err != nil {
			return SessionsListMsg{Err: err}
		}

		// Update properties
		session.Name = newName
		session.SystemPrompt = newSystemPrompt
		session.EnabledPlugins = enabledPlugins

		// Save back
		if err := m.SessionStorage.Save(session); err != nil {
			return SessionsListMsg{Err: err}
		}

		// Update in-memory current session if it's the one being edited
		if m.CurrentSession != nil && m.CurrentSession.ID == sessionID {
			m.CurrentSession.Name = newName
			m.CurrentSession.SystemPrompt = newSystemPrompt
			m.CurrentSession.EnabledPlugins = enabledPlugins
		}

		// Refresh list
		sessions, err := m.SessionStorage.List()
		if err != nil {
			return SessionsListMsg{Err: err}
		}

		return SessionsListMsg{Sessions: sessions, Err: nil}
	}
}

// CreateAndSaveNewSession creates a new session with the given properties and saves it to storage.
// This is the shared implementation used by both Alt+N (main screen) and "n" key (session manager).
// Returns the created session or an error if save fails.
func (m *Model) CreateAndSaveNewSession(name, systemPrompt string, enabledPlugins []string) (*storage.Session, error) {
	// Use "New Session" as default if name is empty
	if name == "" {
		name = "New Session"
	}

	newSession := &storage.Session{
		ID:             "", // Let Save() generate UUID automatically
		Name:           name,
		Model:          m.Config.DefaultModel,    // Use configured default model
		Provider:       m.Config.DefaultProvider, // Use configured default provider
		Messages:       []storage.Message{},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		EnabledPlugins: enabledPlugins,
		SystemPrompt:   systemPrompt,
	}

	// Switch active provider to match session
	m.SwitchToDefaultProvider()

	// Save to storage (generates ID automatically)
	if m.SessionStorage != nil {
		if err := m.SessionStorage.Save(newSession); err != nil {
			return nil, fmt.Errorf("failed to save new Session: %w", err)
		}
		if err := m.SessionStorage.SaveCurrentSessionID(newSession.ID); err != nil {
			return nil, fmt.Errorf("failed to save current session ID: %w", err)
		}
	}

	return newSession, nil
}

// ApplyDataDirSwitch applies a validated data directory switch.
// This is pure business logic: destroys manager, updates storage, clears session.
// Does NOT handle plugin shutdown/restart or provider re-initialization - that's UI orchestration.
//
// STEPS 2-3: Destroy MCP manager, switch data directory
//
// Note: Provider re-initialization must be done by UI layer due to import cycle
// (provider package imports model, so model cannot import provider).
func (m *Model) ApplyDataDirSwitch(newDataDir string, passphrase string) error {
	if config.DebugLog != nil {
		config.DebugLog.Printf("[Model] STEP 2-3: Applying data dir switch to %s", newDataDir)
	}

	// Validate new data directory first
	newStorage, err := storage.NewSessionStorage(newDataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize session storage: %w", err)
	}

	newPluginsConfig, err := config.LoadPluginsConfig(newDataDir)
	if err != nil {
		return fmt.Errorf("failed to load plugins config: %w", err)
	}

	newRegistry, err := mcp.NewRegistry(newDataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize plugin registry: %w", err)
	}

	// STEP 2: Destroy MCP Manager (business logic - manager tied to old data dir)
	if m.MCPManager != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Model] STEP 2: Destroying MCP manager")
		}
		m.MCPManager = nil
	}

	// STEP 3: Switch data directory (business logic - state transitions)
	// Unlock current session from OLD data directory before switching
	if m.CurrentSession != nil && m.SessionStorage != nil {
		_ = m.SessionStorage.UnlockSession(m.CurrentSession.ID)
	}

	// Unlock OTUI instance from OLD data directory before switching
	if m.SessionStorage != nil {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Model] STEP 3a: Unlocking OTUI instance in old data dir")
		}
		_ = m.SessionStorage.UnlockOTUIInstance()
	}

	if config.DebugLog != nil {
		config.DebugLog.Printf("[Model] STEP 3b: Switching data directory")
	}

	m.SessionStorage = newStorage
	m.SearchIndex = storage.NewSearchIndex(newStorage)

	// Lock OTUI instance in NEW data directory
	if err := newStorage.LockOTUIInstance(); err != nil {
		return fmt.Errorf("failed to lock new data directory: %w", err)
	}
	if config.DebugLog != nil {
		config.DebugLog.Printf("[Model] STEP 3c: Locked OTUI instance in new data dir")
	}

	// Update plugin components
	m.Plugins.Registry = newRegistry
	m.Plugins.Config = newPluginsConfig

	// Clear current session (business rule: switching dirs clears session)
	m.CurrentSession = nil
	m.Messages = []Message{}
	m.SessionDirty = false
	m.NeedsInitialRender = false

	// Clear model cache for new data directory
	m.ModelCache = make(map[string][]ollama.ModelInfo)
	m.CacheExpiry = make(map[string]time.Time)

	// Re-initialize debug log for new data directory
	if config.Debug {
		config.InitDebugLog(m.Config.DataDir())
	}

	// Reload config from NEW data directory (business logic - config owns model state)
	if config.DebugLog != nil {
		config.DebugLog.Printf("[Model] STEP 3b: Reloading config from new data directory")
	}
	cfg, err := config.Load()
	if err != nil {
		// If passphrase required and we DON'T have one yet, return error for UI to handle
		if strings.Contains(err.Error(), "passphrase required") {
			if passphrase == "" {
				// Tell UI we need passphrase
				return fmt.Errorf("passphrase required: %w", err)
			}

			// We have passphrase - retry loading credentials with it
			if cfg.CredentialStore != nil {
				cfg.CredentialStore.SetPassphrase(passphrase)
				if err := cfg.CredentialStore.Load(cfg.DataDir()); err != nil {
					return fmt.Errorf("failed to load credentials with passphrase: %w", err)
				}
			}
		} else {
			// Other errors
			return fmt.Errorf("failed to reload config: %w", err)
		}
	}
	m.Config = cfg

	if config.DebugLog != nil {
		config.DebugLog.Printf("[Model] STEP 2-3 complete: Data directory switch applied")
	}

	return nil
}
