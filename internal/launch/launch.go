package launch

import "github.com/Official-Husko/parallax-mod-manager/internal/game"

// LaunchMode is a user's chosen launch strategy for one game, persisted per
// game in preferences.Preferences.LaunchModes (kept as a plain string there,
// like this project's other per-game preference maps - these constants are
// its only valid values) and read by App.LaunchGame to build LaunchOptions.
// See docs/game-launching.md's "Skipping the Paradox Launcher" section.
type LaunchMode string

const (
	// LaunchModeSteam is the default - steam://run/<appid>, going through
	// Steam and the Paradox Launcher exactly as it would without this app
	// involved at all. An empty/unset preference means this, not just an
	// explicit "steam" value, so an existing preferences file from before
	// this setting existed keeps behaving exactly as it already did.
	LaunchModeSteam LaunchMode = "steam"
	// LaunchModeDirect skips the Paradox Launcher (and Steam) entirely,
	// resolving and starting the game's own executable directly via
	// game.GameConfig.ResolveExecutable - the first of the two bypass
	// strategies this project intends to offer. The second - replacing a
	// game's own launcher entry point with a small shim so Steam still
	// launches the game itself while never invoking the Paradox Launcher -
	// is a planned addition, not implemented yet; LaunchMode exists as a
	// string enum rather than LaunchOptions.Direct's plain bool specifically
	// so that mode can be added later without another breaking change here.
	LaunchModeDirect LaunchMode = "direct"
)

// LaunchOptions configures a Launch call.
type LaunchOptions struct {
	// ExtraArgs is appended to the Steam URL, or passed to the direct-exe
	// fallback's arguments.
	ExtraArgs []string
	// Direct forces the direct-exe fallback even when cfg.SteamAppID is
	// set - for GOG/manual installs.
	Direct bool
	// InstallDir is required for the direct-exe path: passed to
	// cfg.ResolveExecutable to find the real executable.
	InstallDir string
}

// Launch starts cfg via l: the Steam protocol URL when cfg.SteamAppID is
// set and !opts.Direct (the primary path per docs/game-launching.md - this
// lets Steam/Proton handle the actual process launch), otherwise
// cfg.ResolveExecutable(opts.InstallDir) + RunExecutable (the fallback for
// non-Steam installs).
func Launch(l Launcher, cfg game.GameConfig, opts LaunchOptions) error {
	if cfg.SteamAppID != "" && !opts.Direct {
		return l.OpenURL(SteamAppURL(cfg, opts.ExtraArgs))
	}

	info, err := cfg.ResolveExecutable(opts.InstallDir)
	if err != nil {
		return err
	}
	if len(opts.ExtraArgs) > 0 {
		info.Args = append(append([]string{}, info.Args...), opts.ExtraArgs...)
	}
	return l.RunExecutable(info)
}
