package gameproc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func selfPID() int { return os.Getpid() }

// startNamed starts a real process that goes by a unique name, so nothing else
// on the machine running the tests can match it. It is a copy of the shell
// (renamed, not a symlink: a process's name is the file it was started from)
// blocked reading its standard input; the returned function kills it.
// script runs before the read - e.g. to make it ignore SIGTERM.
func startNamed(t *testing.T, script string) (name string, pid int, release func()) {
	t.Helper()
	name, cmd := startNamedProcess(t, script)
	// Collect it when it exits, as a parent would - an uncollected exited process
	// is a zombie, which is exactly what Find has to ignore.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	release = func() {
		_ = cmd.Process.Kill()
		<-done
	}
	t.Cleanup(release)
	return name, cmd.Process.Pid, release
}

// startNamedProcess is startNamed without anything collecting the process when it
// exits: the caller owns cmd and must Wait for it.
func startNamedProcess(t *testing.T, script string) (name string, cmd *exec.Cmd) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to copy")
	}
	src, err := os.Open(sh)
	if err != nil {
		t.Skip(err)
	}
	defer src.Close()

	name = fmt.Sprintf("pxt-%08x", time.Now().UnixNano()&0xffffffff)
	path := filepath.Join(t.TempDir(), name)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	dst.Close()

	cmd = exec.Command(path, "-c", script+"read x")
	// Blocks the shell reading; nothing is ever written, and the pipe stays open
	// for as long as cmd does.
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("a renamed copy of the shell won't start here: %v", err)
	}
	waitUntil(t, func() bool {
		procs, _ := Find([]string{name})
		return len(procs) == 1
	}, "the test process never showed up in the process list")
	return name, cmd
}

func waitUntil(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal(msg)
}

func TestFindAndCheckSeeARealProcessByItsName(t *testing.T) {
	name, pid, release := startNamed(t, "")

	st, err := Check([]string{name})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || len(st.PIDs) != 1 || st.PIDs[0] != pid {
		t.Errorf("Check = %+v, want running as pid %d", st, pid)
	}
	// The same name as a Windows executable is not the same game.
	if st, _ := Check([]string{name + "-other"}); st.Running {
		t.Errorf("a different name matched: %+v", st)
	}

	release()
	waitUntil(t, func() bool { st, _ := Check([]string{name}); return !st.Running }, "a process that exited still counts as running")
}

func TestStopEndsAProcessThatHonoursTheRequest(t *testing.T) {
	name, _, _ := startNamed(t, "")

	start := time.Now()
	n, err := Stop([]string{name}, 5*time.Second)
	if err != nil || n != 1 {
		t.Fatalf("Stop = %d, %v, want 1 stopped and no error", n, err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %v: a process that honours SIGTERM should be gone at once, not after the grace period", took)
	}
	if st, _ := Check([]string{name}); st.Running {
		t.Error("still running after Stop")
	}
}

func TestStopForcesAProcessThatIgnoresTheRequest(t *testing.T) {
	name, _, _ := startNamed(t, `trap "" TERM; `)

	start := time.Now()
	n, err := Stop([]string{name}, 300*time.Millisecond)
	if err != nil || n != 1 {
		t.Fatalf("Stop = %d, %v, want 1 stopped and no error", n, err)
	}
	if took := time.Since(start); took < 250*time.Millisecond {
		t.Errorf("took %v: it should have waited out the grace period before forcing", took)
	}
	if st, _ := Check([]string{name}); st.Running {
		t.Error("a process that ignores SIGTERM survived Stop")
	}
}

func TestStopWithNothingRunningIsAQuietNoOp(t *testing.T) {
	n, err := Stop([]string{"a-name-no-process-has-9f3c1e"}, time.Second)
	if n != 0 || err != nil {
		t.Errorf("Stop = %d, %v, want 0 and no error", n, err)
	}
}

func TestStopLeavesOtherProcessesAlone(t *testing.T) {
	name, _, _ := startNamed(t, "")
	other, _, _ := startNamed(t, "")

	if n, err := Stop([]string{name}, 5*time.Second); err != nil || n != 1 {
		t.Fatalf("Stop = %d, %v", n, err)
	}
	if st, _ := Check([]string{other}); !st.Running {
		t.Error("Stop ended a process it wasn't asked to")
	}
}

func TestAProcessThatHasExitedButWasNotCollectedIsNotRunning(t *testing.T) {
	// A game started directly and never waited on becomes a zombie when it
	// closes: gone, but still listed under its old name until its parent collects
	// it. That must read as "not running", or Stop would never appear to work.
	name, cmd := startNamedProcess(t, "")
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// Deliberately not Wait()ing yet: wait for the kernel to mark it dead by
	// checking its state directly.
	waitUntil(t, func() bool {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid))
		return err == nil && zombieState(raw)
	}, "the killed process never became a zombie")

	if st, _ := Check([]string{name}); st.Running {
		t.Errorf("a zombie was reported as running: %+v", st)
	}
	_ = cmd.Wait()
}

// zombieState reports whether a /proc/<pid>/stat line says the process is a
// zombie: its state is the field after the process name, which is in brackets
// and may itself contain spaces and brackets.
func zombieState(stat []byte) bool {
	s := string(stat)
	i := strings.LastIndex(s, ")")
	return i >= 0 && i+2 < len(s) && s[i+2] == 'Z'
}
