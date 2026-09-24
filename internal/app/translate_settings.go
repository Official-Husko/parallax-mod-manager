package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/credentials"
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate/deepl"
)

// deeplFileName is the saved key's file name in the app's config folder.
const deeplFileName = "deepl.jsonc"

// deeplService names this credential set for internal/credentials - see
// credentials.Manager.Service.
const deeplService = "deepl"

// deeplState is the optional DeepL API key for the auto-translation
// feature's Settings > Tools panel, saved encrypted the same way the
// Steam Web API key and LoversLab sign-in already are - see
// steamapi_settings.go and loverslab_settings.go, the two precedents this
// mirrors.
type deeplState struct {
	mu  sync.Mutex
	mgr *credentials.Manager
	// verify checks a key against the real DeepL endpoint with one real
	// translate call; deeplVerify in production, replaced in tests with
	// one that never makes a real request - the same swappable-verify-
	// function pattern steamAPIState.verify already uses for the same
	// reason.
	verify func(ctx context.Context, apiKey string, pro bool) error
}

// deeplVerify is deeplState.verify's real, production implementation: a
// single real translate call is the only way to actually confirm a key
// works, the same "checked with the real service before it is saved" rule
// every other credential save in this app already follows.
func deeplVerify(ctx context.Context, apiKey string, pro bool) error {
	client := deepl.New(apiKey, pro)
	_, err := client.Translate(ctx, "Hello", translate.Language{Code: "DE"})
	return err
}

// DeepLStatus is what the Settings > Tools panel shows for DeepL. It
// never carries the key.
type DeepLStatus struct {
	// HasKey is true once a key is saved and readable.
	HasKey bool
	// Fingerprint is a short, one-way identifier of the saved key (never
	// decryptable back to it), for display.
	Fingerprint string
	// Tier is "free" or "pro" - which of DeepL's two real base URLs the
	// key is used against. An explicit choice made when the key is saved,
	// not auto-detected from the key itself.
	Tier string
	// Unreadable is true when a key is saved but cannot be decrypted here
	// (the file came from another computer, or was altered) - the person
	// is asked to enter it again.
	Unreadable bool
	// Protection says how the saved key is kept safe, for the panel to show.
	Protection string
}

// initDeepL sets up the encrypted key store. dir is empty when the config
// folder could not be found: then nothing can be saved, matching
// initSteamAPI/initLoversLab's own handling of the same situation.
func (a *App) initDeepL(dir string) {
	var path string
	if dir != "" {
		path = filepath.Join(dir, deeplFileName)
	}
	box, err := secretbox.ForThisMachine(dir)
	if err != nil {
		applog.For("Translate").Warnf("could not set up encryption for a saved DeepL key: %v", err)
	}
	a.deepl.mgr = &credentials.Manager{Service: deeplService, Path: path, Box: box}
	if a.deepl.verify == nil {
		a.deepl.verify = deeplVerify
	}
}

func (a *App) deeplStatusLocked() DeepLStatus {
	out := DeepLStatus{
		Tier: "free",
		Protection: "Stored encrypted, bound to this computer. Never shown again once saved, and never " +
			"written to the activity log.",
	}
	mgr := a.deepl.mgr
	if mgr == nil {
		return out
	}
	hasKey, _ := mgr.Has("apiKey")
	if tier, err := mgr.Open("tier"); err == nil && tier != "" {
		out.Tier = tier
	}
	key, err := mgr.Open("apiKey")
	if err != nil {
		// Nothing saved yet is not the same as saved-but-unreadable: only
		// the latter is worth telling the person about.
		out.Unreadable = hasKey
		return out
	}
	out.HasKey = hasKey
	out.Fingerprint = secretbox.Fingerprint(key)
	return out
}

// DeepLStatus reports the optional saved DeepL API key, for the Settings >
// Tools panel.
func (a *App) DeepLStatus() DeepLStatus {
	a.deepl.mu.Lock()
	defer a.deepl.mu.Unlock()
	return a.deeplStatusLocked()
}

// SaveDeepLAPIKey checks key against DeepL itself (one real translate
// call, at whichever tier is chosen) and, only if that succeeds, saves it
// encrypted and bound to this computer - the same "checked with the real
// service before it is saved" rule SaveSteamAPIKey/SaveLoversLabCredentials
// already follow, so a wrong key (or the wrong tier for a real key) is
// never stored.
func (a *App) SaveDeepLAPIKey(key, tier string) (DeepLStatus, error) {
	a.deepl.mu.Lock()
	defer a.deepl.mu.Unlock()

	key = strings.TrimSpace(key)
	if key == "" {
		return a.deeplStatusLocked(), errors.New("enter a DeepL API key")
	}
	if tier != "pro" {
		tier = "free"
	}
	if a.deepl.mgr == nil || a.deepl.mgr.Path == "" {
		return a.deeplStatusLocked(), errors.New("the settings folder could not be found, so this cannot be saved")
	}

	if err := a.deepl.verify(a.baseContext(), key, tier == "pro"); err != nil {
		applog.For("Translate").Warnf("DeepL key rejected, so it was not saved: %v", err)
		return a.deeplStatusLocked(), errors.New("DeepL rejected that key (wrong key, or the wrong tier chosen for it)")
	}

	if err := a.deepl.mgr.Save(map[string]string{"apiKey": key, "tier": tier}); err != nil {
		return a.deeplStatusLocked(), fmt.Errorf("verified, but could not save: %w", err)
	}
	applog.For("Translate").Infof("DeepL API key saved (encrypted), %s tier", tier)
	return a.deeplStatusLocked(), nil
}

// deeplCredentials reads the saved key and tier under the same lock every
// other deeplState access uses, briefly - rather than TranslateMod (a
// potentially long-running call) holding a.deepl.mu for its own entire
// duration, which would otherwise block the Settings > Tools panel from
// even checking the saved key's status while a translation is in
// progress.
func (a *App) deeplCredentials() (apiKey, tier string, err error) {
	a.deepl.mu.Lock()
	defer a.deepl.mu.Unlock()
	if a.deepl.mgr == nil {
		return "", "", errors.New("no DeepL API key is saved - add one in Settings > Tools first")
	}
	apiKey, err = a.deepl.mgr.Open("apiKey")
	if err != nil {
		return "", "", errors.New("no DeepL API key is saved - add one in Settings > Tools first")
	}
	if tier, err = a.deepl.mgr.Open("tier"); err != nil {
		tier = "free"
	}
	return apiKey, tier, nil
}

// ClearDeepLAPIKey deletes the saved key.
func (a *App) ClearDeepLAPIKey() (DeepLStatus, error) {
	a.deepl.mu.Lock()
	defer a.deepl.mu.Unlock()
	if a.deepl.mgr == nil {
		return a.deeplStatusLocked(), nil
	}
	if err := a.deepl.mgr.Clear(); err != nil {
		return a.deeplStatusLocked(), fmt.Errorf("could not clear the saved key: %w", err)
	}
	applog.For("Translate").Infof("DeepL API key removed")
	return a.deeplStatusLocked(), nil
}
