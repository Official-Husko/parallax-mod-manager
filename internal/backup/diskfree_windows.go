package backup

import "golang.org/x/sys/windows"

// diskFree returns the bytes available to this user on the volume holding path.
func diskFree(path string) (uint64, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, false
	}
	return free, true
}
