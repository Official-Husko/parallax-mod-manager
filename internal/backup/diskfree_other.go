//go:build !linux && !darwin && !windows

package backup

// diskFree has no source on this platform: the free-space check is skipped.
func diskFree(path string) (uint64, bool) { return 0, false }
