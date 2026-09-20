//go:build !windows

package gameproc

import "syscall"

// canAsk: a Unix process can be asked to stop, and given time to.
const canAsk = true

// terminate asks pid to stop (SIGTERM), or with force ends it (SIGKILL).
func terminate(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	err := syscall.Kill(pid, sig)
	if err == syscall.ESRCH {
		return nil // already gone
	}
	return err
}
