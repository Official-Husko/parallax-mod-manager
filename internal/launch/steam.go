package launch

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/pkg/browser"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
)

// SteamAppURL builds the steam://run/<appid> launch URL for cfg, with
// extraArgs (if any) space-joined and appended after another "/", per
// docs/game-launching.md's literal wording.
//
// Flagged, not silently assumed: the doc describes this separator in
// prose, not as a byte-for-byte confirmed format - some real-world
// descriptions of Steam's launch-options convention use a double slash
// (steam://run/<appid>//<args>) instead. Verify against a real Steam
// client before relying on extraArgs in practice.
func SteamAppURL(cfg game.GameConfig, extraArgs []string) string {
	url := "steam://run/" + cfg.SteamAppID
	if len(extraArgs) > 0 {
		url += "/" + strings.Join(extraArgs, " ")
	}
	return url
}

// Launcher actually invokes something to open a URL or run a process - the
// one seam that must never execute for real inside this package's own test
// suite (every test here uses FakeLauncher instead).
type Launcher interface {
	// OpenURL asks the OS's default handler to open rawURL - how a
	// steam://run/<appid> URL reaches Steam.
	OpenURL(rawURL string) error
	// RunExecutable starts (does not wait for) a direct executable - the
	// non-Steam fallback path.
	RunExecutable(info game.ExecutableInfo) error
}

// OSLauncher is the production Launcher. OpenURL delegates to
// github.com/pkg/browser (xdg-open on Linux, `open` on macOS, the Windows
// shell handler - already a transitive Wails dependency, so this adds no
// new module). RunExecutable starts the process and returns immediately
// (Start, not Run/Wait), matching "let Steam/the OS own the process."
type OSLauncher struct{}

var _ Launcher = OSLauncher{}

func (OSLauncher) OpenURL(rawURL string) error {
	return browser.OpenURL(rawURL)
}

func (OSLauncher) RunExecutable(info game.ExecutableInfo) error {
	if info.Path == "" {
		return fmt.Errorf("launch: no executable path to run")
	}
	cmd := exec.Command(info.Path, info.Args...)
	return cmd.Start()
}

// FakeLauncher is a Launcher test double: records calls, never opens a URL
// or spawns a process.
type FakeLauncher struct {
	OpenedURLs       []string
	RanExecutables   []game.ExecutableInfo
	OpenURLErr       error
	RunExecutableErr error
}

var _ Launcher = (*FakeLauncher)(nil)

func (f *FakeLauncher) OpenURL(rawURL string) error {
	f.OpenedURLs = append(f.OpenedURLs, rawURL)
	return f.OpenURLErr
}

func (f *FakeLauncher) RunExecutable(info game.ExecutableInfo) error {
	f.RanExecutables = append(f.RanExecutables, info)
	return f.RunExecutableErr
}
