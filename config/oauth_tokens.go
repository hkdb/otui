package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	transport "github.com/mark3labs/mcp-go/client/transport"
)

// FileTokenStore implements TokenStore interface with file persistence
// Respects user's security choice (plaintext vs SSH key encryption)
type FileTokenStore struct {
	pluginID string
	dataDir  string
	security SecurityMethod
	encMgr   *EncryptionManager
	mu       sync.RWMutex
}

// NewFileTokenStore creates a persistent token store
func NewFileTokenStore(pluginID string, dataDir string, security SecurityMethod, encMgr *EncryptionManager) *FileTokenStore {
	return &FileTokenStore{
		pluginID: pluginID,
		dataDir:  dataDir,
		security: security,
		encMgr:   encMgr,
	}
}

// GetToken loads token from disk
func (s *FileTokenStore) GetToken(ctx context.Context) (*transport.Token, error) {
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tokenPath := s.getTokenPath()

	// Check if file exists
	switch _, err := os.Stat(tokenPath); {
	case os.IsNotExist(err):
		return nil, transport.ErrNoToken
	case err != nil:
		return nil, fmt.Errorf("failed to stat token file: %w", err)
	}

	// Read file
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	// Decrypt if using SSH key security
	switch s.security {
	case SecuritySSHKey:
		switch {
		case s.encMgr == nil:
			return nil, fmt.Errorf("encryption manager not initialized")
		}
		decrypted, err := s.encMgr.Decrypt(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt token: %w", err)
		}
		data = decrypted
	case SecurityPlainText:
		// Use data as-is
	default:
		return nil, fmt.Errorf("unknown security method: %s", s.security)
	}

	// Unmarshal token
	var token transport.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return &token, nil
}

// SaveToken saves token to disk
func (s *FileTokenStore) SaveToken(ctx context.Context, token *transport.Token) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal token
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	// Encrypt if using SSH key security
	switch s.security {
	case SecuritySSHKey:
		switch {
		case s.encMgr == nil:
			return fmt.Errorf("encryption manager not initialized")
		}
		encrypted, err := s.encMgr.Encrypt(data)
		if err != nil {
			return fmt.Errorf("failed to encrypt token: %w", err)
		}
		data = encrypted
	case SecurityPlainText:
		// Use data as-is
	default:
		return fmt.Errorf("unknown security method: %s", s.security)
	}

	// Ensure directory exists
	tokenDir := filepath.Dir(s.getTokenPath())
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}

	// Write to file
	tokenPath := s.getTokenPath()
	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// getTokenPath returns the path to the token file for this plugin
func (s *FileTokenStore) getTokenPath() string {
	switch s.security {
	case SecuritySSHKey:
		// Store with other encrypted credentials
		return filepath.Join(s.dataDir, fmt.Sprintf("oauth_token_%s.enc", s.pluginID))
	default:
		// Store in plaintext
		return filepath.Join(s.dataDir, fmt.Sprintf("oauth_token_%s.json", s.pluginID))
	}
}
