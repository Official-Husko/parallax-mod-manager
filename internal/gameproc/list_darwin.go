package gameproc

import (
	"os/exec"
	"strconv"
	"strings"
)

// list asks ps for every process and the path of its executable.
func list() ([]Process, error) {
	raw, err := exec.Command("ps", "-axo", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	var out []Process
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		pidText, path, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		if name := baseName(path); name != "" {
			out = append(out, Process{PID: pid, Names: []string{name}})
		}
	}
	return out, nil
}
