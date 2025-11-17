package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// SecurityMethod defines the credential storage method
type SecurityMethod string

const (
	SecurityPlainText SecurityMethod = "plaintext"
	SecuritySSHKey    SecurityMethod = "ssh_key"
)

// CredentialStore manages encrypted or plain-text API credentials
type CredentialStore struct {
	method      SecurityMethod
	credentials map[string]string // providerID → API key
	sshKeyPath  string            // path to SSH key (ssh_key method only)
	passphrase  string            // Optional passphrase for encrypted keys
	encManager  *EncryptionManager
}

// NewCredentialStore creates a new credential store
func NewCredentialStore(method SecurityMethod, sshKeyPath string) *CredentialStore {
	return &CredentialStore{
		method:      method,
		credentials: make(map[string]string),
		sshKeyPath:  sshKeyPath,
	}
}

// SetPassphrase sets the passphrase for decrypting the SSH key
func (c *CredentialStore) SetPassphrase(passphrase string) {
	c.passphrase = passphrase
	if c.encManager != nil {
		c.encManager.SetPassphrase(passphrase)
	}
}

// Load loads credentials from disk based on the configured security method
func (c *CredentialStore) Load(dataDir string) error {
	switch c.method {
	case SecurityPlainText:
		creds, err := loadPlainText(dataDir)
		if err != nil {
			return err
		}
		c.credentials = creds
		return nil

	case SecuritySSHKey:
		creds, err := c.loadSSHEncrypted(dataDir)
		if err != nil {
			return err
		}
		c.credentials = creds
		return nil

	default:
		return fmt.Errorf("unknown security method: %s", c.method)
	}
}

// Save saves credentials to disk based on the configured security method
func (c *CredentialStore) Save(dataDir string) error {
	switch c.method {
	case SecurityPlainText:
		return savePlainText(dataDir, c.credentials)

	case SecuritySSHKey:
		return c.saveSSHEncrypted(dataDir)

	default:
		return fmt.Errorf("unknown security method: %s", c.method)
	}
}

// Get retrieves a credential for a provider
func (c *CredentialStore) Get(providerID string) string {
	return c.credentials[providerID]
}

// Set stores a credential for a provider
func (c *CredentialStore) Set(providerID string, apiKey string) error {
	c.credentials[providerID] = apiKey
	return nil
}

// Delete removes a credential for a provider
func (c *CredentialStore) Delete(providerID string) error {
	delete(c.credentials, providerID)
	return nil
}

// GetPlugin retrieves a plugin credential
// Format: plugin_<pluginID>_<key>
func (c *CredentialStore) GetPlugin(pluginID, key string) string {
	credKey := fmt.Sprintf("plugin_%s_%s", pluginID, key)
	return c.credentials[credKey]
}

// SetPlugin stores a plugin credential
func (c *CredentialStore) SetPlugin(pluginID, key, value string) error {
	credKey := fmt.Sprintf("plugin_%s_%s", pluginID, key)
	c.credentials[credKey] = value
	return nil
}

// DeletePluginAll removes all credentials for a plugin
func (c *CredentialStore) DeletePluginAll(pluginID string) error {
	prefix := fmt.Sprintf("plugin_%s_", pluginID)
	for key := range c.credentials {
		switch {
		case len(key) >= len(prefix) && key[:len(prefix)] == prefix:
			delete(c.credentials, key)
		}
	}
	return nil
}

// GetEncryptionManager returns the encryption manager (for FileTokenStore)
func (c *CredentialStore) GetEncryptionManager() *EncryptionManager {
	return c.encManager
}

// GetMethod returns the current security method
func (c *CredentialStore) GetMethod() SecurityMethod {
	return c.method
}

// credentialsPath returns the path to the plain text credentials file
func credentialsPath(dataDir string) string {
	return filepath.Join(dataDir, "credentials.toml")
}

// encryptedCredentialsPath returns the path to the encrypted credentials file
func encryptedCredentialsPath(dataDir string) string {
	return filepath.Join(dataDir, "credentials.enc")
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ===== Plain Text Storage =====

// loadPlainText loads credentials from plain text TOML file
func loadPlainText(dataDir string) (map[string]string, error) {
	path := credentialsPath(dataDir)

	// If file doesn't exist, return empty map (no error)
	if !fileExists(path) {
		return make(map[string]string), nil
	}

	// Parse TOML
	type credentialsFile struct {
		Credentials map[string]string `toml:"credentials"`
	}

	var cf credentialsFile
	if _, err := toml.DecodeFile(path, &cf); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	return cf.Credentials, nil
}

// savePlainText saves credentials to plain text TOML file with 0600 permissions
func savePlainText(dataDir string, creds map[string]string) error {
	path := credentialsPath(dataDir)

	// Create credentials structure
	type credentialsFile struct {
		Credentials map[string]string `toml:"credentials"`
	}

	cf := credentialsFile{
		Credentials: creds,
	}

	// Create file with 0600 permissions (owner read/write only)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create credentials file: %w", err)
	}
	defer f.Close()

	// Encode to TOML
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cf); err != nil {
		return fmt.Errorf("failed to encode credentials: %w", err)
	}

	return nil
}

// ===== SSH Key Encrypted Storage =====

// loadSSHEncrypted loads and decrypts credentials using SSH key encryption
func (c *CredentialStore) loadSSHEncrypted(dataDir string) (map[string]string, error) {
	path := encryptedCredentialsPath(dataDir)

	// If file doesn't exist, return empty map (no error)
	if !fileExists(path) {
		return make(map[string]string), nil
	}

	// Reinitialize if manager doesn't exist OR if we now have a passphrase
	if c.encManager == nil || c.passphrase != "" {
		// Security: Commented out to avoid logging passphrase state
		// Uncomment for debugging passphrase-related issues
		// if Debug && DebugLog != nil {
		// 	DebugLog.Printf("[CredentialStore] loadSSHEncrypted: Reinitializing EncryptionManager (encManager==nil: %v, passphrase set: %v)", c.encManager == nil, c.passphrase != "")
		// }
		c.encManager = NewEncryptionManager(EncryptionSSHKey, c.sshKeyPath)
		c.encManager.SetPassphrase(c.passphrase)
		// Security: Commented out to avoid logging passphrase length
		// Uncomment for debugging passphrase-related issues
		// if Debug && DebugLog != nil {
		// 	DebugLog.Printf("[CredentialStore] loadSSHEncrypted: Set passphrase on EncryptionManager (len=%d)", len(c.passphrase))
		// }
		if err := c.encManager.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize encryption: %w", err)
		}
	}

	// Read encrypted file
	encryptedData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted credentials: %w", err)
	}

	// Decrypt
	decryptedData, err := c.encManager.Decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Parse JSON
	var creds map[string]string
	if err := json.Unmarshal(decryptedData, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted credentials: %w", err)
	}

	return creds, nil
}

// saveSSHEncrypted encrypts and saves credentials using SSH key encryption
func (c *CredentialStore) saveSSHEncrypted(dataDir string) error {
	path := encryptedCredentialsPath(dataDir)

	// Reinitialize if manager doesn't exist OR if we now have a passphrase
	if c.encManager == nil || c.passphrase != "" {
		c.encManager = NewEncryptionManager(EncryptionSSHKey, c.sshKeyPath)
		c.encManager.SetPassphrase(c.passphrase)
		if err := c.encManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize encryption: %w", err)
		}
	}

	// Serialize credentials to JSON
	jsonData, err := json.MarshalIndent(c.credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize credentials: %w", err)
	}

	// Encrypt
	encryptedData, err := c.encManager.Encrypt(jsonData)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// Write to file with 0600 permissions
	if err := os.WriteFile(path, encryptedData, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted credentials: %w", err)
	}

	return nil
}
