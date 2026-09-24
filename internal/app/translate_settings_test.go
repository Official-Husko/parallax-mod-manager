package app

import (
	"context"
	"errors"
	"testing"
)

const testDeepLKey = "sentinel-deepl-key"

// newDeeplApp is an App whose DeepL settings live in dir and whose key
// check is scripted: keys in accepted pass, anything else is rejected.
func newDeeplApp(t *testing.T, dir string, accepted ...string) *App {
	t.Helper()
	a := &App{ctx: context.Background(), eventSink: func(string, ...any) {}}
	a.deepl.verify = func(_ context.Context, apiKey string, _ bool) error {
		for _, k := range accepted {
			if k == apiKey {
				return nil
			}
		}
		return errors.New("DeepL rejected the key")
	}
	a.initDeepL(dir)
	return a
}

func TestDeepLStatusStartsWithNoKey(t *testing.T) {
	a := newDeeplApp(t, t.TempDir())
	st := a.DeepLStatus()
	if st.HasKey || st.Tier != "free" || st.Unreadable {
		t.Errorf("status = %+v, want no key, free tier, not unreadable", st)
	}
}

func TestSaveDeepLAPIKeyStoresItEncryptedAndItSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	a := newDeeplApp(t, dir, testDeepLKey)

	st, err := a.SaveDeepLAPIKey("  "+testDeepLKey+"\n", "pro")
	if err != nil {
		t.Fatalf("SaveDeepLAPIKey() error = %v", err)
	}
	if !st.HasKey || st.Tier != "pro" || len(st.Fingerprint) != 8 {
		t.Errorf("status = %+v, want HasKey=true Tier=pro an 8-char fingerprint", st)
	}

	// A fresh App reading the same dir (simulating a restart) sees the
	// same saved state.
	restarted := newDeeplApp(t, dir, testDeepLKey)
	st2 := restarted.DeepLStatus()
	if !st2.HasKey || st2.Tier != "pro" || st2.Fingerprint != st.Fingerprint {
		t.Errorf("status after restart = %+v, want it to match the saved state %+v", st2, st)
	}
}

func TestSaveDeepLAPIKeyRejectsAWrongKeyWithoutSavingIt(t *testing.T) {
	dir := t.TempDir()
	a := newDeeplApp(t, dir, testDeepLKey)

	if _, err := a.SaveDeepLAPIKey("totally-wrong-key", "free"); err == nil {
		t.Fatal("want an error for a key the scripted verify rejects")
	}
	if st := a.DeepLStatus(); st.HasKey {
		t.Error("a rejected key must never be saved")
	}
}

func TestSaveDeepLAPIKeyRejectsAnEmptyKey(t *testing.T) {
	a := newDeeplApp(t, t.TempDir())
	if _, err := a.SaveDeepLAPIKey("   ", "free"); err == nil {
		t.Fatal("want an error for a blank key")
	}
}

func TestSaveDeepLAPIKeyDefaultsAnUnrecognizedTierToFree(t *testing.T) {
	dir := t.TempDir()
	a := newDeeplApp(t, dir, testDeepLKey)
	st, err := a.SaveDeepLAPIKey(testDeepLKey, "not-a-real-tier")
	if err != nil {
		t.Fatalf("SaveDeepLAPIKey() error = %v", err)
	}
	if st.Tier != "free" {
		t.Errorf("Tier = %q, want it to default to \"free\"", st.Tier)
	}
}

func TestClearDeepLAPIKeyRemovesIt(t *testing.T) {
	dir := t.TempDir()
	a := newDeeplApp(t, dir, testDeepLKey)
	if _, err := a.SaveDeepLAPIKey(testDeepLKey, "free"); err != nil {
		t.Fatal(err)
	}
	st, err := a.ClearDeepLAPIKey()
	if err != nil {
		t.Fatalf("ClearDeepLAPIKey() error = %v", err)
	}
	if st.HasKey {
		t.Error("HasKey is still true after ClearDeepLAPIKey")
	}
}

func TestDeepLStatusNeverExposesTheRawKey(t *testing.T) {
	dir := t.TempDir()
	a := newDeeplApp(t, dir, testDeepLKey)
	if _, err := a.SaveDeepLAPIKey(testDeepLKey, "free"); err != nil {
		t.Fatal(err)
	}
	st := a.DeepLStatus()
	if st.Fingerprint == testDeepLKey {
		t.Error("Fingerprint must never be the raw key itself")
	}
}

func TestDeepLWithNoConfigDirDegradesGracefully(t *testing.T) {
	a := newDeeplApp(t, "") // no config dir found - matches initSteamAPI/initLoversLab's own tolerance
	st := a.DeepLStatus()
	if st.HasKey {
		t.Errorf("status = %+v, want no key with no config dir", st)
	}
	if _, err := a.SaveDeepLAPIKey(testDeepLKey, "free"); err == nil {
		t.Error("want an error saving with no settings folder to save into")
	}
}
