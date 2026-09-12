// Package atomicfile writes JSON files atomically (temp file in the same
// directory, then rename) so a crash mid-write can never leave a
// half-written, corrupt file behind. Extracted from internal/cache and
// internal/launch, which each independently implemented this exact
// pattern - see docs/architecture-notes.md's atomic-write convention.
package atomicfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSON marshals v as indented JSON and writes it to dir/filename
// atomically. Returns the final absolute path on success.
func WriteJSON(dir, filename string, v any) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("atomicfile: creating %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("atomicfile: encoding %s: %w", filename, err)
	}

	final := filepath.Join(dir, filename)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("atomicfile: creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("atomicfile: writing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("atomicfile: closing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		return "", fmt.Errorf("atomicfile: renaming %s to %s: %w", tmpPath, final, err)
	}
	return final, nil
}
