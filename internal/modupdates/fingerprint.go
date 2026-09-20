package modupdates

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FingerprintDir summarises everything under root. A root that cannot be read
// gives an unknown fingerprint (see Fingerprint.Known); an unreadable branch
// inside it is skipped, so one bad file never hides the rest.
func FingerprintDir(root string) Fingerprint {
	if root == "" {
		return Fingerprint{}
	}
	if _, err := os.Stat(root); err != nil {
		return Fingerprint{}
	}
	f := Fingerprint{Known: true}
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f.Files++
		f.Size += info.Size()
		if t := info.ModTime().Unix(); t > f.Newest {
			f.Newest = t
		}
		return nil
	})
	return f
}
