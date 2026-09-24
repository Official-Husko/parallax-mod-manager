//go:build !windows

package steamworks

import "github.com/ebitengine/purego"

// openLibrary dlopen's path - see platform_windows.go for the Windows
// equivalent (purego itself has no Dlopen on Windows and says so
// explicitly: LoadLibrary is the documented replacement there).
func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

// symbolExists reports whether name is a real, resolvable exported symbol in
// the library behind handle, without registering it as a callable function -
// used to probe candidate interface-accessor version names (see versions.go)
// and to verify every required symbol exists before RegisterLibFunc, which
// panics rather than erroring on a missing one.
func symbolExists(handle uintptr, name string) bool {
	_, err := purego.Dlsym(handle, name)
	return err == nil
}
