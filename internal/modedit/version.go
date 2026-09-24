package modedit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a mod's version string split into the numbers most Paradox mods' own version=
// values actually use (e.g. "3.2", "1.0.0") - a best-effort parse, not a strict semver
// implementation, since nothing requires an author to follow one.
type Version struct {
	Major, Minor, Patch int
	// HadPatch is false when the source string had only two numbers (e.g. "3.2") - String then
	// keeps that same two-number shape unless a Patch bump specifically introduces a third
	// number (see Bump).
	HadPatch bool
}

// versionShape accepts an optional leading "v" and two required numbers plus an optional third -
// deliberately stricter than fields.go's own SupportedVersionShape (which also allows a trailing
// "*"), since a real version= value is never a wildcard.
var versionShape = regexp.MustCompile(`^[vV]?(\d+)\.(\d+)(?:\.(\d+))?$`)

// ParseVersion parses s as Major.Minor[.Patch], best-effort. ok is false for anything that does
// not look like a plain version number at all (blank, a wildcard, free text) - the version-bump
// feature simply offers no suggestion then, rather than guessing at a version that is not really
// numeric.
func ParseVersion(s string) (Version, bool) {
	m := versionShape.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	v := Version{Major: major, Minor: minor}
	if m[3] != "" {
		v.Patch, _ = strconv.Atoi(m[3])
		v.HadPatch = true
	}
	return v, true
}

// BumpKind is one of the three ordinary version-bump levels.
type BumpKind string

const (
	BumpPatch BumpKind = "patch"
	BumpMinor BumpKind = "minor"
	BumpMajor BumpKind = "major"
)

// Bump returns v incremented at kind, resetting the levels below it - the ordinary rule (1.2.7
// bumped minor is 1.3.0, not 1.3.7). A Patch bump on a version with no third number yet (e.g.
// "3.2") introduces one rather than being a no-op ("3.2" -> Patch -> "3.2.1"); Minor and Major
// keep whatever two- or three-number shape the version already had.
func (v Version) Bump(kind BumpKind) Version {
	switch kind {
	case BumpMajor:
		return Version{Major: v.Major + 1, HadPatch: v.HadPatch}
	case BumpMinor:
		return Version{Major: v.Major, Minor: v.Minor + 1, HadPatch: v.HadPatch}
	default:
		return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1, HadPatch: true}
	}
}

// String renders v back out, keeping the same two- or three-number shape it was parsed from (or
// that Bump left it in).
func (v Version) String() string {
	if v.HadPatch {
		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}
