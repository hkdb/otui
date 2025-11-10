package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// LoadSSHPrivateKey loads an SSH private key from the given path.
// If the key is encrypted, it prompts the user for the passphrase.
// This function is designed to work both in CLI prompts and TUI modals.
func LoadSSHPrivateKey(keyPath string) (ssh.Signer, error) {
	// Read the key file
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key: %w", err)
	}

	// Parse the key (encryption check is done upstream in Initialize())
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}

	return signer, nil
}

// IsSSHKeyEncrypted checks if an SSH private key is encrypted without attempting to decrypt it
func IsSSHKeyEncrypted(keyPath string) (bool, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return false, fmt.Errorf("failed to read SSH key: %w", err)
	}

	// Try to parse without passphrase
	_, err = ssh.ParsePrivateKey(keyData)
	if err == nil {
		return false, nil // Key is not encrypted
	}

	// Check if error is due to encryption
	if strings.Contains(err.Error(), "encrypted") ||
		strings.Contains(err.Error(), "passphrase") {
		return true, nil // Key is encrypted
	}

	// Other error (invalid key format, etc.)
	return false, fmt.Errorf("invalid SSH key: %w", err)
}

// LoadSSHPrivateKeyWithPassphrase loads an encrypted SSH private key using the provided passphrase
func LoadSSHPrivateKeyWithPassphrase(keyPath string, passphrase string) (ssh.Signer, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key (wrong passphrase?): %w", err)
	}

	return signer, nil
}

// FindSSHKeys scans ~/.ssh for SSH private keys and returns their paths.
// Prioritizes otui_ed25519 if it exists.
func FindSSHKeys() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	sshDir := filepath.Join(homeDir, ".ssh")

	// Check if .ssh directory exists
	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	// Common SSH key names (prioritize OTUI key)
	keyNames := []string{
		"otui_ed25519", // OTUI-specific key (highest priority)
		"id_ed25519",   // Modern ED25519
		"id_rsa",       // RSA
		"id_ecdsa",     // ECDSA
		"id_dsa",       // DSA (legacy)
	}

	var foundKeys []string
	for _, name := range keyNames {
		keyPath := filepath.Join(sshDir, name)
		if _, err := os.Stat(keyPath); err == nil {
			// Verify it's actually a private key
			if isPrivateKey(keyPath) {
				foundKeys = append(foundKeys, keyPath)
			}
		}
	}

	return foundKeys, nil
}

// isPrivateKey checks if a file is likely an SSH private key
func isPrivateKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	// Check for common SSH private key markers
	content := string(data)
	return strings.Contains(content, "BEGIN") &&
		strings.Contains(content, "PRIVATE KEY")
}

// CreateOTUIKey generates a new ED25519 SSH key pair for OTUI use.
// The passphrase parameter is optional (empty string for no passphrase).
// Returns the actual path where the key was created.
func CreateOTUIKey(passphrase string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	baseKeyName := "otui_ed25519"
	keyPath := filepath.Join(sshDir, baseKeyName)

	// Check if base key already exists - if so, append timestamp+counter
	if _, err := os.Stat(keyPath); err == nil {
		dateStr := time.Now().Format("20060102") // YYYYMMDD
		counter := 1

		for {
			newKeyName := fmt.Sprintf("%s_%s%02d", baseKeyName, dateStr, counter)
			keyPath = filepath.Join(sshDir, newKeyName)

			// Found unused name
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				break
			}

			counter++
			if counter > 99 {
				return "", fmt.Errorf("exceeded maximum key creation limit for today (99)")
			}
		}

		if DebugLog != nil {
			DebugLog.Printf("[SSH] Base key exists, using unique name: %s", filepath.Base(keyPath))
		}
	}

	// Ensure .ssh directory exists
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Build ssh-keygen command
	args := []string{
		"-t", "ed25519",
		"-f", keyPath,
		"-C", "otui-encryption-key",
		"-N", passphrase, // Empty string for no passphrase
	}

	cmd := exec.Command("ssh-keygen", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to generate SSH key: %w\nOutput: %s", err, output)
	}

	// Set proper permissions on the private key
	if err := os.Chmod(keyPath, 0600); err != nil {
		return "", fmt.Errorf("failed to set key permissions: %w", err)
	}

	if DebugLog != nil {
		DebugLog.Printf("[SSH] Created OTUI encryption key at %s", keyPath)
	}

	return keyPath, nil
}

// GetOTUIKeyPath returns the BASE path to the OTUI-specific SSH key.
// WARNING: This function only returns the base name (~/.ssh/otui_ed25519),
// not timestamped variants like otui_ed25519_2025111001.
//
// For key creation: Use the path returned by CreateOTUIKey() instead.
// For existence checks: This function is appropriate (used by OTUIKeyExists).
func GetOTUIKeyPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".ssh", "otui_ed25519")
}

// OTUIKeyExists checks if the OTUI SSH key already exists
func OTUIKeyExists() bool {
	keyPath := GetOTUIKeyPath()
	_, err := os.Stat(keyPath)
	return err == nil
}
