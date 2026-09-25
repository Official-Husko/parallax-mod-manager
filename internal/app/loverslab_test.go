package app

import (
	"context"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabcategories"
)

func TestFindCategoryByName(t *testing.T) {
	categories := []loverslab.Category{
		{Name: "Skyrim: Special Edition", Depth: 0},
		{Name: "Other", Depth: 0},
		{Name: "Paradox Games", Depth: 1},
	}
	got := findCategoryByName(categories, "paradox games")
	if got == nil || got.Name != "Paradox Games" {
		t.Fatalf("got %v, want the Paradox Games category", got)
	}
	// Case and surrounding whitespace in the real site's own markup should not matter.
	got2 := findCategoryByName([]loverslab.Category{{Name: "  PARADOX GAMES  "}}, "paradox games")
	if got2 == nil {
		t.Error("expected a case/whitespace-insensitive match")
	}
	if findCategoryByName(categories, "no such category") != nil {
		t.Error("expected no match for a name that is not present")
	}
}

func TestBuildBrowseSidebarPutsAllFirstThenRealSubcategories(t *testing.T) {
	paradox := loverslab.Category{ID: 194, Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675, Depth: 1}
	subs := []loverslab.Category{
		{Name: "Crusader Kings 2", URL: "https://www.loverslab.com/files/category/162-crusader-kings-2/", Files: 149},
		{Name: "Crusader Kings 3", URL: "https://www.loverslab.com/files/category/263-crusader-kings-3/", Files: 277},
		{Name: "Stellaris", URL: "https://www.loverslab.com/files/category/192-stellaris/", Files: 249},
	}

	got := buildBrowseSidebar(paradox, subs)

	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4 (All + 3 subcategories): %+v", len(got), got)
	}
	if got[0].Name != "All" || got[0].URL != paradox.URL || got[0].Files != 675 || got[0].Depth != 0 {
		t.Errorf("first entry = %+v, want the relabeled Paradox Games category as All at depth 0", got[0])
	}
	for i, want := range subs {
		row := got[i+1]
		if row.Name != want.Name || row.URL != want.URL || row.Files != want.Files {
			t.Errorf("entry %d = %+v, want %+v", i+1, row, want)
		}
		if row.Depth != 1 {
			t.Errorf("entry %d Depth = %d, want 1", i+1, row.Depth)
		}
	}
}

func TestBuildBrowseSidebarWithNoSubcategoriesIsJustAll(t *testing.T) {
	paradox := loverslab.Category{Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675}
	got := buildBrowseSidebar(paradox, nil)
	if len(got) != 1 || got[0].Name != "All" {
		t.Fatalf("got %+v, want just All", got)
	}
}

func TestBuildBrowseSidebarDropsTheSelfReferencingRow(t *testing.T) {
	// Viewing a category's own page can re-list that category itself in its own sidebar -
	// it must not show up a second time as if it were one of its own subcategories.
	paradox := loverslab.Category{Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675}
	subs := []loverslab.Category{
		{Name: "Paradox Games", URL: paradox.URL, Files: 675},
		{Name: "Stellaris", URL: "https://www.loverslab.com/files/category/192-stellaris/", Files: 249},
	}
	got := buildBrowseSidebar(paradox, subs)
	if len(got) != 2 {
		t.Fatalf("got %+v, want All + Stellaris only", got)
	}
	if got[1].Name != "Stellaris" {
		t.Errorf("got[1] = %+v, want Stellaris", got[1])
	}
}

// --- ensureLoversLabSession: reuse, caching, and the fallback chain ---
// All scripted (newLoversLabApp), never a real request - see loverslab_settings_test.go.

func TestEnsureLoversLabSessionReusesARecentlyVerifiedClientWithoutReVerifying(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	firstClient := a.loverslab.client

	verifyCalls := 0
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		verifyCalls++
		return true, nil
	}

	client, err := a.ensureLoversLabSession(context.Background())
	if err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if client != firstClient {
		t.Error("expected the same in-memory client to be reused")
	}
	if verifyCalls != 0 {
		t.Errorf("verify called %d times, want 0 (still within sessionRecheckInterval)", verifyCalls)
	}
}

func TestEnsureLoversLabSessionReVerifiesOnceTheRecheckIntervalHasPassed(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	a.loverslab.verifiedAt = time.Now().Add(-sessionRecheckInterval - time.Minute)

	verifyCalls := 0
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		verifyCalls++
		return true, nil
	}

	if _, err := a.ensureLoversLabSession(context.Background()); err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if verifyCalls != 1 {
		t.Errorf("verify called %d times, want 1", verifyCalls)
	}
	if time.Since(a.loverslab.verifiedAt) > time.Minute {
		t.Errorf("verifiedAt = %v, want it refreshed to roughly now", a.loverslab.verifiedAt)
	}
}

func TestEnsureLoversLabSessionFallsBackToTheSavedSessionWhenTheLiveClientFailsVerification(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	a.loverslab.verifiedAt = time.Now().Add(-sessionRecheckInterval - time.Minute)

	// The in-memory client fails verification exactly once (simulating a session
	// revoked server-side); the freshly re-imported saved session succeeds.
	failedOnce := false
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		if !failedOnce {
			failedOnce = true
			return false, nil
		}
		return true, nil
	}

	client, err := a.ensureLoversLabSession(context.Background())
	if err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if client == nil || a.loverslab.client == nil {
		t.Error("expected a freshly re-imported client to be cached")
	}
}

func TestEnsureLoversLabSessionFallsBackToAFreshLoginWhenTheSavedSessionIsUnusable(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	// Simulate a session that expired entirely (not just gone stale in memory): drop
	// the in-memory client and corrupt the saved session, but keep username/password.
	a.loverslab.client = nil
	a.loverslab.verifiedAt = time.Time{}
	if err := a.loverslab.mgr.Save(map[string]string{"session": "not a valid session at all"}); err != nil {
		t.Fatalf("corrupting the saved session: %v", err)
	}

	loginCalls := 0
	realLogin := a.loverslab.login
	a.loverslab.login = func(ctx context.Context, auth, password string) (*loverslab.Client, error) {
		loginCalls++
		return realLogin(ctx, auth, password)
	}

	if _, err := a.ensureLoversLabSession(context.Background()); err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if loginCalls != 1 {
		t.Errorf("login called %d times, want 1", loginCalls)
	}
}

func TestEnsureLoversLabSessionWithNothingEverSavedFails(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.ensureLoversLabSession(context.Background()); err == nil {
		t.Error("expected an error when nothing has ever been signed in")
	}
}

// --- LoversLabCategories: the on-disk sidebar cache ---
// fakeLoversLabSession's client has no working ListCategories/ListSubcategories (they would
// make a real request against the real site) - so a passing test here, returning exactly the
// pre-seeded fake data below rather than hanging or failing on a real network call, is itself
// proof the cache-hit path never touched the client at all.

func TestLoversLabCategoriesReturnsAFreshCacheWithoutCallingTheClient(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	a.loversLabCategories = loverslabcategories.Store{Dir: t.TempDir()}

	seeded := []loverslab.Category{{ID: 999, Name: "Seeded From The Cache, Not A Real Fetch", URL: "https://example.invalid/seeded/", Depth: 0}}
	if err := a.loversLabCategories.Save(seeded); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := a.LoversLabCategories()
	if err != nil {
		t.Fatalf("LoversLabCategories: %v", err)
	}
	if len(got) != 1 || got[0].Name != seeded[0].Name {
		t.Errorf("LoversLabCategories = %+v, want the seeded cache back untouched", got)
	}
}

func TestLoversLabCategoriesStillRequiresASignedInSessionEvenWithAFreshCache(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir()) // never signed in
	a.loversLabCategories = loverslabcategories.Store{Dir: t.TempDir()}
	if err := a.loversLabCategories.Save([]loverslab.Category{{ID: 1, Name: "Cached"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := a.LoversLabCategories(); err == nil {
		t.Error("expected an error: a fresh cache must not bypass the sign-in requirement")
	}
}

// --- commentParagraphsToHTML: comment drafts, run by run ---

func TestCommentParagraphsToHTMLPlainText(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{{Text: "Same issue here on 4.0.21."}}}})
	want := "<p>Same issue here on 4.0.21.</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLMultipleParagraphs(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{
		{Runs: []CommentRun{{Text: "First paragraph."}}},
		{Runs: []CommentRun{{Text: "Second paragraph."}}},
	})
	want := "<p>First paragraph.</p><p>Second paragraph.</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLBoldItalicAndPlainRunsInOneParagraph(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{
		{Text: "This is "},
		{Text: "bold", Bold: true},
		{Text: " and "},
		{Text: "italic", Italic: true},
		{Text: " and "},
		{Text: "both", Bold: true, Italic: true},
		{Text: "."},
	}}})
	want := "<p>This is <strong>bold</strong> and <em>italic</em> and <em><strong>both</strong></em>.</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLLink(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{
		{Text: "See "},
		{Text: "the changelog", LinkURL: "https://www.loverslab.com/files/file/12345-x/?do=history"},
		{Text: " for details."},
	}}})
	want := `<p>See <a href="https://www.loverslab.com/files/file/12345-x/?do=history">the changelog</a> for details.</p>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestCommentParagraphsToHTMLEscapesUserTypedMarkup is the whole reason this is built
// from structured runs rather than accepting a raw HTML string: a run's own typed
// text must never be interpreted as markup, only the run's own Bold/Italic/LinkURL
// fields decide what tags wrap it.
func TestCommentParagraphsToHTMLEscapesUserTypedMarkup(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{
		{Text: `<script>alert(1)</script> & "quotes" & 5 < 10`},
	}}})
	want := "<p>&lt;script&gt;alert(1)&lt;/script&gt; &amp; &#34;quotes&#34; &amp; 5 &lt; 10</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLLinkURLIsAlsoEscaped(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{
		{Text: "link", LinkURL: `https://example.com/?a=1&b="x"`},
	}}})
	want := `<p><a href="https://example.com/?a=1&amp;b=&#34;x&#34;">link</a></p>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLNewlineWithinARunBecomesBr(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{{Text: "line one\nline two"}}}})
	want := "<p>line one<br>line two</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLDropsEmptyParagraphsAndRuns(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{
		{Runs: []CommentRun{{Text: ""}}},
		{Runs: nil},
		{Runs: []CommentRun{{Text: ""}, {Text: "real content"}, {Text: ""}}},
	})
	want := "<p>real content</p>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommentParagraphsToHTMLAllEmptyReturnsEmptyString(t *testing.T) {
	got := commentParagraphsToHTML([]CommentParagraph{{Runs: []CommentRun{{Text: "  "}}}, {Runs: nil}, {}})
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
	if got := commentParagraphsToHTML(nil); got != "" {
		t.Errorf("nil paragraphs: got %q, want empty string", got)
	}
}

// --- LoversLabPostComment: wiring, not the site's own real behavior (see docs/loverslab.md) ---

func TestLoversLabPostCommentSendsTheBuiltHTMLAndRefusesAnEmptyDraft(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}

	var sentContent string
	a.loverslab.postComment = func(ctx context.Context, client *loverslab.Client, filePageURL, content string) error {
		sentContent = content
		return nil
	}

	draft := []CommentParagraph{{Runs: []CommentRun{{Text: "hello "}, {Text: "world", Bold: true}}}}
	if err := a.LoversLabPostComment("https://www.loverslab.com/files/file/1-x/", draft); err != nil {
		t.Fatalf("LoversLabPostComment: %v", err)
	}
	if want := "<p>hello <strong>world</strong></p>"; sentContent != want {
		t.Errorf("sent content = %q, want %q", sentContent, want)
	}

	if err := a.LoversLabPostComment("https://www.loverslab.com/files/file/1-x/", []CommentParagraph{{Runs: []CommentRun{{Text: "   "}}}}); err == nil {
		t.Error("expected an error posting an all-whitespace draft")
	}
}
