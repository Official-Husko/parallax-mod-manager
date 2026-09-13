package steamapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realChangelogPage is a (heavily trimmed, structure-preserved) real
// Steam Workshop changelog page, confirmed via a live request during
// development - see docs/steam-web-api.md. It keeps the real, confirmed
// quirk this parser has to handle: a <div> nested directly inside the
// body's <p id="..."> tag, invalid HTML5 that a spec-compliant parser
// auto-closes empty, leaving the real body content as further siblings
// rather than inside it.
const realChangelogPage = `<!DOCTYPE html>
<html><body>
<div class="workshopBrowsePagingControls"></div>
<div class="detailBox workshopAnnouncement noFooter changeLogCtn">
	<div class="changelog headline">
		Update: 3 May @ 7:41am
	</div>
	<div class="changelog author">by <a href="https://steamcommunity.com/id/orrie">Orrie</a></div>
	<div style="clear: right"></div>
	<p id="1777819283"><div class="bb_h1"><b>CHANGELOG v1.6:</b></div><br>1. First real change.<br><br>2. Second real change.</p>
	<div class="commentsLink changeLog">
		Discuss this update in the <a href="https://steamcommunity.com/x">discussions</a> section.
	</div>
</div>
<div class="detailBox workshopAnnouncement noFooter changeLogCtn">
	<div class="changelog headline">
		Update: 29 Apr @ 9:58pm
	</div>
	<div class="changelog author">by <a href="https://steamcommunity.com/id/orrie">Orrie</a></div>
	<div style="clear: right"></div>
	<p id="1777525080"></p>
</div>
</body></html>`

func withChangelogServer(t *testing.T, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	restore := changelogURL
	changelogURL = server.URL + "/?id=%s"
	t.Cleanup(func() { changelogURL = restore })
}

func TestGetChangelogParsesRealPage(t *testing.T) {
	withChangelogServer(t, realChangelogPage)

	entries, err := GetChangelog(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	first := entries[0]
	if first.Headline != "3 May @ 7:41am" {
		t.Errorf("first.Headline = %q", first.Headline)
	}
	if first.Author != "Orrie" {
		t.Errorf("first.Author = %q", first.Author)
	}
	if first.AuthorProfileURL != "https://steamcommunity.com/id/orrie" {
		t.Errorf("first.AuthorProfileURL = %q", first.AuthorProfileURL)
	}
	wantBody := "CHANGELOG v1.6:\n\n1. First real change.\n\n2. Second real change."
	if first.Body != wantBody {
		t.Errorf("first.Body = %q, want %q", first.Body, wantBody)
	}
}

func TestGetChangelogHandlesEmptyBody(t *testing.T) {
	withChangelogServer(t, realChangelogPage)

	entries, err := GetChangelog(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	second := entries[1]
	if second.Headline != "29 Apr @ 9:58pm" {
		t.Errorf("second.Headline = %q", second.Headline)
	}
	if second.Body != "" {
		t.Errorf("second.Body = %q, want empty (a version bump with no written notes is real and normal)", second.Body)
	}
}

func TestGetChangelogNoEntriesReturnsEmptyNotError(t *testing.T) {
	withChangelogServer(t, `<html><body><p>No changelog entries here.</p></body></html>`)

	entries, err := GetChangelog(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}

func TestGetChangelogHTTPErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restore := changelogURL
	changelogURL = server.URL + "/?id=%s"
	defer func() { changelogURL = restore }()

	if _, err := GetChangelog(context.Background(), "1"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
