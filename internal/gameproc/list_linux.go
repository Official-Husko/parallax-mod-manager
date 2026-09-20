package gameproc

import (
	"os"
	"strconv"
	"strings"
)

// list reads /proc. A process is identified by the first word of its command
// line - the name it was started as, which for a game running under Wine is a
// Windows path - and by its short process name; nothing else on the command line
// is looked at (see the package comment).
func list() ([]Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, 256)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		argv0, _, _ := strings.Cut(string(raw), "\x00")
		name := baseName(argv0)
		if name == "" {
			// No command line: a kernel thread, or a zombie - a process that has
			// exited but whose parent hasn't collected it yet, and which keeps its
			// name in /proc until it does. A game that has closed must not count as
			// still running.
			continue
		}
		names := []string{name}
		if comm, err := os.ReadFile("/proc/" + e.Name() + "/comm"); err == nil {
			if short := strings.TrimSpace(string(comm)); short != "" && short != name {
				names = append(names, short)
			}
		}
		out = append(out, Process{PID: pid, Names: names})
	}
	return out, nil
}
