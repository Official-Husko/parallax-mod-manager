package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

const (
	testLoversLabUser = "someone@example.com"
	testLoversLabPass = "hunter2"
)

// fakeLoversLabSession builds a client that looks authenticated (IsLoggedIn/MemberID
// work, since those are pure local cookie checks) without making any real request -
// used as the successful result of a scripted login in tests.
func fakeLoversLabSession(t *testing.T) *loverslab.Client {
	t.Helper()
	client, err := loverslab.New()
	if err != nil {
		t.Fatalf("loverslab.New: %v", err)
	}
	if err := client.ImportSession(`[{"name":"ips4_member_id","value":"1"},{"name":"ips4_login_key","value":"test"},{"name":"ips4_device_key","value":"test"}]`); err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	return client
}

// newLoversLabApp is an App whose LoversLab settings live in dir and whose login,
// session verification and file-detail fetching are all scripted (never a real
// request against the real site): login succeeds only for auth/password matching
// testLoversLabUser/testLoversLabPass, verify trusts whatever IsLoggedIn's own local,
// no-network cookie check already says, and getFileDetail refuses by default (tests
// that need it - loverslabupdates_test.go - replace it with their own fake) - the same
// swappable-verify pattern newSteamApp (steamapi_settings_test.go) uses for the same
// reason. Individual tests that care about exactly when/how often verify runs
// (ensureLoversLabSession's caching - see loverslab_test.go) replace that one with
// their own counting fake afterward.
func newLoversLabApp(t *testing.T, dir string) *App {
	t.Helper()
	a := &App{}
	a.loverslab.login = func(ctx context.Context, auth, password string) (*loverslab.Client, error) {
		if auth != testLoversLabUser || password != testLoversLabPass {
			return nil, errors.New("login: rejected (bad credentials, captcha, or 2FA challenge)")
		}
		return fakeLoversLabSession(t), nil
	}
	a.loverslab.verify = func(ctx context.Context, client *loverslab.Client) (bool, error) {
		return client.IsLoggedIn(), nil
	}
	a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{}, errors.New("getFileDetail: not scripted for this test")
	}
	a.initLoversLab(dir)
	return a
}

func TestLoversLabStatusStartsSignedOut(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	s := a.LoversLabStatus()
	if s.SignedIn || s.Username != "" || s.Unreadable {
		t.Errorf("fresh status = %+v, want signed out with nothing saved", s)
	}
	if !strings.Contains(s.Protection, "encrypted") {
		t.Errorf("Protection = %q, want it to mention encryption", s.Protection)
	}
}

func TestSaveLoversLabCredentialsRoundTrips(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	s, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass)
	if err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	if !s.SignedIn || s.Username != testLoversLabUser {
		t.Errorf("status after saving = %+v, want signed in as %s", s, testLoversLabUser)
	}

	// A fresh status call (mirroring what a reload of the panel would do) sees the
	// same thing.
	again := a.LoversLabStatus()
	if !again.SignedIn || again.Username != testLoversLabUser {
		t.Errorf("LoversLabStatus after save = %+v, want it to persist", again)
	}
}

func TestSaveLoversLabCredentialsRejectedLoginIsNotSaved(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, "wrong password entirely"); err == nil {
		t.Error("expected an error for a login the scripted check rejects")
	}
	if s := a.LoversLabStatus(); s.SignedIn {
		t.Error("a rejected login should not have signed anything in")
	}
}

func TestSaveLoversLabCredentialsRejectsEmptyFields(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials("", testLoversLabPass); err == nil {
		t.Error("expected an error for an empty username")
	}
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, ""); err == nil {
		t.Error("expected an error for an empty password")
	}
	if s := a.LoversLabStatus(); s.SignedIn {
		t.Error("a rejected save should not have signed anything in")
	}
}

func TestSaveLoversLabCredentialsTrimsUsername(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	s, err := a.SaveLoversLabCredentials("  "+testLoversLabUser+"  ", testLoversLabPass)
	if err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	if s.Username != testLoversLabUser {
		t.Errorf("Username = %q, want it trimmed", s.Username)
	}
}

func TestClearLoversLabCredentials(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	s, err := a.ClearLoversLabCredentials()
	if err != nil {
		t.Fatalf("ClearLoversLabCredentials: %v", err)
	}
	if s.SignedIn || s.Username != "" {
		t.Errorf("status after clearing = %+v, want signed out", s)
	}
	if a.loverslab.client != nil {
		t.Error("expected the live client to be dropped on clear")
	}
}

func TestSaveLoversLabCredentialsWithNoConfigDirFails(t *testing.T) {
	a := newLoversLabApp(t, "")
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err == nil {
		t.Error("expected an error when there is no settings folder to save into")
	}
}

func TestLoversLabFileNeverContainsThePlaintext(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, loversLabFileName))
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, testLoversLabUser) || strings.Contains(text, testLoversLabPass) {
		t.Errorf("the saved file contains a plaintext credential:\n%s", text)
	}
}

func TestLoversLabUnreadableOnAnotherComputer(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}

	// A second App pointed at the same saved file, but whose encryption box is bound
	// to a deliberately different secret (initLoversLab's own ForThisMachine can't be
	// forced to disagree with itself within one test process - both Apps would derive
	// the same key from this same machine's real id - so the "different computer" is
	// simulated directly here instead, the same way credentials_test.go's own
	// TestUnreadableOnAnotherMachine does).
	b := newLoversLabApp(t, t.TempDir())
	otherBox, err := secretbox.New([]byte("a different computer entirely"))
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	b.loverslab.mgr.Path = a.loverslab.mgr.Path
	b.loverslab.mgr.Box = otherBox

	s := b.LoversLabStatus()
	if !s.Unreadable {
		t.Errorf("status with a mismatched box = %+v, want Unreadable", s)
	}
}
