package gameproc

import "os"

// canAsk: there is no polite way to stop a process from outside on Windows
// short of driving its windows, so Stop ends it outright.
const canAsk = false

func terminate(pid int, force bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer p.Release()
	return p.Kill()
}
