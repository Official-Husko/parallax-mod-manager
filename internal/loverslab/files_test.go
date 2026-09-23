package loverslab

import "testing"

// downloadDialogFixture mirrors the real, authenticated "Download your
// files" dialog markup, confirmed live against
// https://www.loverslab.com/files/file/8719-stellaris-lustful-void/?do=download.
const downloadDialogFixture = `<li class='ipsDataItem'>
  <div class='ipsDataItem_main'>
    <h4 class='ipsDataItem_title ipsContained_container'>
      <span class='ipsType_break ipsContained'>LV Lewd Rooms.zip</span>
    </h4>
    <p class='ipsType_reset ipsDataItem_meta'>
      2.43 MB
      <span class='ipsType_neutral'> / <time datetime='2020-12-08T22:20:39Z' title='12/08/20 11:20  PM' data-short='5 yr'>December 8, 2020</time></span>
    </p>
  </div>
  <div class='ipsDataItem_generic ipsDataItem_size4 ipsType_right'>
    <span class="ipsHide" data-role="downloadCounterContainer">Download begins in <span data-role="downloadCounter"></span> seconds</span>
    <a href='https://www.loverslab.com/files/file/50753-tether/?do=download&amp;r=2173155&amp;confirm=1&amp;t=1&amp;csrfKey=8a3c356e2c467a183327ac0338c72895' class='ipsButton ipsButton_primary ipsButton_small' data-action="download">Download</a>
  </div>
</li>
<li class='ipsDataItem'>
  <div class='ipsDataItem_main'>
    <h4 class='ipsDataItem_title ipsContained_container'>
      <span class='ipsType_break ipsContained'>TETHER - Update.zip</span>
    </h4>
  </div>
  <div class='ipsDataItem_generic ipsDataItem_size4 ipsType_right'>
    <a href='https://www.loverslab.com/files/file/50753-tether/?do=download&amp;r=2173156&amp;confirm=1&amp;t=1&amp;csrfKey=8a3c356e2c467a183327ac0338c72895' class='ipsButton ipsButton_primary ipsButton_small' data-action="download">Download</a>
  </div>
</li>`

func TestParseDownloadDialogPairsEachVersionWithItsOwnLink(t *testing.T) {
	downloads := parseDownloadDialog([]byte(downloadDialogFixture))
	if len(downloads) != 2 {
		t.Fatalf("expected 2 downloads, got %d: %+v", len(downloads), downloads)
	}
	if downloads[0].Name != "LV Lewd Rooms.zip" {
		t.Errorf("first Name = %q", downloads[0].Name)
	}
	if downloads[0].URL != "https://www.loverslab.com/files/file/50753-tether/?do=download&r=2173155&confirm=1&t=1&csrfKey=8a3c356e2c467a183327ac0338c72895" {
		t.Errorf("first URL = %q, want the &amp;-decoded r=2173155 link", downloads[0].URL)
	}
	if downloads[0].Size != "2.43 MB" {
		t.Errorf("first Size = %q, want %q", downloads[0].Size, "2.43 MB")
	}
	if downloads[0].Posted != "December 8, 2020" {
		t.Errorf("first Posted = %q, want %q", downloads[0].Posted, "December 8, 2020")
	}
	if downloads[1].Name != "TETHER - Update.zip" || downloads[1].URL == downloads[0].URL {
		t.Errorf("second download not paired with its own distinct link: %+v", downloads[1])
	}
	if downloads[1].Size != "" || downloads[1].Posted != "" {
		t.Errorf("second download has no meta paragraph at all, expected empty Size/Posted: %+v", downloads[1])
	}
}

func TestParseDownloadDialogNoMatchesReturnsNil(t *testing.T) {
	if got := parseDownloadDialog([]byte("<html><body>not a download dialog at all</body></html>")); got != nil {
		t.Errorf("expected nil for a page with no download entries, got %+v", got)
	}
}

func TestFilenameFromContentDispositionDecodesURLEncoding(t *testing.T) {
	// docs/downloads.md: the filename is URL-encoded inside the quoted parameter,
	// which is non-standard for Content-Disposition but is what the site sends.
	got := filenameFromContentDisposition(`attachment; filename="TETHER%20-%20Hand-Holding%20for%20Followers.zip"`)
	want := "TETHER - Hand-Holding for Followers.zip"
	if got != want {
		t.Errorf("filenameFromContentDisposition = %q, want %q", got, want)
	}
}

func TestFilenameFromContentDispositionEmptyHeader(t *testing.T) {
	if got := filenameFromContentDisposition(""); got != "" {
		t.Errorf("expected empty string for an empty header, got %q", got)
	}
}

func TestFilenameFromContentDispositionMalformedHeader(t *testing.T) {
	if got := filenameFromContentDisposition("not a valid header;;;"); got != "" {
		t.Errorf("expected empty string for a malformed header, got %q", got)
	}
}
