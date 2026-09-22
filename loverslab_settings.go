package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/credentials"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

// loversLabFileName is the saved sign-in's file name in the app's config folder.
const loversLabFileName = "loverslab.jsonc"

// loversLabService names this credential set for internal/credentials - see
// credentials.Manager.Service.
const loversLabService = "loverslab"

// loversLabState is the LoversLab sign-in for the Browse tab's first source: a
// username/email and password, saved encrypted the same way the Steam Web API key
// already is (steamapi_settings.go), plus the live, authenticated client that sign-in
// unlocks - see loverslab.go for the actual browsing (categories, file listings,
// changelogs) built on top of it.
type loversLabState struct {
	mu  sync.Mutex
	mgr *credentials.Manager
	// client is the current run's authenticated session, once one exists - lazily
	// created and signed in by ensureLoversLabSession (loverslab.go), not here, since
	// a fresh App start has no reason to sign in before anything actually asks to
	// browse.
	client *loverslab.Client
	// verifiedAt is when client was last confirmed still signed in server-side -
	// ensureLoversLabSession only re-checks after sessionRecheckInterval passes,
	// rather than on every single call.
	verifiedAt time.Time
	// login signs in and returns the authenticated client; loverslabLogin in
	// production, replaced in tests with one that never makes a real request against
	// the real site - the same swappable-verify-function pattern
	// steamAPIState.verify (steamapi_settings.go) already uses for the same reason.
	login func(ctx context.Context, auth, password string) (*loverslab.Client, error)
	// verify checks a client is still signed in server-side; loverslabVerify in
	// production (a thin wrapper around Client.VerifySession), replaced in tests with
	// one that never makes a real request - same reason as login above.
	verify func(ctx context.Context, client *loverslab.Client) (bool, error)
}

// loverslabLogin is loversLabState.login's real, production implementation.
func loverslabLogin(ctx context.Context, auth, password string) (*loverslab.Client, error) {
	client, err := loverslab.New()
	if err != nil {
		return nil, err
	}
	if err := client.Login(ctx, auth, password); err != nil {
		return nil, err
	}
	return client, nil
}

// loverslabVerify is loversLabState.verify's real, production implementation.
func loverslabVerify(ctx context.Context, client *loverslab.Client) (bool, error) {
	return client.VerifySession(ctx)
}

// LoversLabStatus is what the Browsing Extensions panel shows for LoversLab. It never
// carries the password.
type LoversLabStatus struct {
	// SignedIn is true once both a username and a password are saved and readable.
	SignedIn bool
	// Username is shown again (unlike the password) once saved, so the panel can say
	// who is signed in without asking again - a username is not a secret the way a
	// password is.
	Username string
	// Unreadable is true when credentials are saved but cannot be decrypted here (the
	// file came from another computer, or was altered) - the person is asked to sign
	// in again.
	Unreadable bool
	// Protection says how the saved sign-in is kept safe, for the panel to show.
	Protection string
}

// initLoversLab sets up the encrypted sign-in store. dir is empty when the config
// folder could not be found: then nothing can be saved, matching initSteamAPI's own
// handling of the same situation.
func (a *App) initLoversLab(dir string) {
	var path string
	if dir != "" {
		path = filepath.Join(dir, loversLabFileName)
	}
	box, err := secretbox.ForThisMachine(dir)
	if err != nil {
		applog.For("LoversLab").Warnf("could not set up encryption for a saved sign-in: %v", err)
	}
	a.loverslab.mgr = &credentials.Manager{Service: loversLabService, Path: path, Box: box}
	if a.loverslab.login == nil {
		a.loverslab.login = loverslabLogin
	}
	if a.loverslab.verify == nil {
		a.loverslab.verify = loverslabVerify
	}
}

func (a *App) loversLabStatusLocked() LoversLabStatus {
	out := LoversLabStatus{
		Protection: "Stored encrypted, bound to this computer. Your password is never shown again and never " +
			"written to the activity log.",
	}
	mgr := a.loverslab.mgr
	if mgr == nil {
		return out
	}
	hasPassword, _ := mgr.Has("password")
	username, err := mgr.Open("username")
	if err != nil {
		// Nothing saved yet is not the same as saved-but-unreadable: only the
		// latter is worth telling the person about.
		out.Unreadable = hasPassword
		return out
	}
	out.Username = username
	out.SignedIn = hasPassword
	return out
}

// LoversLabStatus reports the optional saved LoversLab sign-in, for the Browsing
// Extensions panel.
func (a *App) LoversLabStatus() LoversLabStatus {
	a.loverslab.mu.Lock()
	defer a.loverslab.mu.Unlock()
	return a.loversLabStatusLocked()
}

// SaveLoversLabCredentials checks a username/email and password against LoversLab
// itself and, only if the sign-in succeeds, saves them encrypted and bound to this
// computer (see internal/credentials) - the same "checked with the real service
// before it is saved" rule SaveSteamAPIKey already follows, so a wrong password is
// never stored. The session the successful sign-in produces is saved alongside them
// (also encrypted), so browsing doesn't need to sign in again every time the app
// restarts.
func (a *App) SaveLoversLabCredentials(username, password string) (LoversLabStatus, error) {
	a.loverslab.mu.Lock()
	defer a.loverslab.mu.Unlock()

	username = strings.TrimSpace(username)
	if username == "" {
		return a.loversLabStatusLocked(), errors.New("enter your LoversLab username or email")
	}
	if password == "" {
		return a.loversLabStatusLocked(), errors.New("enter your LoversLab password")
	}
	if a.loverslab.mgr == nil || a.loverslab.mgr.Path == "" {
		return a.loversLabStatusLocked(), errors.New("the settings folder could not be found, so this cannot be saved")
	}

	client, err := a.loverslab.login(a.baseContext(), username, password)
	if err != nil {
		applog.For("LoversLab").Warnf("sign-in rejected, so it was not saved: %v", err)
		return a.loversLabStatusLocked(), errors.New("LoversLab rejected that username/email and password")
	}

	fields := map[string]string{"username": username, "password": password}
	if session, err := client.ExportSession(); err == nil {
		fields["session"] = session
	} else {
		applog.For("LoversLab").Warnf("signed in, but could not export the session to save alongside it: %v", err)
	}
	if err := a.loverslab.mgr.Save(fields); err != nil {
		return a.loversLabStatusLocked(), fmt.Errorf("signed in, but could not save: %w", err)
	}

	a.loverslab.client = client
	a.loverslab.verifiedAt = time.Now()
	applog.For("LoversLab").Infof("signed in as %s (member %s), saved encrypted", username, client.MemberID())
	return a.loversLabStatusLocked(), nil
}

// ClearLoversLabCredentials signs out of the current session (best effort - a failed
// logout request never blocks clearing the saved sign-in) and deletes it.
func (a *App) ClearLoversLabCredentials() (LoversLabStatus, error) {
	a.loverslab.mu.Lock()
	defer a.loverslab.mu.Unlock()
	if a.loverslab.client != nil {
		if err := a.loverslab.client.Logout(a.baseContext()); err != nil {
			applog.For("LoversLab").Warnf("logging out of LoversLab failed (clearing the saved sign-in anyway): %v", err)
		}
		a.loverslab.client = nil
		a.loverslab.verifiedAt = time.Time{}
	}
	if a.loverslab.mgr == nil {
		return a.loversLabStatusLocked(), nil
	}
	if err := a.loverslab.mgr.Clear(); err != nil {
		return a.loversLabStatusLocked(), fmt.Errorf("could not clear the saved sign-in: %w", err)
	}
	applog.For("LoversLab").Infof("saved sign-in removed")
	return a.loversLabStatusLocked(), nil
}
