package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// writeKey writes priv in OpenSSH format (passphrase protected if set) and
// returns the path
func writeKey(t *testing.T, dir, name string, priv crypto.PrivateKey, passphrase string) string {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(priv, "")
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newKeys(t *testing.T) (ed25519.PrivateKey, *rsa.PrivateKey, *ecdsa.PrivateKey) {
	t.Helper()
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return ed, rsaKey, ecKey
}

func TestSSHKeyType(t *testing.T) {
	dir := t.TempDir()
	ed, rsaKey, ecKey := newKeys(t)

	tests := []struct {
		name      string
		path      string
		want      string
		supported bool
	}{
		{"ed25519", writeKey(t, dir, "ed", ed, ""), ssh.KeyAlgoED25519, true},
		{"ed25519 with passphrase", writeKey(t, dir, "ed_enc", ed, "secret"), ssh.KeyAlgoED25519, true},
		{"rsa", writeKey(t, dir, "rsa", rsaKey, ""), ssh.KeyAlgoRSA, true},
		{"ecdsa", writeKey(t, dir, "ec", ecKey, ""), ssh.KeyAlgoECDSA256, false},
		{"ecdsa with passphrase", writeKey(t, dir, "ec_enc", ecKey, "secret"), ssh.KeyAlgoECDSA256, false},
	}

	// Legacy encrypted PEM has no public key - type comes from the .pub file
	//lint:ignore SA1019 legacy format on purpose
	legacyBlock, err := x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey), []byte("secret"), x509.PEMCipherAES256) //nolint:staticcheck
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "legacy_rsa")
	os.WriteFile(legacy, pem.EncodeToMemory(legacyBlock), 0600)
	tests = append(tests, struct {
		name      string
		path      string
		want      string
		supported bool
	}{"legacy encrypted rsa (PEM header)", legacy, ssh.KeyAlgoRSA, true})

	ecPub, _ := ssh.NewPublicKey(&ecKey.PublicKey)
	legacyWithPub := filepath.Join(dir, "legacy_with_pub")
	os.WriteFile(legacyWithPub, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ngarbage\n-----END OPENSSH PRIVATE KEY-----\n"), 0600)
	os.WriteFile(legacyWithPub+".pub", ssh.MarshalAuthorizedKey(ecPub), 0600)
	tests = append(tests, struct {
		name      string
		path      string
		want      string
		supported bool
	}{".pub fallback", legacyWithPub, ssh.KeyAlgoECDSA256, false})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SSHKeyType(tt.path)
			if err != nil {
				t.Fatalf("SSHKeyType: %v", err)
			}
			if got != tt.want {
				t.Errorf("type = %q, want %q", got, tt.want)
			}
			if IsSupportedEncryptionKeyType(got) != tt.supported {
				t.Errorf("IsSupportedEncryptionKeyType(%q) = %v, want %v", got, !tt.supported, tt.supported)
			}
		})
	}

	//lint:ignore SA1019 checks that DSA is rejected
	if IsSupportedEncryptionKeyType(ssh.KeyAlgoDSA) {
		t.Error("DSA must not be supported")
	}
}

func TestFindSSHKeysOnlySupportedTypes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	os.MkdirAll(sshDir, 0700)

	ed, rsaKey, ecKey := newKeys(t)
	writeKey(t, sshDir, "id_ed25519", ed, "")
	writeKey(t, sshDir, "id_ecdsa", ecKey, "")
	os.WriteFile(filepath.Join(sshDir, "id_dsa"), []byte("-----BEGIN DSA PRIVATE KEY-----\nx\n-----END DSA PRIVATE KEY-----\n"), 0600)

	keys, err := FindSSHKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || filepath.Base(keys[0]) != "id_ed25519" {
		t.Errorf("FindSSHKeys = %v, want only id_ed25519", keys)
	}

	// An ECDSA key saved under an RSA name is still excluded
	writeKey(t, sshDir, "id_rsa", ecKey, "")
	keys, _ = FindSSHKeys()
	if len(keys) != 1 {
		t.Errorf("ECDSA key named id_rsa was listed: %v", keys)
	}

	// A real RSA key is listed (existing setups)
	writeKey(t, sshDir, "id_rsa", rsaKey, "")
	keys, _ = FindSSHKeys()
	if len(keys) != 2 || filepath.Base(keys[1]) != "id_rsa" {
		t.Errorf("FindSSHKeys = %v, want id_ed25519 and id_rsa", keys)
	}
}

func TestEncryptionManagerKeyTypes(t *testing.T) {
	dir := t.TempDir()
	_, rsaKey, ecKey := newKeys(t)

	t.Run("ecdsa rejected", func(t *testing.T) {
		em := NewEncryptionManager(EncryptionSSHKey, writeKey(t, dir, "ec", ecKey, ""))
		if err := em.Initialize(); !errors.Is(err, ErrUnsupportedKeyType) {
			t.Fatalf("err = %v, want ErrUnsupportedKeyType", err)
		}
	})

	t.Run("ecdsa with passphrase rejected after passphrase", func(t *testing.T) {
		em := NewEncryptionManager(EncryptionSSHKey, writeKey(t, dir, "ec_enc", ecKey, "secret"))
		em.SetPassphrase("secret")
		if err := em.Initialize(); !errors.Is(err, ErrUnsupportedKeyType) {
			t.Fatalf("err = %v, want ErrUnsupportedKeyType", err)
		}
	})

	t.Run("rsa derives the same key every time", func(t *testing.T) {
		path := writeKey(t, dir, "rsa", rsaKey, "")
		first := NewEncryptionManager(EncryptionSSHKey, path)
		if err := first.Initialize(); err != nil {
			t.Fatal(err)
		}
		encrypted, err := first.Encrypt([]byte(`{"openai":"sk-test"}`))
		if err != nil {
			t.Fatal(err)
		}

		second := NewEncryptionManager(EncryptionSSHKey, path) // next launch
		if err := second.Initialize(); err != nil {
			t.Fatal(err)
		}
		plain, err := second.Decrypt(encrypted)
		if err != nil || string(plain) != `{"openai":"sk-test"}` {
			t.Errorf("RSA round trip failed: %q, %v", plain, err)
		}
	})
}

func TestIsSSHKeyEncrypted(t *testing.T) {
	dir := t.TempDir()
	ed, _, _ := newKeys(t)
	if enc, err := IsSSHKeyEncrypted(writeKey(t, dir, "plain", ed, "")); err != nil || enc {
		t.Errorf("unencrypted key: encrypted=%v err=%v", enc, err)
	}
	if enc, err := IsSSHKeyEncrypted(writeKey(t, dir, "enc", ed, "secret")); err != nil || !enc {
		t.Errorf("encrypted key: encrypted=%v err=%v", enc, err)
	}
}
