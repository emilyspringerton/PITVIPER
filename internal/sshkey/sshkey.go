// Package sshkey generates and loads a real Ed25519 SSH keypair for PITVIPER's own SSH client
// mode (internal/sshconn), so the app is self-contained -- no separate terminal app (Termux or
// otherwise) is needed just to produce a key.
//
// Founder real-time, 2026-09-06: "the phone app will need a way to generate the key and then
// use it i dont know just how to do that randomly on an android phone." Real, deliberate design:
// the private key is generated ON DEVICE and never leaves it -- LoadOrGenerate only ever returns
// the PUBLIC key line for the caller to display/copy, which is safe to transmit any way (typed,
// photographed, QR'd, texted) since it reveals nothing useful to anyone else.
package sshkey

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// LoadOrGenerate loads an existing Ed25519 private key at path, or generates and saves a fresh
// one if none exists yet (creating parent directories as needed). Returns a signer ready to use
// with sshconn.Config.KeyPath's own ssh.ParsePrivateKey path, the real OpenSSH authorized_keys
// line for the PUBLIC half (safe to display/share), and whether a new key was just generated
// (so the caller knows to show it to the user before anything can actually connect).
func LoadOrGenerate(path string) (pubLine string, generated bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		keyBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", false, fmt.Errorf("sshkey: read %s: %w", path, readErr)
		}
		signer, parseErr := ssh.ParsePrivateKey(keyBytes)
		if parseErr != nil {
			return "", false, fmt.Errorf("sshkey: parse %s: %w", path, parseErr)
		}
		return authorizedKeysLine(signer.PublicKey()), false, nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", false, fmt.Errorf("sshkey: generate: %w", err)
	}

	block, err := ssh.MarshalPrivateKey(priv, "pitviper")
	if err != nil {
		return "", false, fmt.Errorf("sshkey: marshal private key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, fmt.Errorf("sshkey: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", false, fmt.Errorf("sshkey: write %s: %w", path, err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", false, fmt.Errorf("sshkey: derive public key: %w", err)
	}
	return authorizedKeysLine(sshPub), true, nil
}

func authorizedKeysLine(pub ssh.PublicKey) string {
	// TrimSpace-equivalent: ssh.MarshalAuthorizedKey already appends a trailing newline; the
	// caller renders this as one line of terminal/UI text, so strip it here once rather than
	// making every caller remember to.
	line := ssh.MarshalAuthorizedKey(pub)
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	return string(line)
}
