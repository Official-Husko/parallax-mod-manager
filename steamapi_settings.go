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
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamconfig"
)

// steamKeyPurpose binds the sealed key to this use (see secretbox.Box.Seal).
const steamKeyPurpose = "steam-web-api-key"

// steamVerifyTimeout bounds the one request that checks a key with Steam.
const steamVerifyTimeout = 20 * time.Second

// steamAPIState is the optional Steam Web API key: the service that decides which
// API answers, the file the key is kept in (encrypted), and what is known about it.
// The key itself lives only inside svc, in memory, and never crosses to the
// frontend or into the log.
type steamAPIState struct {
	// mu serialises the settings calls (save, mode change, check), which read and
	// write the file and the service together.
	mu       sync.Mutex
	svc      *steamapi.Service
	store    steamconfig.Store
	dir      string
	settings steamconfig.Settings
	// unreadable is true when a key is saved but cannot be decrypted here (the file
	// came from another computer, or was altered): the free API is used and the user
	// is asked to enter the key again.
	unreadable bool
	// verify checks a key with Steam; steamapi.VerifyKey, replaced in tests.
	verify func(ctx context.Context, key string) error
}

// SteamAPIStatus is what the Steam API settings panel shows. It never carries the
// key, only a fingerprint of it.
type SteamAPIStatus struct {
	// Mode is the chosen mode: "free", "complete" or "backup".
	Mode string
	// HasKey is true when a key is saved (even one that cannot be read here).
	HasKey bool
	// Fingerprint is the first 8 hex characters of a hash of the saved key.
	Fingerprint string
	// State is what the app is doing right now: "free" (no key in use), "active"
	// (the key is in use), "exhausted" (Steam says the key is out of requests, the
	// free API is used until ExhaustedUntil), "rejected" (Steam refused the key) or
	// "unreadable" (a key is saved that cannot be decrypted on this computer).
	State string
	// ExhaustedUntil is a Unix time in seconds, 0 unless State is "exhausted".
	ExhaustedUntil int64
	// LastError is the latest problem talking to Steam with the key, in words.
	LastError string
	// ItemsFromKey, ItemsFromFree and Rescued count Workshop items this run: answered
	// by the key, by the free API, and returned by the key after the free API could not.
	ItemsFromKey  int
	ItemsFromFree int
	Rescued       int
	// Protection says how the saved key is kept safe, for the panel to show.
	Protection string
}

// initSteamAPI loads the saved Steam API settings from dir, decrypts the key into
// memory, and puts the resulting service in front of the Workshop details fetch.
// dir is empty when the config folder could not be found: then only the free API is
// used and nothing can be saved.
func (a *App) initSteamAPI(dir string) {
	st := &a.steam
	log := applog.For("Steam")
	st.svc = steamapi.NewService(log)
	st.dir = dir
	if st.verify == nil {
		st.verify = steamapi.VerifyKey
	}
	if dir != "" {
		st.store = steamconfig.Store{Path: filepath.Join(dir, steamconfig.FileName)}
	}

	settings, err := st.store.Load()
	if err != nil {
		log.Warnf("the Steam API settings could not be read, so the free API is used: %v", err)
	}
	st.settings = settings

	switch {
	case !settings.Mode.UsesKey():
		log.Infof("Steam API: free API only")
	default:
		key, err := a.openSteamKey(settings.SealedKey)
		if err != nil {
			st.unreadable = true
			log.Warnf("the saved Steam API key (%s) cannot be decrypted on this computer, so the free API is used - enter the key again in Settings > Steam API", settings.Fingerprint)
			break
		}
		st.svc.Configure(settings.Mode, key)
		log.Infof("Steam API: %s use, key %s", settings.Mode, settings.Fingerprint)
	}

	a.workshopDetails.SetFetch(st.svc.Fetch)
}

// steamBox is the encryption bound to this computer.
func (a *App) steamBox() (*secretbox.Box, error) {
	return secretbox.ForThisMachine(a.steam.dir)
}

// openSteamKey decrypts a saved key.
func (a *App) openSteamKey(sealed string) (string, error) {
	box, err := a.steamBox()
	if err != nil {
		return "", err
	}
	return box.Open(sealed, steamKeyPurpose)
}

// steamStatusLocked builds the status from what is known. Callers hold steam.mu (or
// are before the service is shared).
func (a *App) steamStatusLocked() SteamAPIStatus {
	st := &a.steam
	out := SteamAPIStatus{
		Mode:        string(st.settings.Mode),
		HasKey:      st.settings.SealedKey != "",
		Fingerprint: st.settings.Fingerprint,
		Protection:  "Stored encrypted, bound to this computer. The key is never shown again, never written to the activity log, and is only sent to Steam.",
	}
	if out.Mode == "" {
		out.Mode = string(steamapi.ModeFree)
	}
	if st.svc == nil {
		out.State = "free"
		return out
	}
	s := st.svc.Status()
	out.LastError = s.LastError
	out.ItemsFromKey, out.ItemsFromFree, out.Rescued = s.Stats.ItemsFromKey, s.Stats.ItemsFromFree, s.Stats.Rescued
	switch {
	case !st.settings.Mode.UsesKey():
		out.State = "free"
	case st.unreadable:
		out.State = "unreadable"
	case s.Rejected:
		out.State = "rejected"
	case !s.ExhaustedUntil.IsZero():
		out.State = "exhausted"
		out.ExhaustedUntil = s.ExhaustedUntil.Unix()
	default:
		out.State = "active"
	}
	return out
}

// SteamAPIStatus reports the optional Steam Web API key's settings and state, for
// Settings > Steam API.
func (a *App) SteamAPIStatus() SteamAPIStatus {
	a.steam.mu.Lock()
	defer a.steam.mu.Unlock()
	return a.steamStatusLocked()
}

// changedSteamAPI is what follows any change to how Steam is asked: the details
// remembered from the old way are dropped (an unlisted item the free API returned
// "not found" for is found once the key is in use) and the frontend is told to ask
// again.
func (a *App) changedSteamAPI() {
	a.workshopDetails.Forget()
	a.emit("steam-api-changed")
}

// validSteamKey says whether s is shaped like a Steam Web API key (32 letters and
// digits, in practice). Steam's own answer is the real test; this only catches a
// paste that took more or less than the key (a "key=" prefix, a whole URL, spaces).
func validSteamKey(s string) bool {
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// SaveSteamAPIKey checks key with Steam and, only if Steam accepts it, saves it
// (encrypted, bound to this computer) and switches to mode, "complete" or "backup".
// A key that is rejected, or that could not be checked because Steam was unreachable,
// is not saved. Errors never contain the key.
func (a *App) SaveSteamAPIKey(key, mode string) (SteamAPIStatus, error) {
	st := &a.steam
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.svc == nil {
		return a.steamStatusLocked(), errors.New("the Steam API settings are not ready yet")
	}

	m := steamapi.ParseMode(mode)
	if !m.UsesKey() {
		return a.steamStatusLocked(), errors.New("choose Complete or Backup Steam API use to use a key (Free API use keeps no key)")
	}
	key = strings.TrimSpace(key)
	if !validSteamKey(key) {
		return a.steamStatusLocked(), errors.New("that does not look like a Steam Web API key: it is 32 letters and numbers, with nothing else around it")
	}
	if st.dir == "" {
		return a.steamStatusLocked(), errors.New("the settings folder could not be found, so a key cannot be saved")
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), steamVerifyTimeout)
	defer cancel()
	if err := st.verify(ctx, key); err != nil {
		if errors.Is(err, steamapi.ErrKeyRejected) {
			applog.For("Steam").Warnf("Steam rejected the API key that was entered, so it was not saved")
			return a.steamStatusLocked(), errors.New("Steam rejected that key. Check it at steamcommunity.com/dev/apikey and try again")
		}
		var limited *steamapi.RateLimitedError
		if errors.As(err, &limited) {
			return a.steamStatusLocked(), errors.New("Steam says this key is out of requests for now, so it could not be checked. Try again later")
		}
		applog.For("Steam").Warnf("the entered Steam API key could not be checked, so it was not saved: %v", err)
		return a.steamStatusLocked(), fmt.Errorf("could not reach Steam to check the key, so it was not saved: %w", err)
	}

	box, err := a.steamBox()
	if err != nil {
		return a.steamStatusLocked(), fmt.Errorf("could not set up encryption for the key: %w", err)
	}
	sealed, err := box.Seal(key, steamKeyPurpose)
	if err != nil {
		return a.steamStatusLocked(), fmt.Errorf("could not encrypt the key: %w", err)
	}
	settings := steamconfig.Settings{Mode: m, SealedKey: sealed, Fingerprint: secretbox.Fingerprint(key)}
	if err := st.store.Save(settings); err != nil {
		return a.steamStatusLocked(), fmt.Errorf("could not save the key: %w", err)
	}

	st.settings = settings
	st.unreadable = false
	st.svc.Configure(m, key)
	applog.For("Steam").Infof("Steam API key %s saved (encrypted), %s use", settings.Fingerprint, m)
	a.changedSteamAPI()
	return a.steamStatusLocked(), nil
}

// SetSteamAPIMode changes how the saved key is used. "complete" and "backup" need a
// saved key; "free" never uses one and deletes it from the settings file.
func (a *App) SetSteamAPIMode(mode string) (SteamAPIStatus, error) {
	st := &a.steam
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.svc == nil {
		return a.steamStatusLocked(), errors.New("the Steam API settings are not ready yet")
	}
	m := steamapi.ParseMode(mode)

	if !m.UsesKey() {
		hadKey := st.settings.SealedKey != ""
		if st.store.Path != "" {
			if err := st.store.Save(steamconfig.Defaults()); err != nil {
				return a.steamStatusLocked(), fmt.Errorf("could not remove the saved key: %w", err)
			}
		}
		st.settings = steamconfig.Defaults()
		st.unreadable = false
		st.svc.Configure(steamapi.ModeFree, "")
		if hadKey {
			applog.For("Steam").Infof("Steam API set to free API only; the saved key was deleted")
		} else {
			applog.For("Steam").Infof("Steam API set to free API only")
		}
		a.changedSteamAPI()
		return a.steamStatusLocked(), nil
	}

	key := st.svc.Key()
	if key == "" || st.unreadable {
		return a.steamStatusLocked(), errors.New("enter a Steam API key first")
	}
	settings := st.settings
	settings.Mode = m
	if st.store.Path != "" {
		if err := st.store.Save(settings); err != nil {
			return a.steamStatusLocked(), fmt.Errorf("could not save the setting: %w", err)
		}
	}
	st.settings = settings
	st.svc.Configure(m, key)
	applog.For("Steam").Infof("Steam API set to %s use (key %s)", m, settings.Fingerprint)
	a.changedSteamAPI()
	return a.steamStatusLocked(), nil
}

// CheckSteamAPIKey asks Steam whether the saved key still works. A key that works is
// used again if it had been set aside as rejected or out of requests.
func (a *App) CheckSteamAPIKey() (SteamAPIStatus, error) {
	st := &a.steam
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.svc == nil {
		return a.steamStatusLocked(), errors.New("the Steam API settings are not ready yet")
	}
	key := st.svc.Key()
	if key == "" || st.unreadable {
		return a.steamStatusLocked(), errors.New("no Steam API key is saved")
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), steamVerifyTimeout)
	defer cancel()
	err := st.verify(ctx, key)
	switch {
	case err == nil:
		s := st.svc.Status()
		if s.Rejected || !s.ExhaustedUntil.IsZero() {
			st.svc.Configure(st.settings.Mode, key)
			applog.For("Steam").Infof("the Steam API key %s works again, so it is in use", st.settings.Fingerprint)
			a.changedSteamAPI()
		}
		return a.steamStatusLocked(), nil
	case errors.Is(err, steamapi.ErrKeyRejected):
		st.svc.MarkRejected()
		applog.For("Steam").Warnf("Steam rejected the saved API key %s", st.settings.Fingerprint)
		return a.steamStatusLocked(), errors.New("Steam rejected the saved key. Enter a new one, or choose Free API use only")
	default:
		var limited *steamapi.RateLimitedError
		if errors.As(err, &limited) {
			return a.steamStatusLocked(), errors.New("Steam says the key is out of requests for now, which means it is a valid key")
		}
		return a.steamStatusLocked(), fmt.Errorf("could not reach Steam to check the key: %w", err)
	}
}
