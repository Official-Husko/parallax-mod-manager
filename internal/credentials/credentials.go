// Package credentials is a small, reusable encrypted store for logging into an
// external service on the person's behalf: a username and password for LoversLab
// today, and whatever the next service like it needs tomorrow. Every field is sealed
// with internal/secretbox and bound to this computer - one place for "encrypt this,
// persist it, forget the plaintext once saved" instead of each service's own settings
// code rolling that same handful of lines again (see internal/steamconfig, which
// predates this package and still does exactly that for the Steam Web API key).
//
// A Manager handles exactly one service's fields, each named ("username", "password",
// whatever the service calls for) - what the fields are called and mean is entirely up
// to the caller; this package only ever sees opaque field names and their sealed
// values. Written as JSONC like the app's other settings files, one file per service.
package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

// record is the file's on-disk shape.
type record struct {
	Fields map[string]string `json:"fields"`
}

// Manager seals, opens and persists one external service's saved credentials.
type Manager struct {
	// Service names this credential set, both for the JSONC file's own explanatory
	// comment and to bind every seal to it (see purpose below) - a value sealed for
	// one service's "password" can never be opened as another service's "password",
	// or as a different field of the same service.
	Service string
	// Path is the JSONC file's full path. Empty means there is nowhere to save (the
	// app's config folder could not be found) - Save and Clear fail, Load and Open
	// behave as if nothing were ever saved.
	Path string
	// box does the actual sealing/opening; nil means encryption is not ready yet
	// (mirrors steamAPIState's own svc == nil check before it is configured).
	Box *secretbox.Box
}

// purpose binds a seal to this Manager's service and to one named field within it.
func (m *Manager) purpose(field string) string {
	return "credentials:" + m.Service + ":" + field
}

// load reads the sealed fields as they are on disk (still sealed). A missing file is
// not an error, just an empty record - the same "no file yet" tolerance every other
// settings file in this app already has.
func (m *Manager) load() (record, error) {
	if m.Path == "" {
		return record{Fields: map[string]string{}}, nil
	}
	data, err := os.ReadFile(m.Path)
	if os.IsNotExist(err) {
		return record{Fields: map[string]string{}}, nil
	}
	if err != nil {
		return record{}, fmt.Errorf("credentials: reading %s: %w", m.Path, err)
	}
	var rec record
	if err := jsonc.Unmarshal(data, &rec); err != nil {
		return record{}, fmt.Errorf("credentials: %s is not valid: %w", m.Path, err)
	}
	if rec.Fields == nil {
		rec.Fields = map[string]string{}
	}
	return rec, nil
}

// Has reports whether field is saved, without decrypting it - the right check for a
// field that should never be read back at all (a password), matching how the Steam
// panel only ever asks "is a key saved", never "what is it".
func (m *Manager) Has(field string) (bool, error) {
	rec, err := m.load()
	if err != nil {
		return false, err
	}
	_, ok := rec.Fields[field]
	return ok, nil
}

// HasAny reports whether anything at all is saved for this service.
func (m *Manager) HasAny() (bool, error) {
	rec, err := m.load()
	if err != nil {
		return false, err
	}
	return len(rec.Fields) > 0, nil
}

// Open decrypts one saved field - for a field the UI is allowed to show again (a
// username, unlike a password). secretbox.ErrCannotOpen (wrapped) means it was sealed
// on another computer or the file was altered since; callers treat the field as
// absent and ask for it again, the same way an unreadable Steam API key is handled.
func (m *Manager) Open(field string) (string, error) {
	rec, err := m.load()
	if err != nil {
		return "", err
	}
	sealed, ok := rec.Fields[field]
	if !ok {
		return "", fmt.Errorf("credentials: %s has no saved %q", m.Service, field)
	}
	if m.Box == nil {
		return "", fmt.Errorf("credentials: encryption is not ready yet")
	}
	return m.Box.Open(sealed, m.purpose(field))
}

// Save seals and persists every given field, adding or replacing only those field
// names - a field not named here is left exactly as it was, so saving a new password
// never drops an already-saved username.
func (m *Manager) Save(fields map[string]string) error {
	if m.Path == "" {
		return fmt.Errorf("credentials: there is no settings folder to save into")
	}
	if m.Box == nil {
		return fmt.Errorf("credentials: encryption is not ready yet")
	}
	rec, err := m.load()
	if err != nil {
		return err
	}
	for name, value := range fields {
		sealed, err := m.Box.Seal(value, m.purpose(name))
		if err != nil {
			return fmt.Errorf("credentials: sealing %q: %w", name, err)
		}
		rec.Fields[name] = sealed
	}
	return m.write(rec)
}

// Clear deletes every saved field for this service.
func (m *Manager) Clear() error {
	if m.Path == "" {
		return nil
	}
	return m.write(record{Fields: map[string]string{}})
}

func (m *Manager) write(rec record) error {
	dir, file := filepath.Dir(m.Path), filepath.Base(m.Path)
	if _, err := atomicfile.Write(dir, file, render(m.Service, rec)); err != nil {
		return fmt.Errorf("credentials: saving: %w", err)
	}
	return nil
}

// render writes rec as hand-editable JSONC - a comment next to the one thing worth
// explaining (that these values are sealed, not the real ones), matching every other
// settings file this app writes. Field order is sorted for a stable, diffable file.
func render(service string, rec record) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Parallax Mod Manager - saved sign-in for %s.\n", service)
	b.WriteString("//\n" +
		"// Every value below is ENCRYPTED with a key derived from this computer. None of them\n" +
		"// are the real value: copying this file elsewhere, or sharing it, does not reveal what\n" +
		"// you signed in with. You normally change this from within the app rather than by hand.\n" +
		"{\n" +
		"  \"fields\": {")
	names := make([]string, 0, len(rec.Fields))
	for name := range rec.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "\n    %s: %s", quote(name), quote(rec.Fields[name]))
	}
	if len(names) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("}\n}\n")
	return []byte(b.String())
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
