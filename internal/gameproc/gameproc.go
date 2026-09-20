// Package gameproc finds a running game and stops it.
//
// A game started through Steam is not this app's child process - Steam (and,
// on Linux, Proton) starts it - so there is no handle to keep. What is left is
// to recognise the game among the operating system's processes, which is what
// Find does, and to end what it recognised, which is what Stop does.
//
// Recognising is deliberately strict: a process counts only if its own
// executable's file name is one the caller asked for. It is never a substring
// of the command line, because that would also match a file manager or a
// terminal that merely has the game's mod folder open - and Stop would then
// kill it.
package gameproc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Process is one running operating system process.
type Process struct {
	PID int
	// Names are the ways the process identifies its executable: the file name
	// it was started as, and where the OS reports another (Linux's short
	// process name), that too. Never a command-line argument.
	Names []string
}

// Status is whether a game is running, and which processes are it.
type Status struct {
	Running bool
	// PIDs is never nil (it marshals to an empty list, not null).
	PIDs []int
}

// ExecutableNames returns the names a game whose executable is at exePath can
// run under: the file name as given, and the same with the Windows ".exe"
// added or removed. The second form is what a Windows game running under
// Proton shows as - the Linux install's "stellaris" is "stellaris.exe" there.
// Empty when exePath names nothing.
func ExecutableNames(exePath string) []string {
	base := baseName(exePath)
	if base == "" || base == "." {
		return nil
	}
	if trimmed, ok := cutExeSuffix(base); ok {
		if trimmed == "" {
			return []string{base}
		}
		return []string{base, trimmed}
	}
	return []string{base, base + ".exe"}
}

// ExecutableNamesFor is ExecutableNames for the executable at exePath, and also
// for whatever that file starts if it is a launcher script.
//
// That matters because a script isn't the game: Hearts of Iron IV's launcher
// settings name "run_hoi4", a shell script that runs the real "hoi4" binary as a
// child. Stopping only the script would leave the game running, and the script
// alone would not be what a game "is running" means. So every program the script
// mentions that exists next to it is a name the game can run as too.
func ExecutableNamesFor(exePath string) []string {
	names := ExecutableNames(exePath)
	if names == nil {
		return nil
	}
	for _, program := range scriptPrograms(exePath) {
		for _, n := range ExecutableNames(program) {
			if !containsFold(names, n) {
				names = append(names, n)
			}
		}
	}
	return names
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// pathedName matches the file name at the end of a path in a script:
// "$GAME_DIR/hoi4", ./bin/game, "${DIR}/game".
var pathedName = regexp.MustCompile(`[/\\]([A-Za-z0-9_.+-]+)`)

// maxScriptRead bounds how much of a launcher script is looked at. They are a few
// lines long; anything bigger is not one.
const maxScriptRead = 16 << 10

// scriptPrograms returns the names of the programs the script at path starts, if it
// is a script: the files it refers to by path that exist in its own folder, other
// than itself. Restricting to files that really exist there is what keeps this
// from picking up "sh" or "env" from a "/usr/bin/env" line.
func scriptPrograms(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, maxScriptRead)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if !bytes.HasPrefix(buf, []byte("#!")) {
		return nil
	}

	dir, self := filepath.Dir(path), filepath.Base(path)
	var out []string
	for _, m := range pathedName.FindAllSubmatch(buf, -1) {
		name := string(m[1])
		if name == self || containsFold(out, name) {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			out = append(out, name)
		}
	}
	return out
}

func cutExeSuffix(name string) (string, bool) {
	if len(name) >= 4 && strings.EqualFold(name[len(name)-4:], ".exe") {
		return name[:len(name)-4], true
	}
	return name, false
}

// baseName is the last element of p, whichever separator it uses: a process
// started through Wine reports a Windows path ("Z:\games\stellaris.exe") even
// on Linux, where filepath.Base would treat the whole thing as one name.
func baseName(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// Match returns the processes among procs that go by one of names, ignoring
// case (Windows file names are case-insensitive, and Wine keeps whatever case
// the game was installed with). procs are left in the order given.
func Match(procs []Process, names []string) []Process {
	if len(names) == 0 {
		return nil
	}
	var out []Process
	for _, p := range procs {
		if goesBy(p, names) {
			out = append(out, p)
		}
	}
	return out
}

// commMax is the longest short process name Linux keeps (TASK_COMM_LEN - 1). A
// longer name is cut to this many characters, which is how a launcher script shows
// up while it runs.
const commMax = 15

func goesBy(p Process, names []string) bool {
	for _, have := range p.Names {
		if have == "" {
			continue
		}
		for _, want := range names {
			if strings.EqualFold(have, want) {
				return true
			}
			// A name at the cut-off length may be the front of a longer one.
			if len(have) == commMax && len(want) > commMax && strings.EqualFold(have, want[:commMax]) {
				return true
			}
		}
	}
	return false
}

// Find returns the running processes that go by one of names. This app's own
// process is never among them.
func Find(names []string) ([]Process, error) {
	if len(names) == 0 {
		return nil, nil
	}
	all, err := list()
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	found := Match(all, names)
	out := found[:0]
	for _, p := range found {
		if p.PID != self {
			out = append(out, p)
		}
	}
	return out, nil
}

// Check reports whether a process going by one of names is running.
func Check(names []string) (Status, error) {
	procs, err := Find(names)
	if err != nil {
		return Status{PIDs: []int{}}, err
	}
	pids := make([]int, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, p.PID)
	}
	return Status{Running: len(pids) > 0, PIDs: pids}, nil
}

var errStillRunning = errors.New("gameproc: the game is still running")

// stopPoll is how often Stop looks again while it waits for a process to go.
const stopPoll = 50 * time.Millisecond

// Stop ends every running process that goes by one of names, and returns how
// many it found running.
//
// It asks first, where the OS has a way to (SIGTERM), so a game gets the chance
// to close its files, and only forces the processes that are still there after
// grace. Where there is no way to ask (Windows), it ends them straight away.
// An error means something was still running at the end.
func Stop(names []string, grace time.Duration) (int, error) {
	procs, err := Find(names)
	if err != nil || len(procs) == 0 {
		return 0, err
	}
	count := len(procs)

	if canAsk {
		for _, p := range procs {
			_ = terminate(p.PID, false) // one already gone is what was wanted
		}
		deadline := time.Now().Add(grace)
		for time.Now().Before(deadline) {
			if left, err := Find(names); err == nil && len(left) == 0 {
				return count, nil
			}
			time.Sleep(stopPoll)
		}
		if procs, err = Find(names); err != nil || len(procs) == 0 {
			return count, err
		}
	}

	var firstErr error
	for _, p := range procs {
		if err := terminate(p.PID, true); err != nil && firstErr == nil && exists(p.PID, names) {
			firstErr = err
		}
	}
	// A forced end is not instant either: wait for the OS to finish it.
	for i := 0; i < 40; i++ {
		if left, err := Find(names); err == nil && len(left) == 0 {
			return count, nil
		}
		time.Sleep(stopPoll)
	}
	if firstErr != nil {
		return count, firstErr
	}
	return count, errStillRunning
}

// exists reports whether pid is (still) one of the processes going by names.
func exists(pid int, names []string) bool {
	procs, err := Find(names)
	if err != nil {
		return true
	}
	for _, p := range procs {
		if p.PID == pid {
			return true
		}
	}
	return false
}
