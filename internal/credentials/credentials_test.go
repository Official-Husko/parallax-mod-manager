package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/secretbox"
)

func testBox(t *testing.T) *secretbox.Box {
	t.Helper()
	box, err := secretbox.New([]byte("test-machine-secret"))
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	return box
}

func TestSaveOpenRoundTrip(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc"), Box: testBox(t)}

	if err := m.Save(map[string]string{"username": "someone@example.com", "password": "hunter2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := m.Open("username")
	if err != nil || got != "someone@example.com" {
		t.Errorf("Open(username) = %q, %v, want the saved username back", got, err)
	}

	hasPassword, err := m.Has("password")
	if err != nil || !hasPassword {
		t.Errorf("Has(password) = %v, %v, want true", hasPassword, err)
	}
}

func TestSealedFileNeverContainsThePlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loverslab.jsonc")
	m := &Manager{Service: "loverslab", Path: path, Box: testBox(t)}
	if err := m.Save(map[string]string{"username": "someone@example.com", "password": "hunter2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "someone@example.com") || strings.Contains(text, "hunter2") {
		t.Errorf("the saved file contains a plaintext credential:\n%s", text)
	}
	if !strings.Contains(text, "ENCRYPTED") {
		t.Error("the saved file has no explanation that its values are encrypted")
	}
}

func TestSaveAddsWithoutDroppingOtherFields(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc"), Box: testBox(t)}
	if err := m.Save(map[string]string{"username": "someone@example.com"}); err != nil {
		t.Fatalf("Save(username): %v", err)
	}
	if err := m.Save(map[string]string{"password": "hunter2"}); err != nil {
		t.Fatalf("Save(password): %v", err)
	}

	username, err := m.Open("username")
	if err != nil || username != "someone@example.com" {
		t.Errorf("username after a later, unrelated Save = %q, %v, want it preserved", username, err)
	}
	hasPassword, _ := m.Has("password")
	if !hasPassword {
		t.Error("expected the password saved after the username to also be present")
	}
}

func TestSaveReplacesAnExistingField(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc"), Box: testBox(t)}
	if err := m.Save(map[string]string{"password": "old"}); err != nil {
		t.Fatalf("Save(old): %v", err)
	}
	if err := m.Save(map[string]string{"password": "new"}); err != nil {
		t.Fatalf("Save(new): %v", err)
	}
	got, err := m.Open("password")
	if err != nil || got != "new" {
		t.Errorf("Open(password) = %q, %v, want %q", got, err, "new")
	}
}

func TestServicesDoNotCrossOpen(t *testing.T) {
	dir := t.TempDir()
	box := testBox(t)
	a := &Manager{Service: "loverslab", Path: filepath.Join(dir, "loverslab.jsonc"), Box: box}
	if err := a.Save(map[string]string{"password": "hunter2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A different Manager for a different service, pointed at the SAME file, must
	// not be able to open a.'s sealed value - the seal is bound to the service name,
	// not just the field name.
	b := &Manager{Service: "some-other-service", Path: a.Path, Box: box}
	if _, err := b.Open("password"); err == nil {
		t.Error("a different service's Manager opened another service's sealed field")
	}
}

func TestOpenMissingFieldErrors(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc"), Box: testBox(t)}
	if _, err := m.Open("password"); err == nil {
		t.Error("expected an error opening a field that was never saved")
	}
}

func TestClearRemovesEverything(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc"), Box: testBox(t)}
	if err := m.Save(map[string]string{"username": "someone@example.com", "password": "hunter2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	hasAny, err := m.HasAny()
	if err != nil || hasAny {
		t.Errorf("HasAny after Clear = %v, %v, want false", hasAny, err)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "does-not-exist.jsonc"), Box: testBox(t)}
	hasAny, err := m.HasAny()
	if err != nil || hasAny {
		t.Errorf("HasAny on a missing file = %v, %v, want false, nil", hasAny, err)
	}
}

func TestEmptyPathFailsSaveButNotLoad(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: "", Box: testBox(t)}
	if err := m.Save(map[string]string{"username": "x"}); err == nil {
		t.Error("expected Save with no path to fail")
	}
	hasAny, err := m.HasAny()
	if err != nil || hasAny {
		t.Errorf("HasAny with no path = %v, %v, want false, nil", hasAny, err)
	}
}

func TestNilBoxFailsSaveAndOpen(t *testing.T) {
	m := &Manager{Service: "loverslab", Path: filepath.Join(t.TempDir(), "loverslab.jsonc")}
	if err := m.Save(map[string]string{"username": "x"}); err == nil {
		t.Error("expected Save with no Box to fail")
	}
}

func TestUnreadableOnAnotherMachine(t *testing.T) {
	dir := t.TempDir()
	boxA, _ := secretbox.New([]byte("machine-a"))
	boxB, _ := secretbox.New([]byte("machine-b"))
	m := &Manager{Service: "loverslab", Path: filepath.Join(dir, "loverslab.jsonc"), Box: boxA}
	if err := m.Save(map[string]string{"password": "hunter2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	other := &Manager{Service: "loverslab", Path: m.Path, Box: boxB}
	if _, err := other.Open("password"); err != secretbox.ErrCannotOpen {
		t.Errorf("Open on another machine = %v, want secretbox.ErrCannotOpen", err)
	}
}
