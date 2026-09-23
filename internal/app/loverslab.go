package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabmeta"
)

// sessionRecheckInterval is how long an already-verified LoversLab session is trusted before
// ensureLoversLabSession checks it against the site again - browsing a page (categories, then
// its files, then a changelog) makes several calls in quick succession, and re-verifying on
// every single one would mean a burst of extra requests to loverslab.com for no real benefit;
// the three "remember me" cookies that actually matter here are valid for weeks (see
// docs/loverslab.md), so a session that was good a few minutes ago is still overwhelmingly
// likely to be good now.
const sessionRecheckInterval = 5 * time.Minute

// LoversLabFileList is one page of a category's file listing, for the Browse tab -
// see loverslab.Client.ListFiles, which returns the same pair as two separate values;
// Wails bound methods return a single value plus an error, so they're wrapped here the
// same way library.ModFiles wraps ListModFiles' own pair.
type LoversLabFileList struct {
	Files      []LoversLabFileSummary
	TotalPages int
}

// LoversLabFileSummary is one browsing-grid card's real data: the listing
// page's own fields (see loverslab.FileSummary) plus whatever this app has
// separately, opportunistically learned about that same file from a real
// visit to its own detail page (see internal/loverslabmeta and
// cacheFileMeta) - AuthorAvatarURL, Views and RealUpdated are blank for any
// file never opened yet, the same as before any of this existed; they are
// never fetched just to fill a card in.
type LoversLabFileSummary struct {
	ID              int
	Title           string
	URL             string
	Author          string
	AuthorURL       string
	Updated         string // loverslab.FileSummary's own listing-page display string - see RealUpdated below, which is what actually has a value today
	ThumbnailURL    string
	AuthorAvatarURL string
	Views           int
	// RealUpdated is the site's own real "dateModified" ISO 8601 timestamp
	// (loverslab.FileDetail.DateModified), cached from a real detail-page
	// visit - distinct from Updated above, which is the listing page's own
	// display string and, confirmed live, is always empty now (the real
	// site stopped rendering it there entirely).
	RealUpdated string
}

// withCachedMeta fills in whatever this app has separately cached for f's
// own file ID (see internal/loverslabmeta) - f itself already has every
// field the listing page ever provides; this only ever adds to it, never
// overwrites a field the listing page did supply.
func withCachedMeta(f loverslab.FileSummary, cached map[int]loverslabmeta.Entry) LoversLabFileSummary {
	out := LoversLabFileSummary{
		ID: f.ID, Title: f.Title, URL: f.URL, Author: f.Author, AuthorURL: f.AuthorURL,
		Updated: f.Updated, ThumbnailURL: f.ThumbnailURL,
	}
	if e, ok := cached[f.ID]; ok {
		out.AuthorAvatarURL = e.AuthorAvatarURL
		out.Views = e.Views
		out.RealUpdated = e.Updated
	}
	return out
}

// LoversLabAccountProfile is the signed-in account's own real display name,
// profile link and avatar (see loverslab.AccountProfile) - distinct from
// LoversLabStatus.Username, which is only ever whatever was actually typed to
// sign in (an email works just as well as a username there, and is never the
// site's own display name). Fetched separately, lazily, since it needs a real
// request and LoversLabStatus itself stays local/no-network by design.
type LoversLabAccountProfile struct {
	Username   string
	ProfileURL string
	AvatarURL  string
}

// ensureLoversLabSession returns a signed-in client, reusing this run's existing one
// if it is still good, otherwise trying (in order) a saved session and a fresh login
// with the saved username/password - the same fallback chain SteamAPIStatus's own key
// decrypt-at-startup follows, just resolved lazily here since browsing, unlike the key,
// has no reason to sign in before anything actually asks for it.
func (a *App) ensureLoversLabSession(ctx context.Context) (*loverslab.Client, error) {
	a.loverslab.mu.Lock()
	defer a.loverslab.mu.Unlock()

	if a.loverslab.client != nil {
		if time.Since(a.loverslab.verifiedAt) < sessionRecheckInterval {
			return a.loverslab.client, nil
		}
		if ok, err := a.loverslab.verify(ctx, a.loverslab.client); err == nil && ok {
			a.loverslab.verifiedAt = time.Now()
			return a.loverslab.client, nil
		}
		// Either revoked server-side or the check itself failed (network hiccup) -
		// either way, fall through and try to establish a fresh one below rather
		// than keep serving a client that just failed verification.
		a.loverslab.client = nil
	}

	if a.loverslab.mgr == nil {
		return nil, errors.New("LoversLab sign-in is not ready yet")
	}

	client, err := loverslab.New()
	if err != nil {
		return nil, fmt.Errorf("setting up the LoversLab client: %w", err)
	}

	if session, err := a.loverslab.mgr.Open("session"); err == nil {
		if err := client.ImportSession(session); err == nil {
			if ok, err := a.loverslab.verify(ctx, client); err == nil && ok {
				a.loverslab.client = client
				a.loverslab.verifiedAt = time.Now()
				return client, nil
			}
		}
	}

	username, uErr := a.loverslab.mgr.Open("username")
	password, pErr := a.loverslab.mgr.Open("password")
	if uErr != nil || pErr != nil {
		return nil, errors.New("sign in to LoversLab first")
	}
	client, err = a.loverslab.login(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("could not sign in to LoversLab: %w", err)
	}
	if session, err := client.ExportSession(); err == nil {
		if err := a.loverslab.mgr.Save(map[string]string{"session": session}); err != nil {
			applog.For("LoversLab").Warnf("signed in again, but could not save the refreshed session: %v", err)
		}
	}
	applog.For("LoversLab").Infof("signed in to LoversLab again (member %s)", client.MemberID())
	a.loverslab.client = client
	a.loverslab.verifiedAt = time.Now()
	return client, nil
}

// paradoxGamesCategoryName is the one real LoversLab category every Paradox game's mods share
// (confirmed against the real site: "Other" > "Paradox Games", currently id 194) - unlike
// Skyrim, Fallout or The Sims, LoversLab does not give most Paradox games their own top-level
// section; only a few (currently Crusader Kings II, Crusader Kings III and Stellaris) have a
// real subcategory of their own within it. Everything else this app manages (Europa
// Universalis IV, Hearts of Iron IV, Victoria 3, Imperator: Rome) has no subcategory at all and
// is only reachable by browsing "Paradox Games" itself, unfiltered. See docs/loverslab.md.
const paradoxGamesCategoryName = "paradox games"

// allCategoryName is the synthetic first entry LoversLabCategories adds for browsing every
// Paradox game's mods together, unfiltered - the real "Paradox Games" category itself, just
// relabeled since "Paradox Games" would otherwise read as one more sibling of its own real
// subcategories rather than what it actually is: all of them combined.
const allCategoryName = "All"

// LoversLabCategories lists the Browse tab's sidebar: "All" (the real "Paradox Games" category,
// unfiltered) first, then whichever of its own real subcategories currently exist for one
// specific Paradox game - everything else LoversLab hosts (Skyrim, Fallout, The Sims, and so
// on) is dropped, since this app has nothing to do with any of it.
func (a *App) LoversLabCategories() ([]loverslab.Category, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return nil, err
	}

	root, err := client.ListCategories(a.baseContext())
	if err != nil {
		applog.For("LoversLab").Warnf("listing categories failed: %v", err)
		return nil, err
	}
	paradox := findCategoryByName(root, paradoxGamesCategoryName)
	if paradox == nil {
		return nil, nil
	}

	// IPS4's sidebar only reveals a category's own deeper children while that category
	// itself is the one being viewed - this second fetch is what makes Crusader Kings
	// II/III and Stellaris visible at all; ListCategories' own site-wide tree never
	// shows them. A failure here is not fatal: "All" alone still works.
	subs, err := client.ListSubcategories(a.baseContext(), paradox.URL)
	if err != nil {
		applog.For("LoversLab").Warnf("listing Paradox Games' own subcategories failed: %v", err)
		subs = nil
	}
	return buildBrowseSidebar(*paradox, subs), nil
}

// buildBrowseSidebar turns the real "Paradox Games" category and (if it was reachable) its own
// real subcategories into the Browse tab's sidebar - pulled out of LoversLabCategories so it
// can be tested directly against fixture data instead of two real requests.
func buildBrowseSidebar(paradox loverslab.Category, subcategories []loverslab.Category) []loverslab.Category {
	all := paradox
	all.Name = allCategoryName
	all.Depth = 0
	out := []loverslab.Category{all}
	for _, cat := range subcategories {
		if cat.URL == paradox.URL {
			continue // the page's own sidebar re-lists the category being viewed itself
		}
		cat.Depth = 1
		out = append(out, cat)
	}
	return out
}

func findCategoryByName(categories []loverslab.Category, name string) *loverslab.Category {
	for i := range categories {
		if strings.ToLower(strings.TrimSpace(categories[i].Name)) == name {
			return &categories[i]
		}
	}
	return nil
}

// LoversLabFiles lists one (1-indexed) page of a category's files, for the Browse
// tab's card grid.
func (a *App) LoversLabFiles(categoryURL string, page int) (LoversLabFileList, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return LoversLabFileList{}, err
	}
	files, totalPages, err := client.ListFiles(a.baseContext(), categoryURL, page)
	if err != nil {
		applog.For("LoversLab").Warnf("listing files in %s failed: %v", categoryURL, err)
		return LoversLabFileList{}, err
	}

	// Never a reason to fail the listing itself - an unreadable or absent
	// cache just means every card falls back to what it always showed.
	cached, cacheErr := a.loverslabMeta.Load()
	if cacheErr != nil {
		applog.For("LoversLab").Warnf("reading the cached file details failed, cards will show without them: %v", cacheErr)
		cached = nil
	}
	summaries := make([]LoversLabFileSummary, 0, len(files))
	for _, f := range files {
		summaries = append(summaries, withCachedMeta(f, cached))
	}
	return LoversLabFileList{Files: summaries, TotalPages: totalPages}, nil
}

// LoversLabChangelog returns a file's release notes, if it has any - nil, no error is
// a normal result (see docs/loverslab.md): most files never had one written.
func (a *App) LoversLabChangelog(filePageURL string) ([]loverslab.ChangelogEntry, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return nil, err
	}
	entries, err := client.ListChangelog(a.baseContext(), filePageURL)
	if err != nil {
		applog.For("LoversLab").Warnf("reading the changelog for %s failed: %v", filePageURL, err)
	}
	return entries, err
}

// LoversLabFileDetail returns a file's full description and screenshot gallery, for
// the Browse tab's mod detail view.
func (a *App) LoversLabFileDetail(filePageURL string) (loverslab.FileDetail, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return loverslab.FileDetail{}, err
	}
	detail, err := a.loverslab.getFileDetail(a.baseContext(), client, filePageURL)
	if err != nil {
		applog.For("LoversLab").Warnf("getting file detail for %s failed: %v", filePageURL, err)
		return detail, err
	}
	a.cacheFileMeta(filePageURL, detail)
	return detail, nil
}

// cacheFileMeta opportunistically saves whatever detail's own author avatar/
// view count/updated date is, keyed by the file ID in filePageURL, so a
// later visit to the browsing grid can show it on that file's own card - see
// internal/loverslabmeta. Never fails the caller: a file whose ID can't be
// parsed from its own URL, or a cache that can't be read/saved, just means
// this one opportunity to enrich a future card is missed, nothing more.
func (a *App) cacheFileMeta(filePageURL string, detail loverslab.FileDetail) {
	id, ok := loverslab.FileIDFromURL(filePageURL)
	if !ok {
		return
	}
	log := applog.For("LoversLab")
	cached, err := a.loverslabMeta.Load()
	if err != nil {
		log.Warnf("reading the cached file details failed, not caching %s: %v", filePageURL, err)
		return
	}
	cached = loverslabmeta.With(cached, id, loverslabmeta.Entry{
		AuthorAvatarURL: detail.Author.ImageURL,
		Views:           detail.Views,
		Updated:         detail.DateModified,
		CachedAt:        time.Now().Unix(),
	})
	if err := a.loverslabMeta.Save(cached); err != nil {
		log.Warnf("saving the cached file details failed for %s: %v", filePageURL, err)
	}
}

// LoversLabCommentList is one page of a file's support-topic replies (see
// docs/loverslab.md's Comments section: files have no native comments, this is the
// closest thing) - wrapped the same way LoversLabFileList wraps ListFiles' own pair.
type LoversLabCommentList struct {
	Posts      []loverslab.Post
	TotalPages int
	// HasTopic is false when the file has no linked support topic at all - lets the
	// frontend tell "nothing to write a comment to" apart from "has a topic, just
	// zero replies so far, be the first."
	HasTopic bool
	// TopicAuthor is who started the support topic - a forum topic's own post #1 is
	// always its opening post (confirmed live: its author and posted date always
	// match the file's own author/submitted date), not a real reply, so it never
	// appears in Posts at all - only its author survives, for badging any of their
	// later real replies as the topic's own author. Only ever set from a page 1
	// fetch (where that post is actually seen); the frontend keeps reusing it across
	// later pages of the same topic rather than losing it once the page moves on.
	TopicAuthor string
}

// LoversLabComments returns one (1-indexed) page of a file's linked support-topic
// replies. A file with no linked topic at all returns a zero-value, no-error result
// (HasTopic false) - not an error, since plenty of files simply don't have one.
//
// This resolves the support topic URL itself (client.SupportTopicURL) rather than
// calling the package's own ListFileSupportPosts convenience wrapper, which would
// just do the same lookup again internally - HasTopic needs to see that result
// directly, and this way it costs nothing extra.
func (a *App) LoversLabComments(filePageURL string, page int) (LoversLabCommentList, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return LoversLabCommentList{}, err
	}
	topicURL, err := client.SupportTopicURL(a.baseContext(), filePageURL)
	if err != nil {
		applog.For("LoversLab").Warnf("finding the support topic for %s failed: %v", filePageURL, err)
		return LoversLabCommentList{}, err
	}
	if topicURL == "" {
		return LoversLabCommentList{}, nil
	}
	posts, totalPages, err := client.ListTopicPosts(a.baseContext(), topicURL, page)
	if err != nil {
		applog.For("LoversLab").Warnf("listing comments for %s failed: %v", filePageURL, err)
		return LoversLabCommentList{}, err
	}
	var topicAuthor string
	if page == 1 && len(posts) > 0 {
		topicAuthor = posts[0].Author
		posts = posts[1:]
	}
	return LoversLabCommentList{Posts: posts, TotalPages: totalPages, HasTopic: true, TopicAuthor: topicAuthor}, nil
}

// LoversLabPostComment posts content as a reply to filePageURL's linked support
// topic - the write side of LoversLabComments. content is plain text typed into this
// app's own comment box; it is escaped and wrapped into the simple <p> HTML the
// site's own rich text editor actually submits (see paragraphsToHTML) before being
// sent, and returns a clear error if the file has no support topic to write to.
func (a *App) LoversLabPostComment(filePageURL, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("write something before posting")
	}
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return err
	}
	if err := a.loverslab.postComment(a.baseContext(), client, filePageURL, paragraphsToHTML(content)); err != nil {
		applog.For("LoversLab").Warnf("posting a comment on %s failed: %v", filePageURL, err)
		return err
	}
	applog.For("LoversLab").Infof("posted a comment on %s", filePageURL)
	return nil
}

// LoversLabUnreadNotifications reports how many unread notifications the signed-in
// LoversLab account currently has, read straight from the site's own bell badge
// (see docs/loverslab.md's Notifications section) - the site's own live-updating
// bell has its AJAX polling disabled server-side, confirmed live, so this is a
// normal page fetch, the same as everything else this app reads from the site, not
// an invented endpoint. Meant to be called periodically, not on every render.
func (a *App) LoversLabUnreadNotifications() (int, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return 0, err
	}
	count, err := a.loverslab.unreadNotifications(a.baseContext(), client)
	if err != nil {
		applog.For("LoversLab").Warnf("checking notifications failed: %v", err)
	}
	return count, err
}

// LoversLabProfile fetches the signed-in account's own real display name,
// profile link, and avatar - see LoversLabAccountProfile. A zero value, no
// error, when the account menu wasn't found (not actually signed in server-
// side, even though a saved sign-in exists locally).
func (a *App) LoversLabProfile() (LoversLabAccountProfile, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return LoversLabAccountProfile{}, err
	}
	profile, found, err := client.AccountProfile(a.baseContext())
	if err != nil {
		applog.For("LoversLab").Warnf("fetching the account's own profile failed: %v", err)
		return LoversLabAccountProfile{}, err
	}
	if !found {
		return LoversLabAccountProfile{}, nil
	}
	return LoversLabAccountProfile{Username: profile.Username, ProfileURL: profile.ProfileURL, AvatarURL: profile.AvatarURL}, nil
}

// paragraphsToHTML turns plain text typed into this app's own comment box into the
// simple HTML the site's real rich text editor submits for an ordinary reply (see
// docs/loverslab.md's Notifications/Comments research) - blank-line-separated
// paragraphs, each escaped and wrapped in <p>, single line breaks within a paragraph
// becoming <br>.
func paragraphsToHTML(content string) string {
	paras := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n")
	var b strings.Builder
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(p), "\n", "<br>"))
		b.WriteString("</p>")
	}
	if b.Len() == 0 {
		return "<p>" + html.EscapeString(content) + "</p>"
	}
	return b.String()
}
