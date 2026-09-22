package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/credentials"
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

// loversLabFileName is the saved sign-in's file name in the app's config folder.
const loversLabFileName = "loverslab.jsonc"

// loversLabService names this credential set for internal/credentials - see
// credentials.Manager.Service.
const loversLabService = "loverslab"

// loversLabState is the optional LoversLab sign-in for the Browsing Extensions page's
// first source (see internal/credentials): a username/email and password, saved
// encrypted the same way the Steam Web API key already is (steamapi_settings.go).
// Browsing itself is mockup content for now - nothing here is ever sent anywhere; this
// only saves what a real sign-in will need once that part is built.
type loversLabState struct {
	mu  sync.Mutex
	mgr *credentials.Manager
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
}

func (a *App) loversLabStatusLocked() LoversLabStatus {
	out := LoversLabStatus{
		Protection: "Stored encrypted, bound to this computer. Your password is never shown again, never " +
			"written to the activity log, and is never sent anywhere - browsing LoversLab isn't built yet.",
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

// SaveLoversLabCredentials saves a username/email and password, encrypted and bound to
// this computer (see internal/credentials). Nothing is checked against LoversLab
// itself: browsing isn't built yet, so there is nothing real to verify a sign-in
// against - unlike SaveSteamAPIKey, which does check the real Steam API before saving.
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

	if err := a.loverslab.mgr.Save(map[string]string{"username": username, "password": password}); err != nil {
		return a.loversLabStatusLocked(), fmt.Errorf("could not save: %w", err)
	}
	applog.For("LoversLab").Infof("saved sign-in (encrypted)")
	return a.loversLabStatusLocked(), nil
}

// ClearLoversLabCredentials deletes the saved sign-in.
func (a *App) ClearLoversLabCredentials() (LoversLabStatus, error) {
	a.loverslab.mu.Lock()
	defer a.loverslab.mu.Unlock()
	if a.loverslab.mgr == nil {
		return a.loversLabStatusLocked(), nil
	}
	if err := a.loverslab.mgr.Clear(); err != nil {
		return a.loversLabStatusLocked(), fmt.Errorf("could not clear the saved sign-in: %w", err)
	}
	applog.For("LoversLab").Infof("saved sign-in removed")
	return a.loversLabStatusLocked(), nil
}
