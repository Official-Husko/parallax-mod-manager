package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
)

// parsableExtensions are the file types worth handing to a parser at all -
// per docs/script-format.md, most Clausewitz-script-adjacent content plus
// localization; binary assets (.dds, shaders, etc.) are deliberately never
// enumerated here.
var parsableExtensions = map[string]bool{
	".txt": true,
	".yml": true,
	".gfx": true,
	".gui": true,
}

// enumerateFiles walks each of scanFolders (in the given order) under root,
// returning parsable files as root-relative paths. Order is deterministic:
// scanFolders in the order given, and within each folder, filepath.WalkDir's
// stable lexical order - this is what lets LoadMod's worker-pool results
// land at fixed slice indices and still merge deterministically.
func enumerateFiles(root string, scanFolders []string) ([]string, error) {
	var files []string
	for _, folder := range scanFolders {
		folderRoot := filepath.Join(root, folder)
		if info, err := os.Stat(folderRoot); err != nil || !info.IsDir() {
			continue // this mod doesn't touch this content folder; not an error
		}

		err := filepath.WalkDir(folderRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !parsableExtensions[filepath.Ext(path)] {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
