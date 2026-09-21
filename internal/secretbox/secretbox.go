// Package secretbox encrypts a small secret (the Steam Web API key) for storing in
// a settings file, so the file alone does not reveal it.
//
// The encryption key is derived from an identifier of this computer (its machine id,
// see machineid_*.go) with HKDF-SHA256 and the secret is sealed with AES-256-GCM.
// The file is therefore useless anywhere else: copied to another machine, attached to
// a bug report or synced to a cloud drive, it cannot be opened.
//
// What this does not do: stop a program running as the same user on the same computer
// from asking this app's own code for the key. Nothing on disk can, since the program
// has to be able to decrypt it to use it. The file is also written readable by its
// owner only.
//
// A hash would be the wrong tool for the stored value: a hash cannot be turned back
// into the key it came from, and the app has to send the real key to Steam. A hash is
// only good for showing which key is saved - see Fingerprint.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// formatVersion prefixes every sealed value, so the scheme can change later without
// old values becoming ambiguous.
const formatVersion = "v1"

// hkdfSalt and hkdfInfo pin the derived key to this app and this use, so the same
// machine id yields unrelated keys for unrelated purposes.
var (
	hkdfSalt = []byte("parallax-mod-manager/secretbox/salt/v1")
	hkdfInfo = "parallax-mod-manager secretbox key v1"
)

// ErrCannotOpen means a sealed value could not be decrypted: it was sealed on another
// computer, was altered, or is not a sealed value at all. Callers treat the secret as
// absent and ask for it again.
var ErrCannotOpen = errors.New("secretbox: the value cannot be decrypted on this computer")

// Box seals and opens values with one derived key.
type Box struct {
	aead cipher.AEAD
}

// New derives a Box from secret, which must be non-empty.
func New(secret []byte) (*Box, error) {
	if len(secret) == 0 {
		return nil, errors.New("secretbox: empty secret")
	}
	key, err := hkdf.Key(sha256.New, secret, hkdfSalt, hkdfInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("secretbox: deriving key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// ForThisMachine returns a Box bound to this computer: keyed by its machine id, or -
// where the operating system offers none we can read - by a random secret kept in a
// file in fallbackDir (created readable by its owner only on first use).
func ForThisMachine(fallbackDir string) (*Box, error) {
	if id := machineID(); id != "" {
		return New([]byte("machine-id:" + id))
	}
	secret, err := fallbackSecret(fallbackDir)
	if err != nil {
		return nil, err
	}
	return New(secret)
}

// fallbackSecret loads or creates the random per-install secret.
func fallbackSecret(dir string) ([]byte, error) {
	if dir == "" {
		return nil, errors.New("secretbox: no machine id and no folder for a fallback secret")
	}
	path := filepath.Join(dir, "machine_secret.key")
	if data, err := os.ReadFile(path); err == nil {
		if secret, err := hex.DecodeString(strings.TrimSpace(string(data))); err == nil && len(secret) == 32 {
			return append([]byte("file-secret:"), secret...), nil
		}
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("secretbox: writing the fallback secret: %w", err)
	}
	return append([]byte("file-secret:"), secret...), nil
}

// Seal encrypts plaintext. purpose is bound into the result (as authenticated data),
// so a value sealed for one use cannot be opened as another. Every call yields a
// different string for the same input (a fresh random nonce).
func (b *Box) Seal(plaintext, purpose string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secretbox: %w", err)
	}
	ct := b.aead.Seal(nil, nonce, []byte(plaintext), []byte(purpose))
	enc := base64.RawURLEncoding
	return formatVersion + "." + enc.EncodeToString(nonce) + "." + enc.EncodeToString(ct), nil
}

// Open decrypts a value Seal made with the same purpose on the same computer. Any
// failure - another computer, a changed file, a malformed value - is ErrCannotOpen.
func (b *Box) Open(sealed, purpose string) (string, error) {
	parts := strings.Split(strings.TrimSpace(sealed), ".")
	if len(parts) != 3 || parts[0] != formatVersion {
		return "", ErrCannotOpen
	}
	enc := base64.RawURLEncoding
	nonce, err := enc.DecodeString(parts[1])
	if err != nil || len(nonce) != b.aead.NonceSize() {
		return "", ErrCannotOpen
	}
	ct, err := enc.DecodeString(parts[2])
	if err != nil {
		return "", ErrCannotOpen
	}
	plain, err := b.aead.Open(nil, nonce, ct, []byte(purpose))
	if err != nil {
		return "", ErrCannotOpen
	}
	return string(plain), nil
}

// Fingerprint is the first 8 hex characters of the SHA-256 of secret: enough to tell
// which key is saved, far too little to recover it.
func Fingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])[:8]
}
