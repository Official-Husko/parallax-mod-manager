package modedit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// VersionSnapshot is a lightweight, additive record kept alongside a mod's own save history
// (Store's own save folders), used only to power a version-bump suggestion the next time this
// mod is opened in the Editor. Store's own Apply/Undo know nothing about this file and never
// read or write it - matching the same "bookkeeping stays out of the mechanism it doesn't belong
// to" reasoning already used for internal/translatecache, which lives outside the mod entirely
// for the same kind of reason.
type VersionSnapshot struct {
	Fields           Fields    `json:"fields"`
	ContentFileCount int       `json:"contentFileCount"`
	SavedAt          time.Time `json:"savedAt"`
}

const versionSnapshotFile = "version_snapshot.jsonc"

const versionSnapshotHeader = `// Parallax Mod Manager - this mod's descriptor fields and file count as of one save, kept only
// so the next time it is opened, the Editor can suggest whether the version looks like it needs
// a patch, minor or major bump. Store's own save/undo history (in this same folder) never reads
// this file - see modedit.SuggestBump.
`

// SaveDir is where Apply(writes) at time "at" keeps that save's own files - exported so the
// version-bump feature (or anything else wanting to add its own sidecar file next to a save) can
// find the exact folder Apply itself just used, without duplicating its own naming rule.
func (s Store) SaveDir(at time.Time) string {
	return filepath.Join(s.Dir, at.UTC().Format("20060102T150405.000000000Z"))
}

// WriteVersionSnapshot saves snap into dir (a save's own history folder, see SaveDir) -
// best-effort by design: a caller that fails to write this is expected to log it and move on,
// never fail the save itself, since this is a nicety Undo does not depend on.
func WriteVersionSnapshot(dir string, snap VersionSnapshot) error {
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	_, err = atomicfile.Write(dir, versionSnapshotFile, append([]byte(versionSnapshotHeader), body...))
	return err
}

// LatestVersionSnapshot reads the newest save's own VersionSnapshot, if any - ok is false when
// there is no save yet, or the newest one predates this feature (no sidecar file at all), never
// an error either way; a version-bump suggestion simply has nothing to compare against then.
func (s Store) LatestVersionSnapshot() (VersionSnapshot, bool) {
	names := s.saves()
	if len(names) == 0 {
		return VersionSnapshot{}, false
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, names[len(names)-1], versionSnapshotFile))
	if err != nil {
		return VersionSnapshot{}, false
	}
	var snap VersionSnapshot
	if err := jsonc.Unmarshal(data, &snap); err != nil {
		return VersionSnapshot{}, false
	}
	return snap, true
}

// VersionBumpSuggestion is what SuggestBump found: the current version, what it would become at
// each of the three levels, which one this heuristic suggests, and one sentence saying why.
type VersionBumpSuggestion struct {
	Options   map[BumpKind]Version
	Suggested BumpKind
	Reason    string
}

// SuggestBump compares a mod's descriptor fields and content-file count as of its last save
// (last) against its current ones, and suggests which of the three ordinary bump levels fits.
// This is this app's own heuristic, not a rule handed down anywhere else - TODO.txt's own note
// on this feature only ever sketched the *shape* of patch/minor/major (base 1.2.7 -> minor ->
// 1.3.0, and so on), never what should trigger which:
//
//   - the folders this mod replaces changed, or the made-for-game version's major number
//     changed - either is a much bigger compatibility-relevant change than ordinary content
//     work, so this suggests Major;
//   - the mod's own file count changed, or its tags or dependencies changed - suggests Minor;
//   - anything else that changed at all (a name fix, a version-only edit) - suggests Patch;
//   - nothing changed since the last save - ok is false, since there is nothing to suggest a
//     *next* version from.
//
// ok is also false when currentVersion does not parse as a plain version number (ParseVersion) -
// a suggestion needs a real number to bump from.
func SuggestBump(last VersionSnapshot, currentFields Fields, currentFileCount int, currentVersion string) (VersionBumpSuggestion, bool) {
	cur, ok := ParseVersion(currentVersion)
	if !ok {
		return VersionBumpSuggestion{}, false
	}
	currentFields = currentFields.Normalized()
	lastFields := last.Fields.Normalized()

	fileDelta := currentFileCount - last.ContentFileCount
	tagsChanged := !equalLists(lastFields.Tags, currentFields.Tags)
	depsChanged := !equalLists(lastFields.Dependencies, currentFields.Dependencies)
	replaceChanged := !equalLists(lastFields.ReplacePaths, currentFields.ReplacePaths)
	nameChanged := lastFields.Name != currentFields.Name
	supportedChanged := lastFields.SupportedVersion != currentFields.SupportedVersion
	majorGameVersionChanged := madeForGameVersionMajorChanged(lastFields.SupportedVersion, currentFields.SupportedVersion)

	var kind BumpKind
	var reason string
	switch {
	case replaceChanged:
		kind = BumpMajor
		reason = "The folders this mod replaces changed since " + describeVersion(lastFields.Version) + " was saved."
	case majorGameVersionChanged:
		kind = BumpMajor
		reason = "The made-for game version's major number changed since " + describeVersion(lastFields.Version) + " was saved."
	case fileDelta != 0 || tagsChanged || depsChanged:
		kind = BumpMinor
		reason = minorReason(fileDelta, tagsChanged, depsChanged) + " since " + describeVersion(lastFields.Version) + " was saved."
	case nameChanged || supportedChanged:
		kind = BumpPatch
		reason = "A small change since " + describeVersion(lastFields.Version) + " was saved."
	default:
		return VersionBumpSuggestion{}, false
	}

	return VersionBumpSuggestion{
		Options: map[BumpKind]Version{
			BumpPatch: cur.Bump(BumpPatch),
			BumpMinor: cur.Bump(BumpMinor),
			BumpMajor: cur.Bump(BumpMajor),
		},
		Suggested: kind,
		Reason:    reason,
	}, true
}

func describeVersion(v string) string {
	if v == "" {
		return "the last save"
	}
	return v
}

// minorReason composes the "N files added/removed and tags/dependencies changed" clause -
// whichever of the three actually happened, joined the way a person would say them.
func minorReason(fileDelta int, tagsChanged, depsChanged bool) string {
	var parts []string
	if fileDelta > 0 {
		parts = append(parts, strconv.Itoa(fileDelta)+" file"+plural(fileDelta)+" added")
	} else if fileDelta < 0 {
		n := -fileDelta
		parts = append(parts, strconv.Itoa(n)+" file"+plural(n)+" removed")
	}
	if tagsChanged {
		parts = append(parts, "tags changed")
	}
	if depsChanged {
		parts = append(parts, "dependencies changed")
	}
	switch len(parts) {
	case 0:
		return "Something changed"
	case 1:
		return capitalize(parts[0])
	default:
		return capitalize(strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1])
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// madeForGameVersionMajorChanged says whether a and b's leading number differs - "" or free text
// that does not start with a number never counts as a change on its own (a conservative choice:
// only a real, parseable major-number difference should push a suggestion up to Major).
func madeForGameVersionMajorChanged(a, b string) bool {
	am, aok := leadingNumber(a)
	bm, bok := leadingNumber(b)
	return aok && bok && am != bm
}

func leadingNumber(supportedVersion string) (int, bool) {
	s := strings.TrimSpace(supportedVersion)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
