package gameproc

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// list takes a snapshot of the running processes.
func list() ([]Process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	var out []Process
	for {
		if name := windows.UTF16ToString(entry.ExeFile[:]); name != "" {
			out = append(out, Process{PID: int(entry.ProcessID), Names: []string{name}})
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break // ERROR_NO_MORE_FILES: the end of the list
		}
	}
	return out, nil
}
