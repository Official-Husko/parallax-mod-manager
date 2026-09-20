//go:build !linux

package gameproc

import "os"

func selfPID() int { return os.Getpid() }
