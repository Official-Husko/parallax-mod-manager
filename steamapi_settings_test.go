package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamconfig"
)

const testSteamKey = "SENTINELKEY0123456789ABCDEF012345"

type steamEvents struct {
	mu    sync.Mutex
	names []string
}

func (e *steamEvents) sink(name string, _ ...any) {
	e.mu.Lock()
	e.names = append(e.names, name)
	e.mu.Unlock()
}

func (e *steamEvents) count(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, x := range e.names {
		if x == name {
			n++
		}
	}
	return n
}

// newSteamApp is an App whose Steam API settings live in dir and whose key check
// is scripted: keys in accepted pass, anything else is rejected, unless verifyErr is
// set.
func newSteamApp(t *testing.T, dir string, accepted ...string) (*App, *steamEvents) {
	t.Helper()
	events := &steamEvents{}
	a := &App{ctx: context.Background(), eventSink: events.sink}
	a.steam.verify = func(ctx context.Context, key string) error {
		for _, k := range accepted {
			if k == key {
				return nil
			}
		}
		return steamapi.ErrKeyRejected
	}
	a.initSteamAPI(dir)
	return a, events
}

func readSteamFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, steamconfig.FileName))
	if err != nil {
		t.Fatalf("reading the settings file: %v", err)
	}
	return string(data)
}

func TestSteamAPIStartsFreeWithNoKey(t *testing.T) {
	a, _ := newSteamApp(t, t.TempDir())
	st := a.SteamAPIStatus()
	if st.Mode != "free" || st.State != "free" || st.HasKey {
		t.Errorf("status = %+v, want free with no key", st)
	}
}

func TestSteamAPISaveStoresTheKeyEncryptedAndItSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	a, events := newSteamApp(t, dir, testSteamKey)

	st, err := a.SaveSteamAPIKey("  "+testSteamKey+"\n", "complete")
	if err != nil {
		t.Fatalf("SaveSteamAPIKey: %v", err)
	}
	if st.Mode != "complete" || st.State != "active" || !st.HasKey || len(st.Fingerprint) != 8 {
		t.Errorf("status = %+v", st)
	}
	if events.count("steam-api-changed") != 1 {
		t.Errorf("steam-api-changed emitted %d times, want 1", events.count("steam-api-changed"))
	}

	file := readSteamFile(t, dir)
	if strings.Contains(file, testSteamKey) {
		t.Fatalf("the settings file contains the key in the clear:\n%s", file)
	}
	if !strings.Contains(file, `"mode": "complete"`) || !strings.Contains(file, `"key": "v1.`) {
		t.Errorf("the settings file is not what was expected:\n%s", file)
	}

	// A new run of the app reads the file and can use the key again.
	b, _ := newSteamApp(t, dir, testSteamKey)
	if got := b.steam.svc.Key(); got != testSteamKey {
		t.Errorf("after a restart the key = %q, want it decrypted back", got)
	}
	if st := b.SteamAPIStatus(); st.Mode != "complete" || st.State != "active" || st.Fingerprint != a.SteamAPIStatus().Fingerprint {
		t.Errorf("status after a restart = %+v", st)
	}
}

func TestSteamAPIAKeySteamRejectsIsNotSaved(t *testing.T) {
	dir := t.TempDir()
	a, events := newSteamApp(t, dir /* accepts nothing */)

	_, err := a.SaveSteamAPIKey(testSteamKey, "complete")
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err = %v, want a rejection message", err)
	}
	if strings.Contains(err.Error(), testSteamKey) {
		t.Error("the error contains the key")
	}
	if _, statErr := os.Stat(filepath.Join(dir, steamconfig.FileName)); statErr == nil {
		t.Error("a settings file was written for a rejected key")
	}
	if a.steam.svc.Key() != "" || a.SteamAPIStatus().HasKey {
		t.Error("a rejected key was kept in memory")
	}
	if events.count("steam-api-changed") != 0 {
		t.Error("an unchanged state emitted steam-api-changed")
	}
}

func TestSteamAPIAKeyThatCouldNotBeCheckedIsNotSaved(t *testing.T) {
	dir := t.TempDir()
	a, _ := newSteamApp(t, dir)
	a.steam.verify = func(context.Context, string) error {
		return errors.New("steamapi: requesting keyed details: dial tcp: no such host")
	}

	_, err := a.SaveSteamAPIKey(testSteamKey, "backup")
	if err == nil || !strings.Contains(err.Error(), "could not reach Steam") || strings.Contains(err.Error(), testSteamKey) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, steamconfig.FileName)); statErr == nil {
		t.Error("a settings file was written for a key that could not be checked")
	}
}

func TestSteamAPIRefusesBadInputBeforeAskingSteam(t *testing.T) {
	a, _ := newSteamApp(t, t.TempDir(), testSteamKey)
	asked := false
	a.steam.verify = func(context.Context, string) error { asked = true; return nil }

	for name, tc := range map[string]struct{ key, mode string }{
		"free mode":         {testSteamKey, "free"},
		"unknown mode":      {testSteamKey, "turbo"},
		"empty key":         {"", "complete"},
		"a pasted url":      {"https://api.steampowered.com/?key=" + testSteamKey, "complete"},
		"key= prefix":       {"key=" + testSteamKey, "complete"},
		"too short":         {"abc123", "complete"},
		"a space inside it": {"ABCDEF012345 6789ABCDEF0123456789AB", "complete"},
	} {
		_, err := a.SaveSteamAPIKey(tc.key, tc.mode)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if tc.key != "" && strings.Contains(err.Error(), strings.TrimSpace(tc.key)) {
			t.Errorf("%s: the error echoes the input: %v", name, err)
		}
	}
	if asked {
		t.Error("Steam was asked to check input that was refused up front")
	}
}

func TestSteamAPIFreeModeDeletesTheSavedKey(t *testing.T) {
	dir := t.TempDir()
	a, events := newSteamApp(t, dir, testSteamKey)
	if _, err := a.SaveSteamAPIKey(testSteamKey, "complete"); err != nil {
		t.Fatal(err)
	}
	fingerprint := a.SteamAPIStatus().Fingerprint

	st, err := a.SetSteamAPIMode("free")
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "free" || st.State != "free" || st.HasKey || st.Fingerprint != "" {
		t.Errorf("status = %+v, want free with the key gone", st)
	}
	file := readSteamFile(t, dir)
	if strings.Contains(file, "v1.") || strings.Contains(file, fingerprint) {
		t.Errorf("the key is still in the settings file:\n%s", file)
	}
	if a.steam.svc.Key() != "" {
		t.Error("the key is still in memory")
	}
	if events.count("steam-api-changed") != 2 {
		t.Errorf("steam-api-changed emitted %d times, want 2 (save, then free)", events.count("steam-api-changed"))
	}

	// And nothing brings it back on the next run.
	b, _ := newSteamApp(t, dir, testSteamKey)
	if st := b.SteamAPIStatus(); st.HasKey || st.Mode != "free" {
		t.Errorf("after a restart status = %+v, want free with no key", st)
	}
}

func TestSteamAPIKeyModesNeedAKey(t *testing.T) {
	a, _ := newSteamApp(t, t.TempDir())
	for _, mode := range []string{"complete", "backup"} {
		if _, err := a.SetSteamAPIMode(mode); err == nil {
			t.Errorf("mode %q without a key succeeded", mode)
		}
	}
	if st := a.SteamAPIStatus(); st.Mode != "free" {
		t.Errorf("mode = %q, want it to stay free", st.Mode)
	}
}

func TestSteamAPISwitchingBetweenCompleteAndBackupKeepsTheKey(t *testing.T) {
	dir := t.TempDir()
	a, _ := newSteamApp(t, dir, testSteamKey)
	if _, err := a.SaveSteamAPIKey(testSteamKey, "complete"); err != nil {
		t.Fatal(err)
	}
	st, err := a.SetSteamAPIMode("backup")
	if err != nil || st.Mode != "backup" || !st.HasKey || st.State != "active" {
		t.Fatalf("status = %+v, err = %v", st, err)
	}
	b, _ := newSteamApp(t, dir, testSteamKey)
	if st := b.SteamAPIStatus(); st.Mode != "backup" || b.steam.svc.Key() != testSteamKey {
		t.Errorf("after a restart: status = %+v", st)
	}
}

func TestSteamAPIAKeyThatCannotBeDecryptedHereIsSetAsideNotLost(t *testing.T) {
	dir := t.TempDir()
	// A file "saved on another computer": a sealed value this machine cannot open.
	store := steamconfig.Store{Path: filepath.Join(dir, steamconfig.FileName)}
	if err := store.Save(steamconfig.Settings{Mode: steamapi.ModeComplete, SealedKey: "v1.AAAAAAAAAAAAAAAA.AAAAAAAAAAAAAAAAAAAAAAAA", Fingerprint: "deadbeef"}); err != nil {
		t.Fatal(err)
	}

	a, _ := newSteamApp(t, dir, testSteamKey)
	st := a.SteamAPIStatus()
	if st.State != "unreadable" || !st.HasKey || st.Fingerprint != "deadbeef" {
		t.Errorf("status = %+v, want unreadable with the fingerprint still shown", st)
	}
	if a.steam.svc.Status().KeyInUse {
		t.Error("an undecryptable key is in use")
	}
	if _, err := a.SetSteamAPIMode("backup"); err == nil {
		t.Error("switching modes with an unreadable key succeeded")
	}

	// Entering the key again repairs it.
	st, err := a.SaveSteamAPIKey(testSteamKey, "complete")
	if err != nil || st.State != "active" {
		t.Errorf("after entering the key again: status = %+v, err = %v", st, err)
	}
}

func TestSteamAPICheckSetsAsideAndRestoresARejectedKey(t *testing.T) {
	a, _ := newSteamApp(t, t.TempDir(), testSteamKey)
	if _, err := a.SaveSteamAPIKey(testSteamKey, "complete"); err != nil {
		t.Fatal(err)
	}

	// Steam stops accepting it (revoked).
	a.steam.verify = func(context.Context, string) error { return steamapi.ErrKeyRejected }
	st, err := a.CheckSteamAPIKey()
	if err == nil || st.State != "rejected" {
		t.Fatalf("status = %+v, err = %v, want rejected", st, err)
	}

	// It works again (or was a hiccup): checking puts it back in use.
	a.steam.verify = func(context.Context, string) error { return nil }
	st, err = a.CheckSteamAPIKey()
	if err != nil || st.State != "active" {
		t.Errorf("status = %+v, err = %v, want active again", st, err)
	}
}

func TestSteamAPICheckWithNoKeyErrors(t *testing.T) {
	a, _ := newSteamApp(t, t.TempDir())
	if _, err := a.CheckSteamAPIKey(); err == nil {
		t.Error("CheckSteamAPIKey with no key succeeded")
	}
}

func TestSteamAPINoSettingsFolderStaysFree(t *testing.T) {
	a, _ := newSteamApp(t, "", testSteamKey)
	if st := a.SteamAPIStatus(); st.State != "free" {
		t.Errorf("status = %+v", st)
	}
	if _, err := a.SaveSteamAPIKey(testSteamKey, "complete"); err == nil {
		t.Error("saving a key with nowhere to keep it succeeded")
	}
}

func TestSteamAPINeverLeaksTheKey(t *testing.T) {
	dir := t.TempDir()
	a, _ := newSteamApp(t, dir, testSteamKey)
	var all []string
	if st, err := a.SaveSteamAPIKey(testSteamKey, "backup"); err == nil {
		all = append(all, fmt.Sprintf("%+v", st))
	} else {
		t.Fatal(err)
	}
	st, _ := a.SetSteamAPIMode("complete")
	all = append(all, fmt.Sprintf("%+v", st))
	st, _ = a.CheckSteamAPIKey()
	all = append(all, fmt.Sprintf("%+v", st))
	all = append(all, readSteamFile(t, dir))
	for _, e := range applog.Default().Entries() {
		all = append(all, e.Line())
	}
	for _, s := range all {
		if strings.Contains(s, testSteamKey) {
			t.Fatalf("the key appears in: %.200s", s)
		}
	}
}
