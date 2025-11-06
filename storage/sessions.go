package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Message represents a chat message
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Rendered  string    `json:"rendered,omitempty"` // Cached markdown rendering
	Timestamp time.Time `json:"timestamp"`
}

// Session represents a chat session
type Session struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Model          string    `json:"model"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Messages       []Message `json:"messages"`
	SystemPrompt   string    `json:"system_prompt,omitempty"`
	EnabledPlugins []string  `json:"enabled_plugins,omitempty"`
}

// SessionMetadata is a lightweight version of Session for listing
type SessionMetadata struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Model          string    `json:"model"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	MessageCount   int       `json:"message_count"`
	SystemPrompt   string    `json:"system_prompt,omitempty"`
	EnabledPlugins []string  `json:"enabled_plugins,omitempty"`
}

// SessionStorage handles session persistence
type SessionStorage struct {
	sessionsDir string
}

// NewSessionStorage creates a new session storage
func NewSessionStorage(dataDir string) (*SessionStorage, error) {
	sessionsDir := filepath.Join(dataDir, "sessions")

	// Create sessions directory if it doesn't exist (0700 - user-only access)
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	return &SessionStorage{
		sessionsDir: sessionsDir,
	}, nil
}

// Save saves a session to disk
func (s *SessionStorage) Save(session *Session) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}

	session.UpdatedAt = time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = session.UpdatedAt
	}

	filename := fmt.Sprintf("%s.json", session.ID)
	filepath := filepath.Join(s.sessionsDir, filename)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Use 0600 permissions - session files contain sensitive conversation history
	if err := os.WriteFile(filepath, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// Load loads a session from disk
func (s *SessionStorage) Load(id string) (*Session, error) {
	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(s.sessionsDir, filename)

	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// List returns metadata for all sessions, sorted by update time (newest first)
func (s *SessionStorage) List() ([]SessionMetadata, error) {
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SessionMetadata

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filepath := filepath.Join(s.sessionsDir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			continue // Skip corrupted files
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue // Skip corrupted files
		}

		sessions = append(sessions, SessionMetadata{
			ID:             session.ID,
			Name:           session.Name,
			Model:          session.Model,
			CreatedAt:      session.CreatedAt,
			UpdatedAt:      session.UpdatedAt,
			MessageCount:   len(session.Messages),
			SystemPrompt:   session.SystemPrompt,
			EnabledPlugins: session.EnabledPlugins,
		})
	}

	// Sort by UpdatedAt (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Delete deletes a session from disk
func (s *SessionStorage) Delete(id string) error {
	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(s.sessionsDir, filename)

	if err := os.Remove(filepath); err != nil {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	return nil
}

// SaveCurrentSessionID saves the ID of the current session
func (s *SessionStorage) SaveCurrentSessionID(id string) error {
	filepath := filepath.Join(filepath.Dir(s.sessionsDir), "current_session.id")
	return os.WriteFile(filepath, []byte(id), 0600)
}

// LoadCurrentSessionID loads the ID of the last active session
func (s *SessionStorage) LoadCurrentSessionID() (string, error) {
	filepath := filepath.Join(filepath.Dir(s.sessionsDir), "current_session.id")
	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// RenameSession updates the name of a session
func (s *SessionStorage) RenameSession(id string, newName string) error {
	session, err := s.Load(id)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	session.Name = newName

	if err := s.Save(session); err != nil {
		return fmt.Errorf("failed to save renamed session: %w", err)
	}

	return nil
}

// SanitizeFilename removes or replaces characters that are invalid in filenames
func SanitizeFilename(name string) string {
	// Replace problematic characters with hyphens
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "\n", "-")
	name = strings.ReplaceAll(name, "\r", "-")

	// Remove leading/trailing hyphens and dots
	name = strings.Trim(name, "-.")

	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}

	// If empty after sanitization, use generic name
	if name == "" {
		name = "session"
	}

	return name
}

// GenerateExportPath generates a default export path for a session
func GenerateExportPath(sessionName string) string {
	// Get Downloads directory (platform-specific)
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = os.Getenv("USERPROFILE") // Windows fallback
	}

	downloadsDir := filepath.Join(homeDir, "Downloads")

	// Sanitize session name for filename
	sanitized := SanitizeFilename(sessionName)

	// Generate timestamp
	timestamp := time.Now().Format("20060102-150405")

	// Generate filename
	filename := fmt.Sprintf("otui-session-%s-%s.json", sanitized, timestamp)

	return filepath.Join(downloadsDir, filename)
}

// ExportToJSON exports a session to a JSON file at the specified path
func (s *SessionStorage) ExportToJSON(id string, exportPath string) error {
	session, err := s.Load(id)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Ensure directory exists (0700 - user-only access)
	dir := filepath.Dir(exportPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file (0600 - session exports contain sensitive data)
	if err := os.WriteFile(exportPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GenerateSessionName generates a session name from the first user message
func GenerateSessionName(firstMessage string) string {
	if firstMessage == "" {
		return fmt.Sprintf("Session %s", time.Now().Format("Jan 2, 3:04 PM"))
	}

	// Take first 30 characters
	name := firstMessage
	if len(name) > 30 {
		name = name[:30] + "..."
	}

	// Remove newlines
	name = strings.ReplaceAll(name, "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")

	// Trim spaces
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Sprintf("Session %s", time.Now().Format("Jan 2, 3:04 PM"))
	}

	return name
}

// MessageMatch represents a search result within a session
type MessageMatch struct {
	MessageIndex int
	Role         string
	Content      string
	Preview      string
	Timestamp    time.Time
	Score        int
}

// SearchMessages searches messages in the current session
func SearchMessages(messages []Message, query string) []MessageMatch {
	if query == "" {
		return []MessageMatch{}
	}

	queryLower := strings.ToLower(query)
	var matches []MessageMatch

	for i, msg := range messages {
		if msg.Role == "system" {
			continue
		}

		if strings.Contains(strings.ToLower(msg.Content), queryLower) {
			preview := msg.Content
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}

			matches = append(matches, MessageMatch{
				MessageIndex: i,
				Role:         msg.Role,
				Content:      msg.Content,
				Preview:      preview,
				Timestamp:    msg.Timestamp,
				Score:        0,
			})
		}
	}

	return matches
}

func (s *Session) EnablePlugin(pluginID string) {
	if s.EnabledPlugins == nil {
		s.EnabledPlugins = []string{}
	}

	for _, id := range s.EnabledPlugins {
		if id == pluginID {
			return
		}
	}

	s.EnabledPlugins = append(s.EnabledPlugins, pluginID)
}

func (s *Session) DisablePlugin(pluginID string) {
	if s.EnabledPlugins == nil {
		return
	}

	filtered := []string{}
	for _, id := range s.EnabledPlugins {
		if id != pluginID {
			filtered = append(filtered, id)
		}
	}
	s.EnabledPlugins = filtered
}

func (s *Session) IsPluginEnabled(pluginID string) bool {
	if s.EnabledPlugins == nil {
		return false
	}

	for _, id := range s.EnabledPlugins {
		if id == pluginID {
			return true
		}
	}
	return false
}

func (s *Session) GetEnabledPlugins() []string {
	if s.EnabledPlugins == nil {
		return []string{}
	}
	return s.EnabledPlugins
}

// LockSession creates a lock file for a session to indicate it's in use
// Lock file format: ~/.local/share/otui/sessions/{session-id}.lock
// Content: PID of the OTUI instance using this session
func (ss *SessionStorage) LockSession(sessionID string) error {
	lockPath := filepath.Join(ss.sessionsDir, sessionID+".lock")
	pid := os.Getpid()

	// Write PID to lock file (0600 - user-only access)
	return os.WriteFile(lockPath, []byte(fmt.Sprintf("%d", pid)), 0600)
}

// UnlockSession removes the lock file for a session
func (ss *SessionStorage) UnlockSession(sessionID string) error {
	lockPath := filepath.Join(ss.sessionsDir, sessionID+".lock")

	// Ignore error if file doesn't exist
	err := os.Remove(lockPath)
	if os.IsNotExist(err) {
		return nil
	}

	return err
}

// CheckSessionLock checks if a session is locked by another OTUI instance
// Returns (isLocked bool, err error)
// If isLocked is true, the session is actively being used by another instance
func (ss *SessionStorage) CheckSessionLock(sessionID string) (bool, error) {
	lockPath := filepath.Join(ss.sessionsDir, sessionID+".lock")

	// Check if lock file exists
	data, err := os.ReadFile(lockPath)
	if os.IsNotExist(err) {
		return false, nil // No lock file, not locked
	}
	if err != nil {
		return false, fmt.Errorf("failed to read lock file: %w", err)
	}

	// Parse PID from lock file
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		// Invalid lock file, clean it up
		_ = os.Remove(lockPath)
		return false, nil
	}

	// Check if process with this PID is still running
	// os.FindProcess() always succeeds on Unix, but we use it as a basic check
	// For cross-platform compatibility, we trust FindProcess() without signaling
	_, err = os.FindProcess(pid)
	if err != nil {
		// Process not found (Windows), clean up stale lock
		_ = os.Remove(lockPath)
		return false, nil
	}

	// Process exists, session is locked
	// Note: On Unix this doesn't verify the process is truly alive,
	// but it's cross-platform compatible and good enough for our use case
	return true, nil
}

// LockOTUIInstance creates a global lock to ensure single-instance operation
// Lock file: <data_dir>/otui.lock
// Content: PID of the running OTUI instance
// This prevents multiple OTUI instances from running simultaneously to avoid plugin port conflicts
func (ss *SessionStorage) LockOTUIInstance() error {
	// Get parent directory of sessionsDir (the data directory)
	dataDir := filepath.Dir(ss.sessionsDir)
	lockPath := filepath.Join(dataDir, "otui.lock")
	pid := os.Getpid()

	// Write PID to lock file (0600 - user-only access)
	return os.WriteFile(lockPath, []byte(fmt.Sprintf("%d", pid)), 0600)
}

// UnlockOTUIInstance removes the global OTUI instance lock
func (ss *SessionStorage) UnlockOTUIInstance() error {
	// Get parent directory of sessionsDir (the data directory)
	dataDir := filepath.Dir(ss.sessionsDir)
	lockPath := filepath.Join(dataDir, "otui.lock")

	// Ignore error if file doesn't exist
	err := os.Remove(lockPath)
	if os.IsNotExist(err) {
		return nil
	}

	return err
}

// CheckOTUIInstanceLock checks if another OTUI instance is currently running
// Returns (isLocked bool, runningPID int, err error)
// If isLocked is true, another OTUI instance is active on this system
func (ss *SessionStorage) CheckOTUIInstanceLock() (bool, int, error) {
	// Get parent directory of sessionsDir (the data directory)
	dataDir := filepath.Dir(ss.sessionsDir)
	lockPath := filepath.Join(dataDir, "otui.lock")

	// Check if lock file exists
	data, err := os.ReadFile(lockPath)
	if os.IsNotExist(err) {
		return false, 0, nil // No lock file, not locked
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to read lock file: %w", err)
	}

	// Parse PID from lock file
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		// Invalid lock file, clean it up
		_ = os.Remove(lockPath)
		return false, 0, nil
	}

	// Check if process with this PID is still running
	_, err = os.FindProcess(pid)
	if err != nil {
		// Process not found (Windows), clean up stale lock
		_ = os.Remove(lockPath)
		return false, 0, nil
	}

	// Process exists, instance is locked
	return true, pid, nil
}
