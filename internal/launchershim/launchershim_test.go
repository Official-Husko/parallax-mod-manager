package launchershim

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadStatusMissingFileIsNotAnError(t *testing.T) {
	status, found, err := ReadStatus(t.TempDir())
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if found {
		t.Error("found = true for a directory with no status file at all")
	}
	if status.ResolvedExe != "" || status.Success || len(status.Args) != 0 {
		t.Errorf("status = %+v, want the zero value", status)
	}
}

func TestReadStatusRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := Status{
		Time:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		ResolvedExe: "/games/Stellaris/stellaris",
		Args:        []string{"-gdpr-compliant"},
		Success:     true,
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, statusFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, found, err := ReadStatus(dir)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if !found {
		t.Fatal("found = false for a real status file")
	}
	if got.ResolvedExe != want.ResolvedExe || !got.Time.Equal(want.Time) || !got.Success {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadStatusMalformedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, statusFileName), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadStatus(dir); err == nil {
		t.Fatal("expected an error for a malformed status file")
	}
}

func TestPingListenerDeliversAWellFormedPing(t *testing.T) {
	received := make(chan Ping, 1)
	var bindErr error
	StartPingListener(t.Context(), func(p Ping) { received <- p }, func(err error) { bindErr = err })
	if bindErr != nil {
		t.Fatalf("StartPingListener failed to bind: %v", bindErr)
	}
	// The listener's own goroutine needs a moment to actually start Accept()ing.
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", "127.0.0.1:47813")
	if err != nil {
		t.Fatalf("dialing the ping listener: %v", err)
	}
	payload, _ := json.Marshal(Ping{ResolvedExe: "/games/Stellaris/stellaris", PID: 12345})
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("writing the ping: %v", err)
	}
	conn.Close()

	select {
	case p := <-received:
		if p.ResolvedExe != "/games/Stellaris/stellaris" || p.PID != 12345 {
			t.Errorf("received %+v, want the exact payload sent", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onPing was never called")
	}
}
