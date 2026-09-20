// Package backgrounds handles the rotating per-game background art: where it is
// published, listing it, downloading it for offline use, and serving the offline
// copies to the UI.
//
// The images are not part of the app. They live in a GitHub repository, one
// folder per game named by the game's id, and are either streamed from there as
// they are needed (online) or downloaded once into the user's config folder and
// used from there (offline). Bundling them into the binary made it several
// hundred megabytes for art most people would never see all of.
//
// Everything that touches the network takes its base addresses as fields, so the
// tests can point it at a local server.
package backgrounds

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Source says where the published backgrounds are: a folder in a GitHub
// repository that holds one subfolder per game, named by the game's id, each
// with that game's images.
type Source struct {
	// Repo is "owner/name".
	Repo string `json:"repo"`
	// Branch is the branch (or tag) the images are read from.
	Branch string `json:"branch"`
	// Path is the folder inside the repository holding the game folders.
	Path string `json:"path"`
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Validate says what is wrong with s, if anything. The three values end up in URLs
// and are typed by hand in a file, so they are checked rather than trusted.
func (s Source) Validate() error {
	if !repoPattern.MatchString(s.Repo) {
		return fmt.Errorf("repo %q must look like owner/name", s.Repo)
	}
	if s.Branch == "" || strings.ContainsAny(s.Branch, " \t\r\n?#\\") || strings.Contains(s.Branch, "..") {
		return fmt.Errorf("branch %q is not usable", s.Branch)
	}
	if s.Path == "" || strings.HasPrefix(s.Path, "/") || strings.HasSuffix(s.Path, "/") ||
		strings.ContainsAny(s.Path, "\\?#:") || path.Clean(s.Path) != s.Path || strings.HasPrefix(s.Path, "..") {
		return fmt.Errorf("path %q must be a plain folder path inside the repository", s.Path)
	}
	return nil
}

// TreePath is the GitHub API request path that lists everything under Path
// (with sizes) in one call.
func (s Source) TreePath() string {
	return fmt.Sprintf("/repos/%s/git/trees/%s:%s?recursive=1", s.Repo, s.Branch, s.Path)
}

// RawURL is where one image can be downloaded from. base is the raw-content host
// (see Fetcher.RawBase); the name is escaped, since real ones contain spaces.
func (s Source) RawURL(base, gameID, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s", strings.TrimRight(base, "/"), s.Repo, s.Branch, s.Path, gameID, escapeName(name))
}

func escapeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '~':
			b.WriteRune(r)
		default:
			for _, c := range []byte(string(r)) {
				fmt.Fprintf(&b, "%%%02X", c)
			}
		}
	}
	return b.String()
}

// LoadSource reads the built-in source, then lets a file in the user's config
// folder replace it - the same embedded default plus live override the games list
// uses, so the images can be hosted elsewhere (a fork, a mirror) without a rebuild.
// An override that cannot be read or is not valid is ignored and the returned
// notice says why; the built-in one must be valid (a build-time mistake otherwise).
func LoadSource(embedded []byte, override []byte) (Source, string, error) {
	var src Source
	if err := jsonc.Unmarshal(embedded, &src); err != nil {
		return Source{}, "", fmt.Errorf("backgrounds: built-in source is invalid: %w", err)
	}
	if err := src.Validate(); err != nil {
		return Source{}, "", fmt.Errorf("backgrounds: built-in source is invalid: %w", err)
	}
	if len(override) == 0 {
		return src, "", nil
	}
	var custom Source
	if err := jsonc.Unmarshal(override, &custom); err != nil {
		return src, fmt.Sprintf("backgrounds.jsonc could not be read (%v); using the built-in source", err), nil
	}
	if err := custom.Validate(); err != nil {
		return src, fmt.Sprintf("backgrounds.jsonc is not usable (%v); using the built-in source", err), nil
	}
	return custom, "", nil
}
