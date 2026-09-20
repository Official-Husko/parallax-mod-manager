//go:build !linux && !darwin && !windows

package gameproc

import "errors"

// list has no way to enumerate processes on this platform.
func list() ([]Process, error) {
	return nil, errors.New("gameproc: finding running games isn't supported on this platform")
}
