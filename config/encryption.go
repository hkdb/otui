package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// EncryptionMethod defines how data is encrypted
type EncryptionMethod string

const (
	EncryptionNone   EncryptionMethod = "none"
	EncryptionSSHKey EncryptionMethod = "ssh_key"
)

// EncryptionManager provides general-purpose encryption for any OTUI data
// (credentials, sessions, etc.). It's designed to be reusable across different
// storage systems.
type EncryptionManager struct {
	method     EncryptionMethod
	sshKeyPath string
	passphrase string     // Optional passphrase for encrypted keys
	signer     ssh.Signer // Cached SSH signer (if using SSH key method)
	aesKey     []byte     // Cached AES key derived from SSH signature
}

// NewEncryptionManager creates a new encryption manager
func NewEncryptionManager(method EncryptionMethod, sshKeyPath string) *EncryptionManager {
	return &EncryptionManager{
		method:     method,
		sshKeyPath: sshKeyPath,
	}
}

// SetPassphrase sets the passphrase for decrypting the SSH key
func (e *EncryptionManager) SetPassphrase(passphrase string) {
	e.passphrase = passphrase
}

// Initialize prepares the encryption manager (loads keys, derives AES keys, etc.)
// For SSH key method, this loads the SSH key and prompts for passphrase if needed.
func (e *EncryptionManager) Initialize() error {
	switch e.method {
	case EncryptionNone:
		return nil

	case EncryptionSSHKey:
		// Security: Commented out to avoid logging passphrase length
		// Uncomment for debugging passphrase-related issues
		// if Debug && DebugLog != nil {
		// 	DebugLog.Printf("[EncryptionManager] Initialize: Starting (keyPath=%s, passphrase len=%d)", e.sshKeyPath, len(e.passphrase))
		// }

		// First check if key is encrypted (parse only, no decrypt attempt)
		encrypted, err := IsSSHKeyEncrypted(e.sshKeyPath)
		if err != nil {
			return fmt.Errorf("failed to check SSH key: %w", err)
		}

		if Debug && DebugLog != nil {
			DebugLog.Printf("[EncryptionManager] Initialize: Key encrypted=%v", encrypted)
		}

		var signer ssh.Signer

		// If encrypted and no passphrase provided, return error immediately
		if encrypted && e.passphrase == "" {
			if Debug && DebugLog != nil {
				DebugLog.Printf("[EncryptionManager] Initialize: ERROR - Key is encrypted but no passphrase provided")
			}
			return fmt.Errorf("SSH key is encrypted - passphrase required")
		}

		// Load key with appropriate method
		if encrypted {
			signer, err = LoadSSHPrivateKeyWithPassphrase(e.sshKeyPath, e.passphrase)
		} else {
			signer, err = LoadSSHPrivateKey(e.sshKeyPath)
		}

		if err != nil {
			return fmt.Errorf("failed to load SSH key: %w", err)
		}
		e.signer = signer

		// Derive AES key from SSH signature
		aesKey, err := DeriveAESKeyFromSSH(signer)
		if err != nil {
			return fmt.Errorf("failed to derive encryption key: %w", err)
		}
		e.aesKey = aesKey

		return nil

	default:
		return fmt.Errorf("unknown encryption method: %s", e.method)
	}
}

// Encrypt encrypts data using the configured method
// Returns encrypted data or original data if method is EncryptionNone
func (e *EncryptionManager) Encrypt(plaintext []byte) ([]byte, error) {
	switch e.method {
	case EncryptionNone:
		return plaintext, nil

	case EncryptionSSHKey:
		if e.aesKey == nil {
			return nil, fmt.Errorf("encryption manager not initialized")
		}
		return encryptAESGCM(plaintext, e.aesKey)

	default:
		return nil, fmt.Errorf("unknown encryption method: %s", e.method)
	}
}

// Decrypt decrypts data using the configured method
// Returns decrypted data or original data if method is EncryptionNone
func (e *EncryptionManager) Decrypt(ciphertext []byte) ([]byte, error) {
	switch e.method {
	case EncryptionNone:
		return ciphertext, nil

	case EncryptionSSHKey:
		if e.aesKey == nil {
			return nil, fmt.Errorf("encryption manager not initialized")
		}
		return decryptAESGCM(ciphertext, e.aesKey)

	default:
		return nil, fmt.Errorf("unknown encryption method: %s", e.method)
	}
}

// GetMethod returns the current encryption method
func (e *EncryptionManager) GetMethod() EncryptionMethod {
	return e.method
}

// encryptAESGCM encrypts data using AES-256-GCM
// Format: [nonce (12 bytes)][ciphertext + tag]
func encryptAESGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptAESGCM decrypts data using AES-256-GCM
// Expects format: [nonce (12 bytes)][ciphertext + tag]
func decryptAESGCM(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce := ciphertext[:nonceSize]
	ciphertextData := ciphertext[nonceSize:]

	// Decrypt and verify
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// DeriveAESKeyFromSSH derives a 32-byte AES-256 key from an SSH key signature
// This provides deterministic encryption: same SSH key always produces same AES key
func DeriveAESKeyFromSSH(signer ssh.Signer) ([]byte, error) {
	// Sign a fixed message to get a deterministic signature
	message := []byte("otui-encryption-key-derivation-v1")

	signature, err := signer.Sign(rand.Reader, message)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	// Hash the signature to get a 32-byte key
	hash := sha256.Sum256(signature.Blob)
	return hash[:], nil
}
