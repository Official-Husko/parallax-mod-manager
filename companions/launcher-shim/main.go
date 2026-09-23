// Command launcher-shim replaces a Paradox game's own dowser/dowser.exe launcher
// entry point. Steam is configured to launch dowser, not the game itself - dowser's
// only real job (confirmed by disassembling a real, unstripped copy of it) is to make
// sure the Paradox Launcher is installed and up to date, then start it; it has no
// "skip the launcher" mode of its own. Once this shim takes dowser's place, Steam
// still thinks it launched the game directly (its own environment, overlay hookup,
// and process context all carry through untouched) but the Paradox Launcher itself
// never opens - see the parent project's docs/game-launching.md.
//
// Standard library only, deliberately - see info.txt and README.md for why.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// repoURL is where this program's real source is published - see --source below and
// info.txt. Never a copy of the source itself, only a link to it.
const repoURL = "https://github.com/Official-Husko/parallax-mod-manager"

// pingPort is the fixed loopback port Parallax Mod Manager listens on for a live,
// best-effort "the shim just ran" notice - internal/launchershim (the main module)
// documents this exact same number; two separate Go modules can't share a constant,
// so it's kept identical by comment cross-reference instead. A var, not a const, so
// tests can point it at an ephemeral port instead of risking a real collision.
var pingPort = 47813

// statusFileRelPath is where the status file lives under the user's config directory
// (os.UserConfigDir) - the same "parallax-mod-manager/" folder app.go's own
// configAppDir already uses on the main app's side, so Parallax can read this back
// later even if it wasn't running when the shim ran.
var statusFileRelPath = filepath.Join("parallax-mod-manager", "launcher_shim_status.json")

// launchFunc actually starts the resolved game executable; launch in launch_unix.go/
// launch_windows.go in production, replaced in tests with one that just records what
// it was asked to do instead of really starting a process.
var launchFunc = launch

func main() {
	// Deliberately not the standard library's flag package: Steam passes this
	// program whatever launch options the user configured, verbatim, and those are
	// meant for the real game, not for this shim - flag.Parse would refuse to run
	// at all the moment one of them looks like an unrecognized flag (confirmed: a
	// real, if synthetic, Steam-style launch option made an early flag.Parse-based
	// version of this refuse to start). --source is the one and only thing this
	// program itself ever looks at; everything else is pure pass-through.
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-source" || args[0] == "--source") {
		printSource(os.Stdout)
		return
	}

	selfPath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "launcher-shim: finding my own path:", err)
		os.Exit(1)
	}
	if err := runIn(filepath.Dir(selfPath), args); err != nil {
		fmt.Fprintln(os.Stderr, "launcher-shim:", err)
		os.Exit(1)
	}
}

// printSource answers --source: a one-line explanation and a link, never a copy of
// the source itself.
func printSource(w io.Writer) {
	fmt.Fprintln(w, "This is Parallax Mod Manager's launcher shim - it replaces a Paradox game's own")
	fmt.Fprintln(w, "dowser/dowser.exe so Steam launches the game directly, skipping the Paradox")
	fmt.Fprintln(w, "Launcher. It never carries a copy of its own source code; the real thing is")
	fmt.Fprintln(w, "published here:")
	fmt.Fprintln(w, repoURL)
}

// launcherSettings is the tiny subset of a real launcher-settings.json this shim
// actually needs - see game.GameConfig.ResolveExecutable in the main module for the
// full picture. Reimplemented here, not imported: this is a deliberately separate Go
// module with no dependency on anything but the standard library.
type launcherSettings struct {
	ExePath string   `json:"exePath"`
	ExeArgs []string `json:"exeArgs"`
}

// status is what gets written after every run (see writeStatus) - the main module's
// own internal/launchershim package reads this back later.
type status struct {
	Time        time.Time `json:"time"`
	ResolvedExe string    `json:"resolvedExe"`
	Args        []string  `json:"args,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// runIn does the real work, given dir (the shim's own directory - always passed
// explicitly rather than read from os.Executable directly, so tests can point it at
// a fixture directory) and steamArgs (whatever Steam itself passed to the shim).
func runIn(dir string, steamArgs []string) error {
	exe, args, err := resolveLaunch(dir)
	if err != nil {
		writeStatus(status{Time: time.Now(), Success: false, Error: err.Error()})
		return err
	}
	finalArgs := append(append([]string{}, args...), steamArgs...)

	writeStatus(status{Time: time.Now(), ResolvedExe: exe, Args: finalArgs, Success: true})
	pingParallax(exe)

	if err := launchFunc(exe, finalArgs); err != nil {
		writeStatus(status{Time: time.Now(), ResolvedExe: exe, Args: finalArgs, Success: false, Error: err.Error()})
		return fmt.Errorf("starting %s: %w", exe, err)
	}
	return nil
}

// resolveLaunch reads dir/launcher-settings.json and resolves the real game
// executable from its exePath - the same way game.GameConfig.ResolveExecutable does
// in the main module (exePath resolved relative to launcher-settings.json's own
// directory, which is always dir here since a real dowser and launcher-settings.json
// are always colocated - confirmed against this project's own real Stellaris and
// Hearts of Iron IV installs).
func resolveLaunch(dir string) (exe string, args []string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, "launcher-settings.json"))
	if err != nil {
		return "", nil, fmt.Errorf("reading launcher-settings.json in %s: %w", dir, err)
	}
	var ls launcherSettings
	if err := json.Unmarshal(data, &ls); err != nil {
		return "", nil, fmt.Errorf("parsing launcher-settings.json in %s: %w", dir, err)
	}
	if ls.ExePath == "" {
		return "", nil, fmt.Errorf("launcher-settings.json in %s has no exePath", dir)
	}

	exe = filepath.Clean(filepath.Join(dir, ls.ExePath))
	if _, err := os.Stat(exe); err != nil {
		return "", nil, fmt.Errorf("resolved executable does not exist: %s: %w", exe, err)
	}
	return exe, ls.ExeArgs, nil
}

// writeStatus records the outcome of a run, atomically (temp file then rename, the
// same reason internal/atomicfile does this in the main module - not imported here,
// reimplemented in a few lines, since this is a separate module). Best-effort only:
// any failure here (no config directory, a permissions problem) is silently ignored
// rather than turning into a reason the actual game launch fails.
func writeStatus(s status) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	path := filepath.Join(configDir, statusFileRelPath)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}

	destDir := filepath.Dir(path)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(destDir, ".tmp-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

// pingParallax is the live, best-effort half of "communicate with Parallax" (the
// status file above is the authoritative half - this one only matters if Parallax
// happens to already be running). A closed or refused connection here is completely
// normal - it just means Parallax isn't running, or nothing's listening - and is
// silently ignored; this never delays the actual launch beyond the dial/write
// deadlines below.
func pingParallax(resolvedExe string) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", pingPort), 200*time.Millisecond)
	if err != nil {
		return
	}
	defer conn.Close()

	payload, err := json.Marshal(struct {
		ResolvedExe string `json:"resolvedExe"`
		PID         int    `json:"pid"`
	}{ResolvedExe: resolvedExe, PID: os.Getpid()})
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, _ = conn.Write(payload)
}
