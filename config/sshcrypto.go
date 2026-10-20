package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
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

	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return true, nil // Key is encrypted
	}

	// Other error (invalid key format, etc.)
	return false, fmt.Errorf("invalid SSH key: %w", err)
}

// ErrUnsupportedKeyType is returned when an SSH key can't be used to encrypt
// credentials
var ErrUnsupportedKeyType = errors.New("unsupported SSH key type")

// IsSupportedEncryptionKeyType reports whether keys of this type can be used
// to encrypt credentials. The encryption key is derived from a signature, so
// signatures must be deterministic: ed25519 (used for new keys) and RSA (kept
// for existing setups) are. ECDSA and DSA signatures are randomized, which
// would derive a different key every launch.
func IsSupportedEncryptionKeyType(keyType string) bool {
	return keyType == ssh.KeyAlgoED25519 || keyType == ssh.KeyAlgoRSA
}

// SSHKeyType returns the type of a private key file (e.g. "ssh-ed25519")
// without needing its passphrase
func SSHKeyType(keyPath string) (string, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err == nil {
		return signer.PublicKey().Type(), nil
	}

	// Encrypted OpenSSH keys include the public key
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) && missing.PublicKey != nil {
		return missing.PublicKey.Type(), nil
	}

	// Fall back to the matching .pub file
	if pubData, pubErr := os.ReadFile(keyPath + ".pub"); pubErr == nil {
		if pub, _, _, _, pubErr := ssh.ParseAuthorizedKey(pubData); pubErr == nil {
			return pub.Type(), nil
		}
	}

	// Legacy PEM keys name their type in the header
	content := string(keyData)
	switch {
	case strings.Contains(content, "BEGIN RSA PRIVATE KEY"):
		return ssh.KeyAlgoRSA, nil
	case strings.Contains(content, "BEGIN EC PRIVATE KEY"):
		return "ecdsa", nil
	case strings.Contains(content, "BEGIN DSA PRIVATE KEY"):
		return ssh.KeyAlgoDSA, nil
	}

	return "", fmt.Errorf("unable to determine SSH key type: %w", err)
}

// IsIncorrectPassphrase reports whether err was caused by a wrong SSH key
// passphrase (as opposed to e.g. credentials encrypted with a different key)
func IsIncorrectPassphrase(err error) bool {
	return errors.Is(err, x509.IncorrectPasswordError)
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

	// Common SSH key names (prioritize OTUI key). Only key types that can
	// encrypt credentials (see IsSupportedEncryptionKeyType).
	keyNames := []string{
		"otui_ed25519", // OTUI-specific key (highest priority)
		"id_ed25519",   // Modern ED25519
		"id_rsa",       // RSA (existing setups)
	}

	var foundKeys []string
	for _, name := range keyNames {
		keyPath := filepath.Join(sshDir, name)
		if _, err := os.Stat(keyPath); err != nil || !isPrivateKey(keyPath) {
			continue
		}
		// The file name doesn't guarantee the type
		if keyType, err := SSHKeyType(keyPath); err != nil || !IsSupportedEncryptionKeyType(keyType) {
			continue
		}
		foundKeys = append(foundKeys, keyPath)
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

	// Generate the key in-process (not with ssh-keygen: its -N argument
	// would show the passphrase in the process list)
	if err := writeED25519Key(keyPath, passphrase, "otui-encryption-key"); err != nil {
		return "", err
	}

	if DebugLog != nil {
		DebugLog.Printf("[SSH] Created OTUI encryption key at %s", keyPath)
	}

	return keyPath, nil
}

// writeED25519Key generates an ED25519 key pair and writes it in OpenSSH
// format (same as ssh-keygen -t ed25519): the private key to keyPath (0600,
// encrypted if passphrase isn't empty) and the public key to keyPath.pub
func writeED25519Key(keyPath, passphrase, comment string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate SSH key: %w", err)
	}

	var block *pem.Block
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(priv, comment)
	}
	if err != nil {
		return fmt.Errorf("failed to encode SSH key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return fmt.Errorf("failed to encode SSH public key: %w", err)
	}
	pubLine := strings.TrimSuffix(string(ssh.MarshalAuthorizedKey(sshPub)), "\n") + " " + comment + "\n"

	// O_EXCL: never overwrite an existing key
	f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("failed to create SSH key file: %w", err)
	}
	if err := pem.Encode(f, block); err != nil {
		f.Close()
		os.Remove(keyPath)
		return fmt.Errorf("failed to write SSH key: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(keyPath)
		return fmt.Errorf("failed to write SSH key: %w", err)
	}

	if err := os.WriteFile(keyPath+".pub", []byte(pubLine), 0644); err != nil {
		os.Remove(keyPath)
		return fmt.Errorf("failed to write SSH public key: %w", err)
	}
	return nil
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
