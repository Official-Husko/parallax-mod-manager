//go:build windows

package steamworks

import "golang.org/x/sys/windows"

// openLibrary loads path via LoadLibrary - purego has no Dlopen equivalent
// on Windows and says so explicitly; golang.org/x/sys/windows is the
// documented replacement (see platform_unix.go for the Unix side).
func openLibrary(path string) (uintptr, error) {
	h, err := windows.LoadLibrary(path)
	return uintptr(h), err
}

// symbolExists mirrors platform_unix.go's own doc comment - GetProcAddress
// is Windows' equivalent of dlsym.
func symbolExists(handle uintptr, name string) bool {
	_, err := windows.GetProcAddress(windows.Handle(handle), name)
	return err == nil
}
