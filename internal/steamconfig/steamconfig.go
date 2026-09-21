// Package steamconfig persists the optional Steam Web API key settings: which mode
// the key is used in and the key itself, encrypted (see internal/secretbox).
//
// Written as JSONC like the app's other settings files, with a comment beside every
// field, so a person opening it can see what it is - and that the key is not there in
// the clear.
package steamconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// FileName is the settings file's name in the app's config folder.
const FileName = "steam_api.jsonc"

// Settings is what is kept on disk.
type Settings struct {
	Mode steamapi.Mode `json:"mode"`
	// SealedKey is the API key encrypted by secretbox ("v1.<nonce>.<ciphertext>"), or
	// empty when no key is saved.
	SealedKey string `json:"key"`
	// Fingerprint is the first 8 hex characters of the SHA-256 of the key: it lets the
	// UI say which key is saved without decrypting anything.
	Fingerprint string `json:"fingerprint"`
}

// Defaults is the state of a fresh install: free API only, no key.
func Defaults() Settings { return Settings{Mode: steamapi.ModeFree} }

// Store reads and writes the settings file. Path is the full file path; empty means
// there is nowhere to keep it (the config folder could not be found).
type Store struct {
	Path string
}

// Load reads the file. A missing file is not an error (it is Defaults). A file that
// cannot be read or parsed returns Defaults together with the error, so the caller can
// log it and carry on with the safe state instead of failing to start.
func (s Store) Load() (Settings, error) {
	if s.Path == "" {
		return Defaults(), nil
	}
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), fmt.Errorf("steamconfig: reading %s: %w", s.Path, err)
	}
	var raw struct {
		Mode        string `json:"mode"`
		Key         string `json:"key"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := jsonc.Unmarshal(data, &raw); err != nil {
		return Defaults(), fmt.Errorf("steamconfig: %s is not valid: %w", s.Path, err)
	}
	st := Settings{Mode: steamapi.ParseMode(raw.Mode), SealedKey: strings.TrimSpace(raw.Key), Fingerprint: raw.Fingerprint}
	return normalise(st), nil
}

// Save writes the settings atomically, readable by the owner only. Free mode never
// keeps a key: saving it removes the key from the file, which is what choosing "free
// API only" promises.
func (s Store) Save(st Settings) error {
	if s.Path == "" {
		return fmt.Errorf("steamconfig: there is no settings folder to save into")
	}
	st = normalise(st)
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("steamconfig: %w", err)
	}
	if _, err := atomicfile.Write(filepath.Dir(s.Path), filepath.Base(s.Path), render(st)); err != nil {
		return fmt.Errorf("steamconfig: saving: %w", err)
	}
	return nil
}

// normalise enforces the invariants: free mode holds no key, and a key needs a
// fingerprint (or neither).
func normalise(st Settings) Settings {
	st.Mode = steamapi.ParseMode(string(st.Mode))
	if !st.Mode.UsesKey() || st.SealedKey == "" {
		st.SealedKey, st.Fingerprint = "", ""
	}
	if st.SealedKey == "" && st.Mode.UsesKey() {
		// A key mode without a key is meaningless: fall back to the safe default.
		st.Mode = steamapi.ModeFree
	}
	return st
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// render writes the file with a comment beside each field.
func render(st Settings) []byte {
	var b strings.Builder
	b.WriteString(`// Parallax Mod Manager - Steam Web API settings.
//
// Everything here is optional. Without a key the app asks Steam's free, keyless API for
// Workshop details, which works for most mods. A key lets it also get the ones the free
// API cannot return (unlisted items, for one).
//
// You normally change this in Settings > Steam API rather than by hand.
{
  // How the key is used:
  //   "free"     - never use a key; only the free API. (No key is kept in this mode.)
  //   "complete" - use the key for all Workshop details, and the free API if the key runs out.
  //   "backup"   - use the free API, and the key only for what the free API cannot return.
  "mode": `)
	b.WriteString(quote(string(st.Mode)))
	b.WriteString(`,

  // Your Steam Web API key, ENCRYPTED with a key derived from this computer. It is not the
  // key itself and it cannot be opened on another computer: copying this file elsewhere,
  // or sharing it, does not reveal your key. Empty when no key is saved. Do not edit.
  "key": `)
	b.WriteString(quote(st.SealedKey))
	b.WriteString(`,

  // The first 8 characters of a hash of your key, only so the app can show which key is
  // saved. It cannot be turned back into the key.
  "fingerprint": `)
	b.WriteString(quote(st.Fingerprint))
	b.WriteString("\n}\n")
	return []byte(b.String())
}
