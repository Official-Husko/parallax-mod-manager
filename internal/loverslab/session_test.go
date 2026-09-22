package loverslab

import (
	"strings"
	"testing"
)

func TestExportImportSessionRoundTrips(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.ImportSession(`[{"name":"ips4_member_id","value":"98765"},{"name":"ips4_login_key","value":"secret-key"},{"name":"ips4_device_key","value":"device-abc"}]`); err != nil {
		t.Fatalf("ImportSession: %v", err)
	}

	exported, err := a.ExportSession()
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if strings.Contains(exported, "session.json") {
		t.Error("ExportSession should return raw JSON, not a file reference")
	}

	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.ImportSession(exported); err != nil {
		t.Fatalf("ImportSession(exported): %v", err)
	}
	if !b.IsLoggedIn() || b.MemberID() != "98765" {
		t.Errorf("session did not round-trip: IsLoggedIn=%v MemberID=%q", b.IsLoggedIn(), b.MemberID())
	}
}

func TestImportSessionRejectsGarbage(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.ImportSession("not json at all"); err == nil {
		t.Error("expected ImportSession to fail on invalid JSON")
	}
}

func TestExportSessionOnAFreshClientIsEmptyButValid(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	exported, err := c.ExportSession()
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	// A fresh client has no cookies at all, which marshals as JSON null, not
	// an empty array - both are valid JSON and ImportSession must accept it.
	other, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := other.ImportSession(exported); err != nil {
		t.Errorf("re-importing a fresh (empty) session failed: %v", err)
	}
}
