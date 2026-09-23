//go:build !windows

package main

import (
	"os"
	"syscall"
)

// launch replaces this process's own image with exe (same PID, never returns on
// success) - the cleanest way to keep Steam's own context (environment variables,
// overlay hookup) exactly as it set it up for what it still thinks is the launcher.
// Confirmed against this project's own real Stellaris and Hearts of Iron IV installs
// (Linux, native binaries, no Proton layer involved).
func launch(exe string, args []string) error {
	argv := append([]string{exe}, args...)
	return syscall.Exec(exe, argv, os.Environ())
}
