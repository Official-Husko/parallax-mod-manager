package loverslab

import "testing"

const changelogSectionFixture = `<html><body>
<section data-controller="downloads.front.view.changeLog">
  <h2 class="ipsType_sectionHead">What's New in Version <span data-role='versionTitle'>1.0.2</span></h2>
  <div data-role="changeLogData">
    <p class='ipsType_reset ipsType_light ipsMargin_bottom:half'>
      Released <time datetime='2026-08-15T04:56:33Z'>August 15</time>
    </p>
    <div class='ipsType_richText ipsType_normal'>
      <p>Fixed a crash on load.</p>
      <p>See <a href="https://github.com/example/mod">the GitHub repo</a> for details.</p>
    </div>
  </div>
</section>
</body></html>`

func TestListChangelogParsesAPresentSection(t *testing.T) {
	doc := parseFixture(t, changelogSectionFixture)
	section := findOne(doc, isChangelogSection)
	if section == nil {
		t.Fatal("expected to find the changelog section")
	}
	entry, ok := parseChangelogSection(section)
	if !ok {
		t.Fatal("parseChangelogSection reported no entry for a section that has one")
	}
	if entry.Version != "1.0.2" {
		t.Errorf("Version = %q, want 1.0.2", entry.Version)
	}
	if entry.Released != "August 15" {
		t.Errorf("Released = %q, want August 15", entry.Released)
	}
	// richText() puts a trailing newline after each <p>, so two adjacent
	// paragraphs come out with a blank line between them - a reasonable
	// prose paragraph break, not a bug.
	wantDescription := "Fixed a crash on load.\n\nSee the GitHub repo (https://github.com/example/mod) for details."
	if entry.Description != wantDescription {
		t.Errorf("Description = %q, want %q", entry.Description, wantDescription)
	}

	// This fixture's rich-text div has no lightbox controller attribute (the
	// fallback path - see TestListChangelogParsesADescriptionStyleBody below
	// for the primary one), but DescriptionBlocks must still be real and
	// non-nil either way, the same as FileDetail's own.
	if len(entry.DescriptionBlocks) != 2 {
		t.Fatalf("got %d DescriptionBlocks, want 2: %+v", len(entry.DescriptionBlocks), entry.DescriptionBlocks)
	}
	if runsText(entry.DescriptionBlocks[0].Runs) != "Fixed a crash on load." {
		t.Errorf("DescriptionBlocks[0] = %+v", entry.DescriptionBlocks[0])
	}
	var linkURL string
	for _, r := range entry.DescriptionBlocks[1].Runs {
		if r.LinkURL != "" {
			linkURL = r.LinkURL
		}
	}
	if linkURL != "https://github.com/example/mod" {
		t.Errorf("DescriptionBlocks[1] = %+v, want the real link kept as a real link, not flattened", entry.DescriptionBlocks[1])
	}
}

// A changelog entry's rich text is wrapped in the exact same "can hold
// lightboxed images" controller the description body uses (confirmed live) -
// this is the primary match path, over the plain .ipsType_richText fallback
// above.
func TestListChangelogParsesADescriptionStyleBody(t *testing.T) {
	doc := parseFixture(t, `<html><body>
<section data-controller="downloads.front.view.changeLog">
  <h2 class="ipsType_sectionHead">What's New in Version <span data-role='versionTitle'>2.0.0</span></h2>
  <div data-role="changeLogData">
    <p class='ipsType_reset ipsType_light ipsMargin_bottom:half'>Released <time>Yesterday</time></p>
    <div class='ipsType_richText ipsType_normal'>
      <div class='ipsType_richText ipsType_normal' data-controller="core.front.core.lightboxedImages">
        <p># New Release</p>
        <p><strong>Bold</strong> change note.</p>
        <p><img src="https://static.loverslab.com/x.png"></p>
      </div>
    </div>
  </div>
</section>
</body></html>`)
	section := findOne(doc, isChangelogSection)
	entry, ok := parseChangelogSection(section)
	if !ok {
		t.Fatal("parseChangelogSection reported no entry")
	}
	if len(entry.DescriptionBlocks) != 3 {
		t.Fatalf("got %d DescriptionBlocks, want 3: %+v", len(entry.DescriptionBlocks), entry.DescriptionBlocks)
	}
	if entry.DescriptionBlocks[0].Heading != 1 {
		t.Errorf("DescriptionBlocks[0] = %+v, want a real Heading 1 from the literal '# ' text", entry.DescriptionBlocks[0])
	}
	if !entry.DescriptionBlocks[1].Runs[0].Bold {
		t.Errorf("DescriptionBlocks[1] = %+v, want the real <strong> bold kept", entry.DescriptionBlocks[1])
	}
	if entry.DescriptionBlocks[2].ImageURL != "https://static.loverslab.com/x.png" {
		t.Errorf("DescriptionBlocks[2] = %+v, want the embedded image", entry.DescriptionBlocks[2])
	}
}

// docs/changelogs.md: the whole <section> is simply absent when the author
// never wrote release notes - this must read as "no changelog", not an error.
func TestListChangelogSectionAbsentIsNotAnError(t *testing.T) {
	doc := parseFixture(t, `<html><body><p>no changelog widget here at all</p></body></html>`)
	section := findOne(doc, isChangelogSection)
	if section != nil {
		t.Fatal("expected no changelog section to be found")
	}
}

func TestParseChangelogSectionWithoutDataRoleFails(t *testing.T) {
	doc := parseFixture(t, `<html><body><section data-controller="downloads.front.view.changeLog"></section></body></html>`)
	section := findOne(doc, isChangelogSection)
	if section == nil {
		t.Fatal("expected to find the (empty) changelog section")
	}
	if _, ok := parseChangelogSection(section); ok {
		t.Error("expected parseChangelogSection to fail without a changeLogData node")
	}
}
