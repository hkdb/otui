package config

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestCreateOTUIKeyWithoutSSHKeygen(t *testing.T) {
	sshKeygen, _ := exec.LookPath("ssh-keygen")

	for _, passphrase := range []string{"", "correct horse"} {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("PATH", "") // ssh-keygen isn't needed (or run)

		keyPath, err := CreateOTUIKey(passphrase)
		if err != nil {
			t.Fatalf("CreateOTUIKey(%q): %v", passphrase, err)
		}
		if info, _ := os.Stat(keyPath); info.Mode().Perm() != 0600 {
			t.Errorf("key permissions %v, want 0600", info.Mode().Perm())
		}

		data, _ := os.ReadFile(keyPath)
		var signer ssh.Signer
		if passphrase == "" {
			signer, err = ssh.ParsePrivateKey(data)
		} else {
			var missing *ssh.PassphraseMissingError
			if _, err := ssh.ParsePrivateKey(data); !errors.As(err, &missing) {
				t.Errorf("key isn't encrypted: %v", err)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
		}
		if err != nil {
			t.Fatalf("key doesn't parse: %v", err)
		}
		if signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
			t.Errorf("key type %s", signer.PublicKey().Type())
		}

		pub, _ := os.ReadFile(keyPath + ".pub")
		want := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
		if !strings.HasPrefix(string(pub), want+" otui-encryption-key") {
			t.Errorf(".pub = %q, want %q + comment", pub, want)
		}

		// OTUI can use it for encryption
		em := NewEncryptionManager(EncryptionSSHKey, keyPath)
		em.SetPassphrase(passphrase)
		if err := em.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		enc, err := em.Encrypt([]byte("secret"))
		if err != nil {
			t.Fatal(err)
		}
		if dec, err := em.Decrypt(enc); err != nil || string(dec) != "secret" {
			t.Errorf("round trip: %q, %v", dec, err)
		}

		// Readable by OpenSSH too
		if sshKeygen != "" {
			out, err := exec.Command(sshKeygen, "-y", "-P", passphrase, "-f", keyPath).Output()
			if err != nil || !bytes.HasPrefix(out, []byte(want)) {
				t.Errorf("ssh-keygen -y: %q, %v", out, err)
			}
		}
	}
}

func TestRedact(t *testing.T) {
	for name, want := range map[string]string{"API Key": "****", "GITHUB_PERSONAL_ACCESS_TOKEN": "****",
		"Authorization": "****", "password": "****", "Directory": "/x", "server_url": "/x"} {
		if got := Redact(name, "/x"); got != want {
			t.Errorf("Redact(%q) = %q, want %q", name, got, want)
		}
	}
	if got := EnvNames([]string{"B=2", "A=secret"}); strings.Join(got, ",") != "A,B" {
		t.Errorf("EnvNames = %v", got)
	}
}
