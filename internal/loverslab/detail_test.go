package loverslab

import "testing"

// Trimmed from a real file page (Stable Portraits, fetched 2026-09-23 - see
// docs/loverslab.md's "A file's detail page" section), including a couple of the
// site-wide boilerplate JSON-LD blocks that appear later on every page, to confirm
// those are correctly skipped rather than mistaken for the real one.
const fileDetailFixture = `<html><head>
<script type='application/ld+json'>
{
    "@context": "http://schema.org",
    "@type": "WebApplication",
    "url": "https://www.loverslab.com/files/file/31347-stable-portraits/",
    "name": "Stable Portraits",
    "description": "Hello,\n \n\tThis is an animated portrait pack.\n \n\t132 Elf Portraits.",
    "applicationCategory": "Stellaris",
    "downloadUrl": "https://www.loverslab.com/files/file/31347-stable-portraits/?do=download",
    "dateCreated": "2024-01-10T18:09:13+0100",
    "fileSize": "111.41 MB",
    "softwareVersion": "v2",
    "author": {
        "@type": "Person",
        "name": "Karsus",
        "image": "https://static.loverslab.com/uploads/profiles/profile/photo-thumb-993535.jpg",
        "url": "https://www.loverslab.com/profile/993535-karsus/"
    },
    "interactionStatistic": [
        {"@type": "InteractionCounter", "interactionType": "http://schema.org/ViewAction", "userInteractionCount": 33402},
        {"@type": "InteractionCounter", "interactionType": "http://schema.org/DownloadAction", "userInteractionCount": 6049}
    ],
    "dateModified": "2026-09-13T20:01:52+0200",
    "screenshot": [
        {
            "@type": "ImageObject",
            "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00066-4195690853.png.dd8fe7e146d4ba158b92ae814053e1eb.png",
            "thumbnail": {"@type": "ImageObject", "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00066-4195690853.thumb.png.f9184ebbcff34d5673a6f9e7c3579498.png"}
        },
        {
            "@type": "ImageObject",
            "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00115-4146571628.png.38c1757839e7061dd764912fc1033754.png",
            "thumbnail": {"@type": "ImageObject", "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00115-4146571628.thumb.png.07e958974137c3d6ae3d3df4cb799876.png"}
        }
    ]
}
</script>
<script type='application/ld+json'>
{"@context": "http://www.schema.org", "@type": "WebSite", "name": "LoversLab", "url": "https://www.loverslab.com/"}
</script>
<script type='application/ld+json'>
{"@context": "http://schema.org", "@type": "BreadcrumbList", "itemListElement": []}
</script>
</head><body></body></html>`

func TestParseFileDetail(t *testing.T) {
	doc := parseFixture(t, fileDetailFixture)
	detail, ok := parseFileDetail(doc)
	if !ok {
		t.Fatal("expected the WebApplication block to be found")
	}

	if detail.Title != "Stable Portraits" {
		t.Errorf("Title = %q, want Stable Portraits", detail.Title)
	}
	if detail.Description == "" || len(detail.Description) < 20 {
		t.Errorf("Description = %q, want the real description text", detail.Description)
	}
	if detail.Version != "v2" {
		t.Errorf("Version = %q, want v2", detail.Version)
	}
	if detail.FileSize != "111.41 MB" {
		t.Errorf("FileSize = %q, want 111.41 MB", detail.FileSize)
	}
	if detail.Author.Name != "Karsus" || detail.Author.URL != "https://www.loverslab.com/profile/993535-karsus/" {
		t.Errorf("Author = %+v, want Karsus", detail.Author)
	}
	if detail.Views != 33402 {
		t.Errorf("Views = %d, want 33402", detail.Views)
	}
	if detail.Downloads != 6049 {
		t.Errorf("Downloads = %d, want 6049", detail.Downloads)
	}
	if detail.DateModified != "2026-09-13T20:01:52+0200" {
		t.Errorf("DateModified = %q, want the real ISO 8601 timestamp", detail.DateModified)
	}
	if len(detail.Screenshots) != 2 {
		t.Fatalf("got %d screenshots, want 2", len(detail.Screenshots))
	}
	if detail.Screenshots[0].URL == "" || detail.Screenshots[0].URL == detail.Screenshots[0].ThumbnailURL {
		t.Errorf("Screenshots[0] = %+v, want distinct full/thumbnail URLs", detail.Screenshots[0])
	}
}

func TestParseFileDetailMissingReturnsFalse(t *testing.T) {
	doc := parseFixture(t, `<html><body>no ld+json here at all</body></html>`)
	if _, ok := parseFileDetail(doc); ok {
		t.Error("expected no WebApplication block to be found")
	}
}

func TestParseFileDetailIgnoresNonWebApplicationBlocks(t *testing.T) {
	doc := parseFixture(t, `<html><head>
<script type='application/ld+json'>{"@type": "WebSite", "name": "LoversLab"}</script>
<script type='application/ld+json'>{"@type": "Organization", "name": "LoversLab"}</script>
</head><body></body></html>`)
	if _, ok := parseFileDetail(doc); ok {
		t.Error("expected no WebApplication block among only WebSite/Organization ones")
	}
}
