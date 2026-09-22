package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
)

// LoversLabFileList is one page of a category's file listing, for the Browse tab -
// see loverslab.Client.ListFiles, which returns the same pair as two separate values;
// Wails bound methods return a single value plus an error, so they're wrapped here the
// same way library.ModFiles wraps ListModFiles' own pair.
type LoversLabFileList struct {
	Files      []loverslab.FileSummary
	TotalPages int
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
		if ok, err := a.loverslab.client.VerifySession(ctx); err == nil && ok {
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
			if ok, err := client.VerifySession(ctx); err == nil && ok {
				a.loverslab.client = client
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
	return LoversLabFileList{Files: files, TotalPages: totalPages}, nil
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
