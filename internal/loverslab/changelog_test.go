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
