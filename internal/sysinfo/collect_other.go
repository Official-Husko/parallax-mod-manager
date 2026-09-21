//go:build !linux && !windows && !darwin

package sysinfo

import "runtime"

// collect knows nothing about this platform beyond what the Go runtime says.
func collect() Info { return Info{OS: runtime.GOOS} }
