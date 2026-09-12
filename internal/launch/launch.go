package launch

import "github.com/Official-Husko/parallax-mod-manager/internal/game"

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
