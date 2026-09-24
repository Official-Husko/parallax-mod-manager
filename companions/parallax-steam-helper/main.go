package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/companions/parallax-steam-helper/steamworks"
)

// openClient is steamworks.Open by default, wrapped to return the steamClient
// interface rather than steamworks.Open's own concrete *steamworks.Client -
// replaced in tests with a fake that never dlopen's anything real (the same
// "var wraps the real thing" seam companions/launcher-shim's own launchFunc
// uses).
var openClient = func(libPath string) (steamClient, error) {
	return steamworks.Open(libPath)
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "parallax-steam-helper:", err)
		os.Exit(1)
	}
}

// run reads one Request from in, drives publish, and writes every Event to
// out as newline-delimited JSON, flushed after each line so a caller
// streaming this process's stdout sees progress live rather than only at
// exit.
func run(in io.Reader, out io.Writer) error {
	var req Request
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return fmt.Errorf("reading request from stdin: %w", err)
	}

	w := bufio.NewWriter(out)
	emit := func(e Event) {
		data, err := json.Marshal(e)
		if err != nil {
			return // can't happen for this fixed shape - nothing useful to do if it somehow did
		}
		_, _ = w.Write(data)
		_, _ = w.WriteString("\n")
		_ = w.Flush()
	}

	restoreWD, err := prepareWorkDir(req)
	if err != nil {
		emit(Event{Stage: "error", Message: err.Error()})
		return err
	}
	defer restoreWD()

	client, err := openClient(req.LibraryPath)
	if err != nil {
		emit(Event{Stage: "error", Message: err.Error()})
		return err
	}

	if err := publish(client, req, emit); err != nil {
		emit(Event{Stage: "error", Message: err.Error()})
		return err
	}
	return nil
}

// prepareWorkDir creates req.WorkDir if needed, writes its steam_appid.txt
// (Valve's own documented development-time mechanism for telling
// SteamAPI_Init which AppID context to use without being launched through
// Steam), and switches into it - SteamAPI_Init reads that file from the
// process's current working directory, not from any path passed to it.
//
// restore, if non-nil, changes the process back to its original working
// directory - this helper's own production lifetime ends moments after
// (the whole process exits right after run returns), but restoring it
// matters for tests, which call run repeatedly in the same process and
// would otherwise leak this chdir across test cases.
func prepareWorkDir(req Request) (restore func(), err error) {
	if err := os.MkdirAll(req.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating work directory %s: %w", req.WorkDir, err)
	}
	appIDPath := filepath.Join(req.WorkDir, "steam_appid.txt")
	if err := os.WriteFile(appIDPath, fmt.Appendf(nil, "%d", req.AppID), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", appIDPath, err)
	}

	original, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("reading the current working directory: %w", err)
	}
	if err := os.Chdir(req.WorkDir); err != nil {
		return nil, fmt.Errorf("switching to work directory %s: %w", req.WorkDir, err)
	}
	return func() { _ = os.Chdir(original) }, nil
}
