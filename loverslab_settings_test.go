package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

func newLoversLabApp(t *testing.T, dir string) *App {
	t.Helper()
	a := &App{}
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
	s, err := a.SaveLoversLabCredentials("someone@example.com", "hunter2")
	if err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	if !s.SignedIn || s.Username != "someone@example.com" {
		t.Errorf("status after saving = %+v, want signed in as someone@example.com", s)
	}

	// A fresh status call (mirroring what a reload of the panel would do) sees the
	// same thing.
	again := a.LoversLabStatus()
	if !again.SignedIn || again.Username != "someone@example.com" {
		t.Errorf("LoversLabStatus after save = %+v, want it to persist", again)
	}
}

func TestSaveLoversLabCredentialsRejectsEmptyFields(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials("", "hunter2"); err == nil {
		t.Error("expected an error for an empty username")
	}
	if _, err := a.SaveLoversLabCredentials("someone@example.com", ""); err == nil {
		t.Error("expected an error for an empty password")
	}
	if s := a.LoversLabStatus(); s.SignedIn {
		t.Error("a rejected save should not have signed anything in")
	}
}

func TestSaveLoversLabCredentialsTrimsUsername(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	s, err := a.SaveLoversLabCredentials("  someone@example.com  ", "hunter2")
	if err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	if s.Username != "someone@example.com" {
		t.Errorf("Username = %q, want it trimmed", s.Username)
	}
}

func TestClearLoversLabCredentials(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials("someone@example.com", "hunter2"); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	s, err := a.ClearLoversLabCredentials()
	if err != nil {
		t.Fatalf("ClearLoversLabCredentials: %v", err)
	}
	if s.SignedIn || s.Username != "" {
		t.Errorf("status after clearing = %+v, want signed out", s)
	}
}

func TestSaveLoversLabCredentialsWithNoConfigDirFails(t *testing.T) {
	a := newLoversLabApp(t, "")
	if _, err := a.SaveLoversLabCredentials("someone@example.com", "hunter2"); err == nil {
		t.Error("expected an error when there is no settings folder to save into")
	}
}

func TestLoversLabFileNeverContainsThePlaintext(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	if _, err := a.SaveLoversLabCredentials("someone@example.com", "hunter2"); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, loversLabFileName))
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "someone@example.com") || strings.Contains(text, "hunter2") {
		t.Errorf("the saved file contains a plaintext credential:\n%s", text)
	}
}

func TestLoversLabUnreadableOnAnotherComputer(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	if _, err := a.SaveLoversLabCredentials("someone@example.com", "hunter2"); err != nil {
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
