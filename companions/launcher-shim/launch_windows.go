//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// launch spawns exe as a child process and waits for it to exit, then propagates its
// exit code. Windows has no exec-replace primitive the way Unix's execve does, so
// this is the closest equivalent - it keeps a live process around for as long as
// Steam expects to see one, rather than exiting immediately and risking Steam
// thinking the game already closed.
//
// Unverified against real Windows/Steam behavior - there is no Windows machine to
// test this against in this project's own development environment. Treat this path
// as unconfirmed until checked for real; the Linux path in launch_unix.go is the one
// actually tested.
func launch(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	// The real game's own directory, never the shim's - matches ResolveExecutable's
	// own WorkingDir convention in the main module (a game that resolves its own
	// relative paths - a bundled library, its own data files - needs that to be its
	// own install, not wherever the shim happens to be running from).
	cmd.Dir = filepath.Dir(exe)

	runErr := cmd.Run()
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	if runErr != nil {
		return runErr
	}
	os.Exit(0)
	return nil
}
