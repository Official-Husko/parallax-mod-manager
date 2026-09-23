package loverslab

import "testing"

// Markup quoted from docs/loverslab.md / the research project's comments-and-attachments.md,
// plus a real support topic's own reply-content wrapper (confirmed live 2026-09-23 against
// Stable Portraits' support topic: data-role='commentContent', not a distinctive class name -
// see docs/loverslab.md).
const topicPostsFixture = `<html><body>
<ul class='ipsPagination' data-pages='3' data-ipsPagination-perPage='25'></ul>
<article id="elComment_12345" class="cPost ipsComment ipsClearfix ipsComment_verified">
  <aside class="ipsComment_author cAuthorPane">
    <a href="/profile/1-someauthor/">SomeAuthor</a>
  </aside>
  <a href="/topic/269604-tether/?do=findComment&amp;comment=12345#findComment-12345">#1</a>
  <time datetime="2019-11-05T10:00:00Z">November 5, 2019</time>
  <div data-role='commentContent' class='ipsType_normal ipsType_richText ipsContained'>
    <p>Great mod! Here's a screenshot:</p>
    <a href="https://www.loverslab.com/uploads/monthly_2019_11/blackrockshooter.png.de550090eb7189bdfa94ff865459ebd6.png"
       class="ipsAttachLink ipsAttachLink_image">
      <img data-fileid="806008" src="https://www.loverslab.com/uploads/monthly_2019_11/blackrockshooter.png.de550090eb7189bdfa94ff865459ebd6.png" alt="blackrockshooter.png">
    </a>
  </div>
</article>
<article id="elComment_12346" class="cPost ipsComment ipsClearfix">
  <aside class="ipsComment_author cAuthorPane">
    <a href="/profile/91861-sexlabteam/">SexLabTeam</a>
  </aside>
  <a href="/topic/91861-sexlab-se/?do=findComment&amp;comment=12346#findComment-12346">#2</a>
  <time datetime="2019-11-06T10:00:00Z">November 6, 2019</time>
  <div data-role='commentContent' class='ipsType_normal ipsType_richText ipsContained'>
    <p>Here's the beta build.</p>
    <a class="ipsAttachLink" data-fileext="zip" data-fileid="1232475"
       href="https://www.loverslab.com/applications/core/interface/file/attachment.php?id=1232475&amp;key=2c290a90f6e731d8bb9c42825b7a795e"
       rel="norewrite">SexLabFrameworkSE_v163_BETA9.zip</a>
  </div>
</article>
</body></html>`

func TestParseTopicPosts(t *testing.T) {
	doc := parseFixture(t, topicPostsFixture)
	posts, totalPages := parseTopicPosts(doc)

	if totalPages != 3 {
		t.Errorf("totalPages = %d, want 3", totalPages)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}

	first := posts[0]
	if first.ID != "12345" {
		t.Errorf("first.ID = %q, want 12345", first.ID)
	}
	if first.Author != "SomeAuthor" || first.AuthorURL != "/profile/1-someauthor/" {
		t.Errorf("first author = %q/%q, want SomeAuthor/profile URL", first.Author, first.AuthorURL)
	}
	// This fixture's own author panel has no photo at all (common - not every
	// member sets one) - critically, this also confirms the first post's own
	// attachment image (a real <img>, just elsewhere in the article, outside
	// the author panel) is never mistaken for the author's avatar.
	if first.AuthorAvatarURL != "" {
		t.Errorf("first.AuthorAvatarURL = %q, want none (this fixture's author panel has no photo)", first.AuthorAvatarURL)
	}
	if first.Posted == "" {
		t.Error("expected a non-empty Posted string")
	}
	if first.URL == "" {
		t.Error("expected a deep-link URL (#findComment-)")
	}
	if first.Content != "Great mod! Here's a screenshot:" {
		t.Errorf("first.Content = %q, want the reply's own text with the attachment link excluded", first.Content)
	}
	if len(first.Attachments) != 1 {
		t.Fatalf("got %d attachments on the first post, want 1", len(first.Attachments))
	}
	img := first.Attachments[0]
	if !img.IsImage {
		t.Error("expected the first post's attachment to be detected as an image")
	}
	if img.Filename != "blackrockshooter.png" {
		t.Errorf("image filename = %q, want it recovered from <img alt>", img.Filename)
	}

	second := posts[1]
	if second.Content != "Here's the beta build." {
		t.Errorf("second.Content = %q, want the reply's own text with the attachment link excluded", second.Content)
	}
	if len(second.Attachments) != 1 {
		t.Fatalf("got %d attachments on the second post, want 1", len(second.Attachments))
	}
	zip := second.Attachments[0]
	if zip.IsImage {
		t.Error("expected the .zip attachment to not be detected as an image")
	}
	if zip.Filename != "SexLabFrameworkSE_v163_BETA9.zip" {
		t.Errorf("zip filename = %q, want SexLabFrameworkSE_v163_BETA9.zip", zip.Filename)
	}
	if zip.Extension != "zip" {
		t.Errorf("zip Extension = %q, want zip", zip.Extension)
	}
}

func TestParseTopicPostsExtractsTheAuthorAvatar(t *testing.T) {
	doc := parseFixture(t, `<html><body>
<article id="elComment_1" class="cPost ipsComment">
  <aside class="ipsComment_author cAuthorPane">
    <a href="/profile/1102521-lithia/">Lithia&lt;3</a>
    <ul class="cAuthorPane_info">
      <li data-role="photo" class="cAuthorPane_photo">
        <a href="/profile/1102521-lithia/" class="ipsUserPhoto ipsUserPhoto_large">
          <img src="https://static.loverslab.com/uploads/profiles/monthly_2023_06/pp7_2.thumb.jpg" alt="Lithia&lt;3">
        </a>
      </li>
    </ul>
  </aside>
</article>
</body></html>`)
	posts, _ := parseTopicPosts(doc)
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	if posts[0].AuthorAvatarURL != "https://static.loverslab.com/uploads/profiles/monthly_2023_06/pp7_2.thumb.jpg" {
		t.Errorf("AuthorAvatarURL = %q, want the real photo URL", posts[0].AuthorAvatarURL)
	}
}

// Confirmed live against a real topic (see docs/loverslab.md): the author
// panel's own group/post-count/custom-title, and this reply's own real
// "Author"/"Popular Post" badges, reaction count, and "(edited)" note.
func TestParseTopicPostsExtractsRealAuthorAndReplyMetadata(t *testing.T) {
	doc := parseFixture(t, `<html><body>
<article id="elComment_1" class="cPost ipsComment">
  <aside class="ipsComment_author cAuthorPane">
    <a href="/profile/1102521-lithia/">Lithia&lt;3</a>
    <ul class="cAuthorPane_info">
      <li data-role="group"><span>Members</span></li>
      <li data-role="stats">
        <a title="215 posts">215</a>
      </li>
      <li data-role="custom-field"><em class="ipsFieldRow_desc">Snuggle Butt Princess</em></li>
    </ul>
  </aside>
  <div class="ipsComment_badges">
    <strong class="ipsBadge ipsBadge_popular">Popular Post</strong>
    <strong class="ipsBadge ipsComment_authorBadge">Author</strong>
  </div>
  <span class="ipsResponsive_hidePhone">(edited)</span>
  <div class="ipsReact">
    <span data-role="reactCountText">119</span>
  </div>
</article>
</body></html>`)
	posts, _ := parseTopicPosts(doc)
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	p := posts[0]
	if p.AuthorGroup != "Members" {
		t.Errorf("AuthorGroup = %q, want Members", p.AuthorGroup)
	}
	if p.AuthorPostCount != 215 {
		t.Errorf("AuthorPostCount = %d, want 215", p.AuthorPostCount)
	}
	if p.AuthorTitle != "Snuggle Butt Princess" {
		t.Errorf("AuthorTitle = %q, want the real custom tagline", p.AuthorTitle)
	}
	if !p.IsPopular {
		t.Error("expected IsPopular true from the real Popular Post badge")
	}
	if !p.IsTopicAuthor {
		t.Error("expected IsTopicAuthor true from the real Author badge")
	}
	if !p.Edited {
		t.Error("expected Edited true from the real '(edited)' note")
	}
	if p.Reactions != 119 {
		t.Errorf("Reactions = %d, want 119", p.Reactions)
	}
}

func TestParseTopicPostsWithNoneOfTheOptionalMetadataStillParses(t *testing.T) {
	doc := parseFixture(t, `<html><body>
<article id="elComment_1" class="cPost ipsComment">
  <aside class="ipsComment_author cAuthorPane"><a href="/profile/1-x/">X</a></aside>
</article>
</body></html>`)
	posts, _ := parseTopicPosts(doc)
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	p := posts[0]
	if p.AuthorGroup != "" || p.AuthorPostCount != 0 || p.AuthorTitle != "" || p.IsPopular || p.IsTopicAuthor || p.Edited || p.Reactions != 0 {
		t.Errorf("expected every optional field at its zero value for a post with none of this markup, got %+v", p)
	}
	if p.ContentBlocks == nil {
		t.Error("ContentBlocks must never be nil, even with no commentContent body at all")
	}
}

// A reply that quotes an earlier one - the exact real markup confirmed live
// (<blockquote class="ipsQuote">) - must land as its own attributed
// ContentBlocks entry, not flattened into the reply's own plain Content.
func TestParseTopicPostsExtractsAQuoteInsideAReply(t *testing.T) {
	doc := parseFixture(t, `<html><body>
<article id="elComment_1" class="cPost ipsComment">
  <aside class="ipsComment_author cAuthorPane"><a href="/profile/1-x/">X</a></aside>
  <div data-role='commentContent' class='ipsType_richText'>
    <blockquote class="ipsQuote" data-ipsquote-username="darkspleen">
      <div class="ipsQuote_citation">said:</div>
      <div class="ipsQuote_contents"><p>We have arrived.</p></div>
    </blockquote>
    <p>Agreed!</p>
  </div>
</article>
</body></html>`)
	posts, _ := parseTopicPosts(doc)
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	blocks := posts[0].ContentBlocks
	if len(blocks) != 2 {
		t.Fatalf("got %d ContentBlocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].QuotedAuthor != "darkspleen" {
		t.Errorf("blocks[0].QuotedAuthor = %q, want darkspleen", blocks[0].QuotedAuthor)
	}
	if runsText(blocks[1].Runs) != "Agreed!" {
		t.Errorf("blocks[1] = %+v, want the reply's own new text", blocks[1])
	}
}

func TestParseTopicPostsNoPaginationDefaultsToOnePage(t *testing.T) {
	doc := parseFixture(t, `<html><body><article id="elComment_1" class="cPost ipsComment"></article></body></html>`)
	_, totalPages := parseTopicPosts(doc)
	if totalPages != 1 {
		t.Errorf("totalPages = %d, want 1", totalPages)
	}
}

func TestParseSupportTopicURL(t *testing.T) {
	doc := parseFixture(t, `<html><body>
		<a href="https://www.loverslab.com/topic/269604-tether-hand-holding-for-followers/"
		   title="Get support for this download" class="ipsButton ipsButton_normal ipsButton_fullWidth">Get Support</a>
	</body></html>`)
	got := parseSupportTopicURL(doc)
	want := "https://www.loverslab.com/topic/269604-tether-hand-holding-for-followers/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSupportTopicURLAbsentIsNotAnError(t *testing.T) {
	doc := parseFixture(t, `<html><body><p>This file has no support topic linked.</p></body></html>`)
	if got := parseSupportTopicURL(doc); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
