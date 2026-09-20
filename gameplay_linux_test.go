package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/gamelog"
	"github.com/Official-Husko/parallax-mod-manager/internal/gameproc"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// testGameApp is an App with one registered game, "Test Game", whose executable
// is called exeName and whose install folder and user data are temp folders.
func testGameApp(t *testing.T, exeName string) *App {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := game.GameConfig{
		ID:                 "test-game",
		DisplayName:        "Test Game",
		FolderName:         "TestGame",
		ExecutableFallback: game.ExecutableInfo{Path: "/somewhere/" + exeName},
	}
	a := &App{registry: game.NewRegistry([]game.GameConfig{cfg})}
	a.preferences = preferences.Defaults()
	// An explicit install folder, so nothing goes looking through real Steam libraries.
	a.preferences.GamePaths = map[string]string{cfg.ID: t.TempDir()}
	return a
}

// startFakeGame runs a real process going by name (a renamed copy of the shell,
// waiting on its input), and collects it when it ends.
func startFakeGame(t *testing.T) (name string, cmd *exec.Cmd) {
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
	name = fmt.Sprintf("pxg-%08x", time.Now().UnixNano()&0xffffffff)
	path := filepath.Join(t.TempDir(), name)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	dst.Close()

	cmd = exec.Command(path, "-c", "read x")
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("a renamed copy of the shell won't start here: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	return name, cmd
}

func launchLines() []string {
	var out []string
	for _, e := range applog.Default().Entries() {
		if e.Component == "Launch" {
			out = append(out, e.Message)
		}
	}
	return out
}

func countContaining(lines []string, sub string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

func TestGameStatusAndStopGameFollowARealProcess(t *testing.T) {
	// The app looks the game up by its executable name, so the game is registered
	// under the name the process was started as.
	procName, cmd := startFakeGame(t)
	a := testGameApp(t, procName)
	applog.Default().Clear()

	st, err := a.GameStatus("test-game")
	if err != nil || !st.Running || len(st.PIDs) != 1 || st.PIDs[0] != cmd.Process.Pid {
		t.Fatalf("GameStatus = %+v, %v, want running as pid %d", st, err, cmd.Process.Pid)
	}
	// Asking again is not news.
	for i := 0; i < 3; i++ {
		if _, err := a.GameStatus("test-game"); err != nil {
			t.Fatal(err)
		}
	}
	if got := countContaining(launchLines(), "is running"); got != 1 {
		t.Errorf("logged the game running %d times over 4 looks, want once: %q", got, launchLines())
	}

	n, err := a.StopGame("test-game")
	if err != nil || n != 1 {
		t.Fatalf("StopGame = %d, %v, want 1 process stopped", n, err)
	}
	if st, _ := a.GameStatus("test-game"); st.Running {
		t.Fatalf("still running after StopGame: %+v", st)
	}
	lines := launchLines()
	if countContaining(lines, "stopping 'Test Game'") != 1 || countContaining(lines, "'Test Game' stopped") != 1 {
		t.Errorf("expected one 'stopping' and one 'stopped' line, got %q", lines)
	}
	if countContaining(lines, "has closed") != 0 {
		t.Errorf("a stop the app did itself was announced again as the game closing: %q", lines)
	}

	// Nothing left to stop: not an error.
	if n, err := a.StopGame("test-game"); n != 0 || err != nil {
		t.Errorf("StopGame on a game that isn't running = %d, %v, want 0 and no error", n, err)
	}
}

func TestGameStatusLogsAGameThatCloseOnItsOwn(t *testing.T) {
	procName, cmd := startFakeGame(t)
	a := testGameApp(t, procName)
	applog.Default().Clear()

	if st, _ := a.GameStatus("test-game"); !st.Running {
		t.Fatal("expected running")
	}
	_ = cmd.Process.Kill() // the player quit the game
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := a.GameStatus("test-game"); !st.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := countContaining(launchLines(), "has closed"); got != 1 {
		t.Errorf("logged the game closing %d times, want once: %q", got, launchLines())
	}
}

func TestGameStatusForAGameThatIsNotRunningIsQuiet(t *testing.T) {
	a := testGameApp(t, "no-such-game-process-77aa")
	applog.Default().Clear()
	for i := 0; i < 3; i++ {
		st, err := a.GameStatus("test-game")
		if err != nil || st.Running || st.PIDs == nil {
			t.Fatalf("GameStatus = %+v, %v, want not running with a non-nil list", st, err)
		}
	}
	if lines := launchLines(); len(lines) != 0 {
		t.Errorf("a game that was never running logged %q", lines)
	}
}

func TestGameStatusAndStopGameRejectAnUnknownGame(t *testing.T) {
	a := testGameApp(t, "x")
	if _, err := a.GameStatus("nope"); err == nil {
		t.Error("GameStatus for an unknown game should fail")
	}
	if _, err := a.StopGame("nope"); err == nil {
		t.Error("StopGame for an unknown game should fail")
	}
}

// collectEvents captures what the app sends the frontend.
type collectEvents struct {
	mu      sync.Mutex
	batches []struct {
		Session int
		Batch   gamelog.Batch
	}
}

func (c *collectEvents) sink(name string, data ...any) {
	if name != gameLogEvent || len(data) != 2 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batches = append(c.batches, struct {
		Session int
		Batch   gamelog.Batch
	}{data[0].(int), data[1].(gamelog.Batch)})
}

func (c *collectEvents) waitFor(t *testing.T, pred func(session int, b gamelog.Batch) bool, msg string) {
	t.Helper()
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		c.mu.Lock()
		for _, e := range c.batches {
			if pred(e.Session, e.Batch) {
				c.mu.Unlock()
				return
			}
		}
		c.mu.Unlock()
	}
	t.Fatal(msg)
}

func (c *collectEvents) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.batches)
}

func writeGameLog(t *testing.T, a *App, name, content string, appendTo bool) {
	t.Helper()
	cfg, _ := a.registry.Get("test-game")
	dir, err := gameLogsDir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendTo {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(filepath.Join(dir, name), flags, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func hasLine(b gamelog.Batch, line string) bool {
	for _, l := range b.Lines {
		if l == line {
			return true
		}
	}
	return false
}

func TestGameLogFilesListsWhatTheGameWrote(t *testing.T) {
	a := testGameApp(t, "x")

	// Never run: no logs folder yet.
	listing, err := a.GameLogFiles("test-game")
	if err != nil || listing.Files == nil || len(listing.Files) != 0 || !strings.HasSuffix(listing.Dir, filepath.Join("TestGame", "logs")) {
		t.Fatalf("before any run: %+v, %v, want the folder and an empty non-nil list", listing, err)
	}

	writeGameLog(t, a, "error.log", "boom\n", false)
	writeGameLog(t, a, "game.log", "hi\n", false)
	listing, err = a.GameLogFiles("test-game")
	if err != nil || len(listing.Files) != 2 || listing.Files[0].Name != "error.log" {
		t.Fatalf("after a run: %+v, %v, want error.log then game.log", listing, err)
	}
	if _, err := a.GameLogFiles("nope"); err == nil {
		t.Error("an unknown game should fail")
	}
}

func TestWatchGameLogStreamsTheFileAndSwitchesCleanly(t *testing.T) {
	a := testGameApp(t, "x")
	events := &collectEvents{}
	a.eventSink = events.sink
	t.Cleanup(a.StopWatchingGameLog)
	writeGameLog(t, a, "error.log", "[10:00:00][a.cpp:1]: first\n[10:00:01][a.cpp:2]: second\n", false)
	writeGameLog(t, a, "game.log", "game log line\n", false)

	s1, err := a.WatchGameLog("test-game", "error.log")
	if err != nil {
		t.Fatal(err)
	}
	events.waitFor(t, func(s int, b gamelog.Batch) bool {
		return s == s1 && b.Reset && hasLine(b, "[10:00:01][a.cpp:2]: second")
	}, "the end of the log never arrived")

	writeGameLog(t, a, "error.log", "[10:00:02][a.cpp:3]: third\n", true)
	events.waitFor(t, func(s int, b gamelog.Batch) bool {
		return s == s1 && !b.Reset && hasLine(b, "[10:00:02][a.cpp:3]: third")
	}, "the appended line never arrived")

	// Switching to another file: a new session, and its content.
	s2, err := a.WatchGameLog("test-game", "game.log")
	if err != nil {
		t.Fatal(err)
	}
	if s2 <= s1 {
		t.Errorf("session numbers must only grow: %d then %d", s1, s2)
	}
	events.waitFor(t, func(s int, b gamelog.Batch) bool {
		return s == s2 && b.Reset && hasLine(b, "game log line")
	}, "the second file never arrived")

	// The first file is no longer followed: nothing more for its session.
	time.Sleep(700 * time.Millisecond) // past one poll of any straggler
	before := events.count()
	writeGameLog(t, a, "error.log", "after the switch\n", true)
	time.Sleep(900 * time.Millisecond)
	if events.count() != before {
		t.Errorf("the file that was switched away from still produced events")
	}

	a.StopWatchingGameLog()
	time.Sleep(700 * time.Millisecond)
	before = events.count()
	writeGameLog(t, a, "game.log", "after stop\n", true)
	time.Sleep(900 * time.Millisecond)
	if events.count() != before {
		t.Errorf("events kept arriving after StopWatchingGameLog")
	}
}

func TestWatchGameLogRefusesAnythingButALogFileInTheLogsFolder(t *testing.T) {
	a := testGameApp(t, "x")
	a.eventSink = (&collectEvents{}).sink
	for _, bad := range []string{"", "../error.log", "sub/error.log", "/etc/passwd", "error.txt", ".hidden.log", `..\error.log`} {
		if _, err := a.WatchGameLog("test-game", bad); err == nil {
			t.Errorf("WatchGameLog(%q) was accepted", bad)
		}
	}
	if a.gameLog != nil {
		t.Error("a refused name must not start following anything")
	}
	if _, err := a.WatchGameLog("nope", "error.log"); err == nil {
		t.Error("an unknown game should fail")
	}
}

func TestStopEndsBothALauncherScriptAndTheGameItStarted(t *testing.T) {
	// Hearts of Iron IV's launcher settings name a shell script, which runs the real
	// game as its child. Stopping only the script would leave the game running.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to copy")
	}
	dir := t.TempDir()
	program := fmt.Sprintf("pxw-%08x", time.Now().UnixNano()&0xffffffff)
	src, err := os.ReadFile(sh)
	if err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(dir, program), src, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run_"+program)
	body := "#!/bin/sh\nGAME_DIR=`dirname \"$0\"`\n\"$GAME_DIR/" + program + "\" -c 'read x'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	a := testGameApp(t, "unused")
	cfg, _ := a.registry.Get("test-game")
	cfg.ExecutableFallback.Path = script
	a.registry = game.NewRegistry([]game.GameConfig{cfg})

	cmd := exec.Command(script)
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("can't start the script here: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	names := gameproc.ExecutableNamesFor(script)
	t.Cleanup(func() { _, _ = gameproc.Stop(names, 0); _ = cmd.Process.Kill(); <-done })

	// Both the script and the game it started are running.
	deadline := time.Now().Add(3 * time.Second)
	var st gameproc.Status
	for time.Now().Before(deadline) {
		st, _ = a.GameStatus("test-game")
		if len(st.PIDs) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !st.Running || len(st.PIDs) != 2 {
		t.Fatalf("GameStatus = %+v, want the script and the game it started (2 processes)", st)
	}

	n, err := a.StopGame("test-game")
	if err != nil || n != 2 {
		t.Fatalf("StopGame = %d, %v, want both processes stopped", n, err)
	}
	if st, _ := a.GameStatus("test-game"); st.Running {
		t.Errorf("the game is still running after Stop: %+v", st)
	}
}
