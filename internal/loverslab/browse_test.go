package loverslab

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseFixture(t *testing.T, fragment string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return doc
}

// The sidebar markup quirk documented in docs/browsing.md: a top-level
// section has its count in a <span>, a subcategory has it in a <strong> -
// with the name text in whichever tag the count isn't in.
const categorySidebarFixture = `<html><body>
<ul class="ipsSideMenu_list">
  <li>
    <a href="/files/category/163-skyrim-special-edition/" class='ipsSideMenu_item ipsTruncate ipsTruncate_line'>
      <span class='ipsBadge ipsBadge_style1 ipsPos_right cDownloadsCategoryCount'>3582</span>
      <strong class='ipsType_normal'>Skyrim: Special Edition</strong>
    </a>
    <ul class="ipsSideMenu_list">
      <li>
        <a href="/files/category/165-regular-mods/" class='ipsSideMenu_item ipsTruncate ipsTruncate_line'>
          <strong class='ipsPos_right ipsType_small cDownloadsCategoryCount'>1201</strong>Regular Mods
        </a>
      </li>
    </ul>
  </li>
  <li>
    <a href='/files/category/161-the-sims-4/' class='ipsSideMenu_item'>
      <span class='ipsType_light ipsType_small'>(and 7 more)</span>
    </a>
  </li>
</ul>
</body></html>`

func TestParseCategoriesHandlesSwappedCountAndNameTags(t *testing.T) {
	doc := parseFixture(t, categorySidebarFixture)
	cats := parseCategories(doc)

	if len(cats) != 2 {
		t.Fatalf("expected 2 real categories (the expander excluded), got %d: %+v", len(cats), cats)
	}

	top := cats[0]
	if top.ID != 163 || top.Name != "Skyrim: Special Edition" || top.Files != 3582 || top.Depth != 0 {
		t.Errorf("top-level category = %+v, want {ID:163 Name:Skyrim: Special Edition Files:3582 Depth:0}", top)
	}

	sub := cats[1]
	if sub.ID != 165 || sub.Name != "Regular Mods" || sub.Files != 1201 || sub.Depth != 1 {
		t.Errorf("subcategory = %+v, want {ID:165 Name:Regular Mods Files:1201 Depth:1}", sub)
	}
}

func TestParseCategoriesExcludesAndMoreExpander(t *testing.T) {
	doc := parseFixture(t, categorySidebarFixture)
	cats := parseCategories(doc)
	for _, c := range cats {
		if strings.Contains(c.Name, "and 7 more") {
			t.Errorf("the '(and N more)' expander was treated as a real category: %+v", c)
		}
	}
}

// The prefix-tag trap documented in docs/browsing.md: a tag link appears
// before the real title inside the same <h4>, and must not be mistaken for
// it. This fixture also matches the real, current markup confirmed live on
// https://www.loverslab.com/files/category/192-stellaris/ (this app's own
// listing came back completely empty on every real page until this was
// fixed): the title's own <h4> no longer carries an "ipsDataItem_title"
// class at all (just "ipsContained_container"), and the listing no longer
// has a <time> element anywhere - the site dropped the per-file "updated"
// timestamp from this view entirely, so FileSummary.Updated is legitimately
// blank now rather than something still parseable here.
const fileListingFixture = `<html><body>
<ul class='ipsPagination' data-pages='49' data-ipsPagination-perPage='25'></ul>
<li class="ipsDataItem">
  <h4 class='ipsContained_container'>
    <span><span class='ipsItemStatus'>status</span></span>
    <span>
      <a href="/tags/immersion/" class='ipsTag_prefix'><span>immersion</span></a>
    </span>
    <span class='ipsType_semiBold ipsType_normal ipsType_break ipsContained'>
      <a href='/files/file/50948-sit-with-me-.../'>Sit With Me - sit with your buddies</a>
    </span>
  </h4>
  <a class="ipsType_break" href="/profile/12345-someauthor/">SomeAuthor</a>
  <a class=" ipsThumb ipsThumb_large ipsThumb_bg" href="/files/file/50948-sit-with-me-.../">
    <img src="https://static.loverslab.com/screenshots/monthly_2026_08/thumb.gif">
  </a>
</li>
<li class="ipsDataItem">
  <h4 class='ipsContained_container'>
    <span class='ipsType_semiBold ipsType_normal ipsType_break ipsContained'>
      <a href='/files/file/12-plain-file/'>A Plain File With No Prefix Tag</a>
    </span>
  </h4>
</li>
</body></html>`

func TestParseFileListingSkipsThePrefixTagLink(t *testing.T) {
	doc := parseFixture(t, fileListingFixture)
	files, totalPages := parseFileListing(doc)

	if totalPages != 49 {
		t.Errorf("totalPages = %d, want 49 (from data-pages)", totalPages)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(files), files)
	}

	f := files[0]
	if f.ID != 50948 {
		t.Errorf("ID = %d, want 50948 (the tag link's own /tags/immersion/ href must never be used)", f.ID)
	}
	if f.Title != "Sit With Me - sit with your buddies" {
		t.Errorf("Title = %q, want the real title, not the prefix tag's own text", f.Title)
	}
	if f.URL != "/files/file/50948-sit-with-me-.../" {
		t.Errorf("URL = %q, want the title link's href, not /tags/immersion/", f.URL)
	}
	if f.Author != "SomeAuthor" || f.AuthorURL != "/profile/12345-someauthor/" {
		t.Errorf("author = %q / %q, want SomeAuthor / /profile/12345-someauthor/", f.Author, f.AuthorURL)
	}
	if f.Updated != "" {
		t.Errorf("Updated = %q, want empty - the real listing no longer has a <time> element at all", f.Updated)
	}
	if f.ThumbnailURL != "https://static.loverslab.com/screenshots/monthly_2026_08/thumb.gif" {
		t.Errorf("ThumbnailURL = %q", f.ThumbnailURL)
	}
}

func TestParseFileListingHandlesAFileWithNoPrefixTag(t *testing.T) {
	doc := parseFixture(t, fileListingFixture)
	files, _ := parseFileListing(doc)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	f := files[1]
	if f.ID != 12 || f.Title != "A Plain File With No Prefix Tag" {
		t.Errorf("second file = %+v, want ID 12 with its own plain title", f)
	}
	if f.Author != "" || f.ThumbnailURL != "" {
		t.Errorf("second file has no author/thumbnail markup, expected both empty, got author=%q thumb=%q", f.Author, f.ThumbnailURL)
	}
}

func TestParseFileListingDefaultsToOnePageWithoutPagination(t *testing.T) {
	doc := parseFixture(t, `<html><body><li class="ipsDataItem"></li></body></html>`)
	_, totalPages := parseFileListing(doc)
	if totalPages != 1 {
		t.Errorf("totalPages with no pagination element = %d, want 1", totalPages)
	}
}

func TestFileIDFromURL(t *testing.T) {
	id, ok := FileIDFromURL("https://www.loverslab.com/files/file/8719-stellaris-lustful-void/")
	if !ok || id != 8719 {
		t.Errorf("FileIDFromURL = %d, %v, want 8719, true", id, ok)
	}
	if _, ok := FileIDFromURL("https://www.loverslab.com/topic/119724-not-a-file-page/"); ok {
		t.Error("expected false for a URL with no /files/file/ id in it at all")
	}
}

func TestSideMenuDepthTopLevelIsZero(t *testing.T) {
	doc := parseFixture(t, categorySidebarFixture)
	cats := parseCategories(doc)
	if cats[0].Depth != 0 {
		t.Errorf("top-level Depth = %d, want 0", cats[0].Depth)
	}
	if cats[1].Depth != 1 {
		t.Errorf("subcategory Depth = %d, want 1", cats[1].Depth)
	}
}
