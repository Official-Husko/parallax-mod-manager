package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/gamelog"
	"github.com/Official-Husko/parallax-mod-manager/internal/gameproc"
)

// The bound methods for a game that is running: whether it is, stopping it, and
// following its own log files. Launching lives in app.go (LaunchGame); this is
// what comes after.

const (
	// gameLogEvent carries an update from the followed game log to the frontend:
	// (session int, batch gamelog.Batch). See WatchGameLog.
	gameLogEvent = "game-log"

	// stopGrace is how long a game is given to close after being asked to before
	// it is forced. Long enough to write its files; short enough that the person
	// who pressed Stop isn't left wondering.
	stopGrace = 5 * time.Second

	// gameExeTTL is how long a game's executable name is remembered. Working it out
	// reads the Steam library files, and the frontend asks whether the game is
	// running every few seconds.
	gameExeTTL = time.Minute
)

// cachedGameExe is a game's executable names, and what they were worked out from.
type cachedGameExe struct {
	names    []string
	override string
	at       time.Time
}

// gameLogFollow is the one game log being followed, if any.
type gameLogFollow struct {
	cancel  context.CancelFunc
	session int
}

// baseContext is the app's context, or a plain one before startup has run (and
// in tests).
func (a *App) baseContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// emit sends a frontend event. eventSink replaces the real thing in tests.
func (a *App) emit(name string, data ...any) {
	if a.eventSink != nil {
		a.eventSink(name, data...)
		return
	}
	wailsruntime.EventsEmit(a.baseContext(), name, data...)
}

// gameExecutableNames returns the names cfg's game runs as, for finding it among
// the running processes - the executable its own launcher-settings.json names,
// or the built-in fallback when the install can't be found. Remembered briefly.
func (a *App) gameExecutableNames(cfg game.GameConfig) []string {
	override, _ := a.gamePathOverride(cfg)

	a.gameProcMu.Lock()
	if e, ok := a.gameExes[cfg.ID]; ok && e.override == override && time.Since(e.at) < gameExeTTL {
		a.gameProcMu.Unlock()
		return e.names
	}
	a.gameProcMu.Unlock()

	var names []string
	if dir, ok := a.resolveInstallDir(cfg); ok {
		if info, err := cfg.ResolveExecutable(dir); err == nil {
			names = gameproc.ExecutableNamesFor(info.Path)
		}
	}
	if names == nil {
		names = gameproc.ExecutableNamesFor(cfg.ExecutableFallback.Path)
	}

	a.gameProcMu.Lock()
	if a.gameExes == nil {
		a.gameExes = map[string]cachedGameExe{}
	}
	a.gameExes[cfg.ID] = cachedGameExe{names: names, override: override, at: time.Now()}
	a.gameProcMu.Unlock()
	return names
}

// GameStatus reports whether gameID's game is running right now and which
// processes are it - however it was started: from here, from Steam, from the
// Paradox Launcher. The frontend asks every few seconds to show Play or Stop.
//
// A game that can't be told apart from the other processes (no executable name
// is known for it) simply reads as not running.
func (a *App) GameStatus(gameID string) (gameproc.Status, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return gameproc.Status{PIDs: []int{}}, fmt.Errorf("app: unknown game %q", gameID)
	}
	st, err := gameproc.Check(a.gameExecutableNames(cfg))
	if err != nil {
		// Listing processes isn't possible here (an unsupported platform, say).
		// Say so once rather than on every poll, and read as not running.
		if _, seen := a.gameProcWarned.LoadOrStore(cfg.ID, true); !seen {
			applog.For("Launch").Warnf("can't tell whether '%s' is running: %v", cfg.DisplayName, err)
		}
		return gameproc.Status{PIDs: []int{}}, nil
	}
	a.noteGameRunning(cfg, st)
	return st, nil
}

// noteGameRunning logs a game starting or closing - when it changes, not on every
// look (see docs/logging.md: log news, not state).
func (a *App) noteGameRunning(cfg game.GameConfig, st gameproc.Status) {
	a.gameProcMu.Lock()
	was, known := a.gameRunning[cfg.ID]
	if a.gameRunning == nil {
		a.gameRunning = map[string]bool{}
	}
	a.gameRunning[cfg.ID] = st.Running
	a.gameProcMu.Unlock()

	switch {
	case st.Running && !was:
		applog.For("Launch").Infof("'%s' is running (%s)", cfg.DisplayName, describePIDs(st.PIDs))
	case !st.Running && was && known:
		applog.For("Launch").Infof("'%s' has closed", cfg.DisplayName)
	}
}

func describePIDs(pids []int) string {
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = strconv.Itoa(p)
	}
	if len(parts) == 1 {
		return "process " + parts[0]
	}
	return "processes " + strings.Join(parts, ", ")
}

// StopGame ends gameID's running game: it asks the game to close, and forces it
// if it hasn't after a few seconds. Returns how many processes it ended, which is
// 0 when the game wasn't running. It blocks until the game is gone, so the UI can
// show it as stopping meanwhile.
func (a *App) StopGame(gameID string) (int, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return 0, fmt.Errorf("app: unknown game %q", gameID)
	}
	names := a.gameExecutableNames(cfg)
	log := applog.For("Launch")
	timer := log.Begin()

	st, _ := gameproc.Check(names)
	if !st.Running {
		log.Infof("asked to stop '%s', but it isn't running", cfg.DisplayName)
		return 0, nil
	}
	log.Infof("stopping '%s' (%s)", cfg.DisplayName, describePIDs(st.PIDs))
	n, err := gameproc.Stop(names, stopGrace)
	if err != nil {
		timer.Errorf("couldn't stop '%s': %v", cfg.DisplayName, err)
		return n, fmt.Errorf("couldn't stop %s: %w", cfg.DisplayName, err)
	}
	// Already known to be closed - the next status check needn't announce it again.
	a.gameProcMu.Lock()
	if a.gameRunning == nil {
		a.gameRunning = map[string]bool{}
	}
	a.gameRunning[cfg.ID] = false
	a.gameProcMu.Unlock()
	timer.Infof("'%s' stopped", cfg.DisplayName)
	return n, nil
}

// gameLogsDir is where cfg's game writes its logs.
func gameLogsDir(cfg game.GameConfig) (string, error) {
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userDir, "logs"), nil
}

// GameLogFiles lists the log files gameID's game has written, most useful first,
// and the folder they are in.
func (a *App) GameLogFiles(gameID string) (gamelog.Listing, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return gamelog.Listing{Files: []gamelog.File{}}, fmt.Errorf("app: unknown game %q", gameID)
	}
	dir, err := gameLogsDir(cfg)
	if err != nil {
		return gamelog.Listing{Files: []gamelog.File{}}, err
	}
	files, err := gamelog.List(dir)
	if err != nil {
		applog.For("Files").Warnf("couldn't list the game logs of '%s' in '%s': %v", cfg.DisplayName, dir, err)
		return gamelog.Listing{Dir: dir, Files: []gamelog.File{}}, err
	}
	return gamelog.Listing{Dir: dir, Files: files}, nil
}

// WatchGameLog starts following one of gameID's game log files, replacing any
// other being followed: its last lines arrive at once and new ones as the game
// writes them, as "game-log" events carrying (session, batch). Returns the
// session number those events are tagged with, so the frontend can ignore
// updates still in flight from the file it was watching before.
func (a *App) WatchGameLog(gameID, file string) (int, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return 0, fmt.Errorf("app: unknown game %q", gameID)
	}
	if !gamelog.ValidName(file) {
		return 0, fmt.Errorf("app: %q isn't a game log file", file)
	}
	dir, err := gameLogsDir(cfg)
	if err != nil {
		return 0, err
	}

	a.gameLogMu.Lock()
	defer a.gameLogMu.Unlock()
	if a.gameLog != nil {
		a.gameLog.cancel()
	}
	a.gameLogLast++
	session := a.gameLogLast
	ctx, cancel := context.WithCancel(a.baseContext())
	a.gameLog = &gameLogFollow{cancel: cancel, session: session}

	path := filepath.Join(dir, file)
	go gamelog.Follow(ctx, path, gamelog.Options{}, func(b gamelog.Batch) {
		a.emit(gameLogEvent, session, b)
	})
	applog.For("Files").Debugf("following the game log '%s' of '%s'", file, cfg.DisplayName)
	return session, nil
}

// StopWatchingGameLog stops following the game log, if one is being followed.
func (a *App) StopWatchingGameLog() {
	a.gameLogMu.Lock()
	defer a.gameLogMu.Unlock()
	if a.gameLog != nil {
		a.gameLog.cancel()
		a.gameLog = nil
	}
}
