//go:build linux || darwin

package backup

import "golang.org/x/sys/unix"

// diskFree returns the bytes available to this user on the volume holding path.
func diskFree(path string) (uint64, bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
