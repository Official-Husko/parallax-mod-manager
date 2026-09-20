package backgrounds

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// File is one image: its name in the game's folder and its size in bytes.
type File struct {
	Name string
	Size int64
}

var gameIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)

// imageExts are the file types treated as backgrounds.
var imageExts = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp",
}

// partSuffix marks a download still in progress; such files are never listed.
const partSuffix = ".part"

// ValidGameID says whether id can name a game's folder: ids come from the games
// list (UUIDs) and from the network, so anything that could climb out of the
// folder or is otherwise odd is refused.
func ValidGameID(id string) bool { return gameIDPattern.MatchString(id) }

// ValidName says whether name is a plain image file name that is safe to create
// in a game's folder and to put in a URL.
func ValidName(name string) bool {
	if name == "" || len(name) > 200 || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.ContainsAny(name, "/\\:") || strings.HasSuffix(name, partSuffix) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	_, ok := imageExts[strings.ToLower(filepath.Ext(name))]
	return ok
}

// MimeType returns the content type for an image file name ("" if it is not one).
func MimeType(name string) string { return imageExts[strings.ToLower(filepath.Ext(name))] }

// Store is the offline copy: <Dir>/<gameID>/<file>. It is what "offline" reads,
// and anything a user drops into a game's folder by hand counts too. Dir empty
// (the config folder could not be resolved) makes every method a no-op.
type Store struct {
	Dir string
}

// GameDir is where gameID's images live ("" when the id or the store is unusable).
func (s Store) GameDir(gameID string) string {
	if s.Dir == "" || !ValidGameID(gameID) {
		return ""
	}
	return filepath.Join(s.Dir, gameID)
}

// Path is the location of one image, refused unless both parts are valid.
func (s Store) Path(gameID, name string) (string, bool) {
	dir := s.GameDir(gameID)
	if dir == "" || !ValidName(name) {
		return "", false
	}
	return filepath.Join(dir, name), true
}

// List returns gameID's images on disk, sorted by name.
func (s Store) List(gameID string) []File {
	dir := s.GameDir(gameID)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, File{Name: e.Name(), Size: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

// Has says whether f is already on disk with the size it should have.
func (s Store) Has(gameID string, f File) bool {
	p, ok := s.Path(gameID, f.Name)
	if !ok {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular() && info.Size() == f.Size
}

// Usage totals gameID's images on disk.
func (s Store) Usage(gameID string) (files int, bytes int64) {
	for _, f := range s.List(gameID) {
		files++
		bytes += f.Size
	}
	return files, bytes
}

// Games lists the games that have a folder in the store.
func (s Store) Games() []string {
	if s.Dir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && ValidGameID(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

// RemovePack deletes gameID's whole folder. The id is validated first, so this can
// only ever remove a folder directly inside Dir.
func (s Store) RemovePack(gameID string) error {
	dir := s.GameDir(gameID)
	if dir == "" {
		return os.ErrInvalid
	}
	return os.RemoveAll(dir)
}
