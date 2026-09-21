// Package checksum calculates the four-character gameplay checksum a Paradox game shows on
// its main menu, offline: the code every player in a multiplayer game must share, so a
// mismatch (a different mod version, a leftover file, another load order) is found before
// anyone starts the game rather than by a failed join.
//
// The game builds it from every file its checksum_manifest.txt names, taken from the game's own
// folder with each enabled mod laid over it, hashed together with the game version. The schemes
// were recovered by studying the games' behaviour and are checked against the real values the
// games print (see docs/checksum.md), so each supported game has its own algorithm here rather
// than one shared guess - the details differ per game (what is hashed, how mods are laid over the
// game, in what order).
package checksum

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Algorithm names one game's checksum scheme.
type Algorithm string

const (
	// Stellaris hashes every file's bytes and its path in one running MD5, then the version
	// once at the end. Mods are laid over the game in load order, a dependency always after what
	// it depends on.
	Stellaris Algorithm = "stellaris"
	// HOI4 hashes each file into its own MD5 (bytes, then path), then those digests and the
	// version into a final MD5. Mods are laid over the game in dependency order, then by
	// registry name, not in the launcher's load order.
	HOI4 Algorithm = "hoi4"
)

// Known says whether a is one of the schemes above.
func (a Algorithm) Known() bool { return a == Stellaris || a == HOI4 }

// Mod is one enabled mod, as the checksum needs to see it.
type Mod struct {
	ID string
	// Name is the mod's display name; other mods name their dependencies by it.
	Name string
	// RegistryID is how the game's own files name the mod ("mod/ugc_123.mod").
	RegistryID string
	// Content is the mod's content folder, or a .zip archive.
	Content      string
	ReplacePaths []string
	Dependencies []string
}

// Input is everything one calculation needs.
type Input struct {
	Algorithm Algorithm
	// GameDir is the game's install folder (where checksum_manifest.txt is).
	GameDir string
	// LauncherSettings is the full path of the game's launcher-settings.json, which carries the
	// game version the checksum is salted with.
	LauncherSettings string
	// ModDir is the game's own mod folder holding the registration files (HOI4 ignores a mod
	// whose name another registration already took).
	ModDir string
	// Mods are the enabled mods in load order.
	Mods []Mod
}

// Result is a finished calculation.
type Result struct {
	Algorithm Algorithm
	// Checksum is what the game shows: the last four hex characters, upper case.
	Checksum string
	// Full is the whole MD5 the four characters are the end of.
	Full string
	// Files is how many files went into it.
	Files int
	// Salt is the version text mixed in.
	Salt string
	// Order lists the mods by name in the order they were laid over the game.
	Order []string
	// Warnings are things worth knowing that did not stop the calculation.
	Warnings []string
}

// MissingContentError means enabled mods have nothing on disk to read, so no honest checksum
// exists: leaving them out would give a code the game never shows.
type MissingContentError struct{ Mods []string }

func (e *MissingContentError) Error() string {
	if len(e.Mods) == 1 {
		return fmt.Sprintf("checksum: the content of '%s' was not found on disk", e.Mods[0])
	}
	return fmt.Sprintf("checksum: the content of %d enabled mods was not found on disk (%s)", len(e.Mods), strings.Join(e.Mods, ", "))
}

// Compute calculates the checksum of the game with the given mods laid over it.
func Compute(ctx context.Context, in Input) (Result, error) {
	if !in.Algorithm.Known() {
		return Result{}, fmt.Errorf("checksum: unknown algorithm %q", in.Algorithm)
	}
	if in.GameDir == "" {
		return Result{}, errors.New("checksum: the game's install folder is unknown")
	}

	var missing []string
	for _, m := range in.Mods {
		if !contentExists(m.Content) {
			missing = append(missing, displayName(m))
		}
	}
	if len(missing) > 0 {
		return Result{}, &MissingContentError{Mods: missing}
	}

	switch in.Algorithm {
	case Stellaris:
		return computeStellaris(ctx, in)
	default:
		return computeHOI4(ctx, in)
	}
}

func contentExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func displayName(m Mod) string {
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	if m.RegistryID != "" {
		return m.RegistryID
	}
	return m.ID
}

// Rule is one checksum_manifest.txt entry: the files of a directory with an extension.
type Rule struct {
	// Directory is the folder under the game or mod root, with forward slashes.
	Directory string
	Extension string
	Recursive bool
}

// ParseManifest reads the game's checksum_manifest.txt: a list of
//
//	directory
//	name = common
//	sub_directories = yes
//	file_extension = .txt
//
// blocks. A block missing its name or extension is dropped.
func ParseManifest(text string) []Rule {
	var rules []Rule
	var cur *Rule
	flush := func() {
		if cur != nil && cur.Directory != "" && cur.Extension != "" {
			rules = append(rules, *cur)
		}
		cur = nil
	}
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if strings.EqualFold(line, "directory") {
			flush()
			cur = &Rule{}
			continue
		}
		if cur == nil || line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "name":
			cur.Directory = normalizeVirtual(value)
		case "file_extension":
			cur.Extension = value
		case "sub_directories":
			cur.Recursive = strings.EqualFold(value, "yes")
		}
	}
	flush()
	return rules
}

// readManifest reads the manifest in gameDir.
func readManifest(gameDir string) ([]Rule, error) {
	data, err := os.ReadFile(filepath.Join(gameDir, "checksum_manifest.txt"))
	if err != nil {
		return nil, err
	}
	rules := ParseManifest(readText(data))
	if len(rules) == 0 {
		return nil, errors.New("checksum: checksum_manifest.txt has no usable rules")
	}
	return rules, nil
}

// readText decodes a text file: UTF-8, with a leading byte order mark dropped.
func readText(data []byte) string {
	return strings.TrimPrefix(string(data), "\xef\xbb\xbf")
}

// normalizeVirtual is a path with forward slashes and no leading or trailing slash: how the
// game names a file inside its virtual file system.
func normalizeVirtual(p string) string {
	return strings.Trim(strings.ReplaceAll(p, `\`, "/"), "/")
}

// underPath says whether p is root or lies below it (case-sensitive).
func underPath(p, root string) bool {
	p, root = normalizeVirtual(p), normalizeVirtual(root)
	if root == "" {
		return false
	}
	return p == root || strings.HasPrefix(p, root+"/")
}

var launcherSettingsField = func(key string) *regexp.Regexp {
	return regexp.MustCompile(`"` + key + `"\s*:\s*"([^"]*)"`)
}

// launcherVersion reads the "version" and "rawVersion" fields of launcher-settings.json.
func launcherVersion(path string) (version, raw string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("checksum: reading the game version: %w", err)
	}
	text := readText(data)
	var parsed struct {
		Version    string `json:"version"`
		RawVersion string `json:"rawVersion"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil {
		version, raw = strings.TrimSpace(parsed.Version), strings.TrimSpace(parsed.RawVersion)
	} else {
		// A launcher file with something the JSON parser refuses: read the two fields as text.
		if m := launcherSettingsField("version").FindStringSubmatch(text); m != nil {
			version = strings.TrimSpace(m[1])
		}
		if m := launcherSettingsField("rawVersion").FindStringSubmatch(text); m != nil {
			raw = strings.TrimSpace(m[1])
		}
	}
	if version == "" {
		return "", "", errors.New("checksum: launcher-settings.json has no version")
	}
	return version, raw, nil
}

var nameRe = regexp.MustCompile(`(?im)^\s*name\s*=\s*(?:"([^"]*)"|([^\s#]+))`)

// descriptorName reads the name of a descriptor's text ("" when it has none).
func descriptorName(text string) string {
	m := nameRe.FindStringSubmatchIndex(text)
	if m == nil {
		return ""
	}
	if m[2] >= 0 {
		return strings.TrimSpace(text[m[2]:m[3]])
	}
	return strings.TrimSpace(text[m[4]:m[5]])
}

var (
	replacePathRe  = regexp.MustCompile(`(?im)^\s*replace_path\s*=\s*"([^"]+)"`)
	dependenciesRe = regexp.MustCompile(`(?ims)(?:^|\s)dependencies\s*=\s*\{(.*?)\}`)
	quotedRe       = regexp.MustCompile(`"([^"]+)"`)
)

// descriptorReplacePaths reads every replace_path of a descriptor's text, normalised by norm.
func descriptorReplacePaths(text string, norm func(string) string) []string {
	var out []string
	for _, m := range replacePathRe.FindAllStringSubmatch(text, -1) {
		v := norm(m[1])
		if v != "" && !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// descriptorDependencies reads the names a descriptor's dependencies block lists.
func descriptorDependencies(text string) []string {
	var out []string
	for _, block := range dependenciesRe.FindAllStringSubmatch(text, -1) {
		for _, m := range quotedRe.FindAllStringSubmatch(block[1], -1) {
			v := strings.TrimSpace(m[1])
			if v != "" && !contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// hashFile streams a file into w.
func hashFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func hexUpper(sum []byte) string { return strings.ToUpper(hex.EncodeToString(sum)) }

// emptyMD5 is the MD5 of no bytes: what HOI4 hashes in place of a file a mod blanked out.
var emptyMD5 = md5.Sum(nil)
