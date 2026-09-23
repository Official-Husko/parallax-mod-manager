// Package launchershim installs, detects, repairs, and removes the launcher shim
// (companions/launcher-shim - a separate Go module, built as its own standalone
// executable, never imported here) in place of a game's own dowser/dowser.exe, and
// reads what it reports. See docs/game-launching.md's "Skipping the Paradox
// Launcher" section for the full picture, and companions/launcher-shim/main.go for
// the shim's own side of everything documented here.
package launchershim

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// PingPort must match companions/launcher-shim/main.go's own pingPort constant
// exactly - two separate Go modules, so it can't be a shared Go constant; kept
// identical by comment cross-reference in both places instead.
const PingPort = 47813

// statusFileName must match companions/launcher-shim/main.go's own
// statusFileRelPath's file name.
const statusFileName = "launcher_shim_status.json"

// Status mirrors the shim's own status-file JSON shape (its own status type) - this
// package only ever reads this file, the shim only ever writes it.
type Status struct {
	Time        time.Time `json:"time"`
	ResolvedExe string    `json:"resolvedExe"`
	Args        []string  `json:"args,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// ReadStatus reads the shim's own most recent run, if any, from configAppDir - the
// same "<user config dir>/parallax-mod-manager" folder app.go's own configAppDir
// already is, which is exactly where the shim itself writes this file (it resolves
// the same folder independently, via os.UserConfigDir, since it can't share this
// app's own configAppDir value across the module boundary). A missing file just
// means the shim has never run yet on this machine - not an error.
func ReadStatus(configAppDir string) (Status, bool, error) {
	data, err := os.ReadFile(filepath.Join(configAppDir, statusFileName))
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, err
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return Status{}, false, err
	}
	return s, true, nil
}
