package blog

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func strPtr(s string) *string { return &s }

func at(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	return &t
}

func post(slug, title string, published *time.Time) Post {
	return Post{
		ID:          uuid.New(),
		Slug:        slug,
		Status:      PostStatusPublished,
		PublishedAt: published,
		Content:     &PostContent{Locale: "en", Title: title},
	}
}

func TestJoinURL(t *testing.T) { // every feed link is built through this, so a doubled or missing slash breaks every entry in the document
	for name, tc := range map[string]struct{ base, path, want string }{
		"plain":         {"https://x.invalid", "blog/a", "https://x.invalid/blog/a"},
		"trailing base": {"https://x.invalid/", "blog/a", "https://x.invalid/blog/a"},
		"leading path":  {"https://x.invalid", "/blog/a", "https://x.invalid/blog/a"},
		"both slashes":  {"https://x.invalid/", "/blog/a", "https://x.invalid/blog/a"},
		"many slashes":  {"https://x.invalid///", "blog/a", "https://x.invalid/blog/a"},
		"no base":       {"", "blog/a", "blog/a"},
		"subpath base":  {"https://x.invalid/shop", "blog/a", "https://x.invalid/shop/blog/a"},
	} {
		if got := joinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("%s: joinURL(%q, %q) = %q, want %q", name, tc.base, tc.path, got, tc.want)
		}
	}
}

func TestJoinURLNeverDoublesASeparatorAfterTheScheme(t *testing.T) { // "//" mid-path is a different URL to most routers, and it would 404 every post
	for _, base := range []string{"https://x.invalid", "https://x.invalid/", "https://x.invalid//", "https://x.invalid/shop/"} {
		for _, path := range []string{"blog/a", "/blog/a", "//blog/a"} {
			got := joinURL(base, path)
			if rest := strings.TrimPrefix(got, "https://"); strings.Contains(rest, "//") {
				t.Errorf("joinURL(%q, %q) = %q, which doubles a separator", base, path, got)
			}
		}
	}
}

func TestFallback(t *testing.T) {
	if got := fallback("", "Blog"); got != "Blog" {
		t.Errorf("fallback(\"\") = %q, want the default", got)
	}
	if got := fallback("Mine", "Blog"); got != "Mine" {
		t.Errorf("fallback = %q, want the supplied value", got)
	}
	if got := fallback(" ", "Blog"); got != " " {
		t.Errorf("fallback(\" \") = %q, want the whitespace kept - only empty falls back", got)
	}
}

func TestPostDisplayTitleFallsBackToTheSlug(t *testing.T) { // a feed entry with no title is unreadable in a reader, so the slug is better than nothing
	if got := postDisplayTitle(post("my-post", "Real Title", nil)); got != "Real Title" {
		t.Errorf("title = %q, want the content title", got)
	}
	untitled := post("my-post", "", nil)
	if got := postDisplayTitle(untitled); got != "my-post" {
		t.Errorf("title = %q, want the slug fallback", got)
	}
	noContent := Post{ID: uuid.New(), Slug: "bare"}
	if got := postDisplayTitle(noContent); got != "bare" {
		t.Errorf("title with no content row = %q, want the slug", got)
	}
}

func TestPostPlainExcerptPrefersTheMetaDescription(t *testing.T) { // the meta description is hand-written for exactly this purpose, so it beats scraping the body
	p := post("a", "T", nil)
	p.Content.MetaDescription = strPtr("A hand-written summary")
	p.Content.Excerpt = json.RawMessage(`{"root":{"children":[{"text":"scraped"}]}}`)

	if got := postPlainExcerpt(p); got != "A hand-written summary" {
		t.Errorf("excerpt = %q, want the meta description", got)
	}

	// an empty meta description is not a summary - fall through to the excerpt
	p.Content.MetaDescription = strPtr("")
	if got := postPlainExcerpt(p); got != "scraped" {
		t.Errorf("excerpt = %q, want the scraped Lexical text", got)
	}
}

func TestPostPlainExcerptIsEmptyWhenThereIsNothingToSay(t *testing.T) { // readers tolerate an empty summary, so this must not invent one or panic
	for name, p := range map[string]Post{
		"no content row": {ID: uuid.New(), Slug: "a"},
		"no excerpt":     post("a", "T", nil),
		"empty excerpt":  mutate(post("a", "T", nil), func(p *Post) { p.Content.Excerpt = json.RawMessage(`{}`) }),
		"malformed":      mutate(post("a", "T", nil), func(p *Post) { p.Content.Excerpt = json.RawMessage(`{not json`) }),
	} {
		if got := postPlainExcerpt(p); got != "" {
			t.Errorf("%s: excerpt = %q, want empty", name, got)
		}
	}
}

func mutate(p Post, fn func(*Post)) Post {
	fn(&p)
	return p
}

func TestLexicalPlainTextWalksTheWholeTree(t *testing.T) { // the excerpt is a Lexical document, so text leaves live at arbitrary depth under root/children
	raw := json.RawMessage(`{"root":{"children":[
		{"type":"paragraph","children":[{"text":"Hello"},{"text":"world"}]},
		{"type":"list","children":[{"children":[{"text":"one"}]},{"children":[{"text":"two"}]}]}
	]}}`)
	got := lexicalPlainText(raw)
	for _, want := range []string{"Hello", "world", "one", "two"} {
		if !strings.Contains(got, want) {
			t.Errorf("lexicalPlainText = %q, want it to contain %q", got, want)
		}
	}
}

func TestLexicalPlainTextCapsLength(t *testing.T) { // a feed summary is a preview, and an unbounded one bloats every entry in the document
	long := strings.Repeat("word ", 200)
	raw, err := json.Marshal(map[string]any{"root": map[string]any{"children": []any{map[string]any{"text": long}}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := lexicalPlainText(raw)
	if len([]byte(got)) > 280+len("…") {
		t.Errorf("lexicalPlainText produced %d bytes, want it capped near 280", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a truncated summary = %q, want it to end in an ellipsis", got[max(0, len(got)-10):])
	}
}

func TestLexicalPlainTextOnJunk(t *testing.T) { // a hand-edited or malformed excerpt must yield nothing rather than failing the whole feed
	for name, raw := range map[string]json.RawMessage{
		"malformed":  json.RawMessage(`{not json`),
		"empty":      json.RawMessage(``),
		"a string":   json.RawMessage(`"just text"`),
		"a number":   json.RawMessage(`42`),
		"null":       json.RawMessage(`null`),
		"empty tree": json.RawMessage(`{"root":{"children":[]}}`),
	} {
		if got := lexicalPlainText(raw); got != "" {
			t.Errorf("%s: lexicalPlainText = %q, want empty", name, got)
		}
	}
}

func TestPubDateRFC1123(t *testing.T) { // RSS pubDate is a fixed wire format - readers drop entries whose date they can't parse
	if got := pubDateRFC1123(nil); got != "" {
		t.Errorf("pubDateRFC1123(nil) = %q, want empty so the element is omitted", got)
	}
	got := pubDateRFC1123(at(2026, time.August, 19))
	if _, err := time.Parse(time.RFC1123Z, got); err != nil {
		t.Errorf("pubDateRFC1123 = %q, which does not parse as RFC1123Z: %v", got, err)
	}
}

func TestFormatSitemapTime(t *testing.T) { // a zero lastmod must be omitted rather than emitted as year 1, which crawlers treat as ancient
	if got := formatSitemapTime(time.Time{}); got != "" {
		t.Errorf("formatSitemapTime(zero) = %q, want empty", got)
	}
	got := formatSitemapTime(time.Date(2026, time.August, 19, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600)))
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("formatSitemapTime = %q, which does not parse as RFC3339: %v", got, err)
	}
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("formatSitemapTime = %q, want it normalized to UTC", got)
	}
}

func TestRenderRSSProducesParseableXML(t *testing.T) { // the document goes straight to feed readers, so it has to be well-formed and carry the declaration
	posts := []Post{post("first", "First Post", at(2026, time.August, 19)), post("second", "Second", at(2026, time.August, 18))}
	opts := FeedOptions{BaseURL: "https://shop.invalid", Title: "My Blog", Description: "News", Locale: "en"}

	body, contentType, err := renderRSS(posts, opts)
	if err != nil {
		t.Fatalf("renderRSS: %v", err)
	}
	if !strings.HasPrefix(string(body), xml.Header) {
		t.Errorf("body does not start with the XML declaration: %q", string(body[:40]))
	}
	if !strings.Contains(contentType, "application/rss+xml") {
		t.Errorf("content type = %q, want application/rss+xml", contentType)
	}

	var got rssEnvelope
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("the rendered feed does not parse: %v\n%s", err, body)
	}
	if got.Version != "2.0" {
		t.Errorf("rss version = %q, want 2.0", got.Version)
	}
	if got.Channel.Title != "My Blog" || got.Channel.Link != "https://shop.invalid" || got.Channel.Language != "en" {
		t.Errorf("channel = %+v, want the options carried through", got.Channel)
	}
	if len(got.Channel.Items) != 2 {
		t.Fatalf("%d items, want one per post", len(got.Channel.Items))
	}
	if got.Channel.Items[0].Title != "First Post" {
		t.Errorf("first item title = %q, want the post title", got.Channel.Items[0].Title)
	}
	if got.Channel.Items[0].Link != "https://shop.invalid/blog/first" {
		t.Errorf("first item link = %q, want the absolute post URL", got.Channel.Items[0].Link)
	}
	if got.Channel.Items[0].GUID == "" || got.Channel.Items[0].GUID == got.Channel.Items[1].GUID {
		t.Error("items share a GUID, so readers would collapse them into one entry")
	}
}

func TestRenderRSSTitleDefaultsAndEmptyFeed(t *testing.T) { // an unconfigured store still has to serve a valid document rather than an error
	body, _, err := renderRSS(nil, FeedOptions{})
	if err != nil {
		t.Fatalf("renderRSS on an empty feed: %v", err)
	}
	var got rssEnvelope
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("the empty feed does not parse: %v", err)
	}
	if got.Channel.Title != "Blog" {
		t.Errorf("title = %q, want the Blog default", got.Channel.Title)
	}
	if len(got.Channel.Items) != 0 {
		t.Errorf("%d items, want none", len(got.Channel.Items))
	}
	if got.Channel.LastBuildDate != "" {
		t.Errorf("lastBuildDate = %q, want it omitted with no posts", got.Channel.LastBuildDate)
	}
}

func TestRenderRSSEscapesAuthoredText(t *testing.T) { // a title with markup would break the document, and an unescaped one is an injection into every subscriber's reader
	p := post("x", `Tires & "Wheels" <script>alert(1)</script>`, at(2026, time.August, 19))
	p.Content.MetaDescription = strPtr(`5 > 3 & <b>bold</b>`)

	body, _, err := renderRSS([]Post{p}, FeedOptions{BaseURL: "https://shop.invalid"})
	if err != nil {
		t.Fatalf("renderRSS: %v", err)
	}
	if strings.Contains(string(body), "<script>") {
		t.Errorf("the raw script tag survived into the feed:\n%s", body)
	}
	var got rssEnvelope
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("a feed with authored markup does not parse: %v\n%s", err, body)
	}
	// escaping is only correct if it round-trips back to the original text
	if got.Channel.Items[0].Title != p.Content.Title {
		t.Errorf("title round-tripped to %q, want %q", got.Channel.Items[0].Title, p.Content.Title)
	}
	if got.Channel.Items[0].Description != *p.Content.MetaDescription {
		t.Errorf("description round-tripped to %q, want %q", got.Channel.Items[0].Description, *p.Content.MetaDescription)
	}
}

func TestRenderAtomProducesParseableXML(t *testing.T) { // Atom is the other half of the contract, and its ids are urn:uuid rather than bare
	posts := []Post{post("first", "First Post", at(2026, time.August, 19))}
	body, contentType, err := renderAtom(posts, FeedOptions{BaseURL: "https://shop.invalid", Title: "My Blog"})
	if err != nil {
		t.Fatalf("renderAtom: %v", err)
	}
	if !strings.Contains(contentType, "application/atom+xml") {
		t.Errorf("content type = %q, want application/atom+xml", contentType)
	}

	var got atomFeed
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("the rendered feed does not parse: %v\n%s", err, body)
	}
	if got.Title != "My Blog" {
		t.Errorf("title = %q, want My Blog", got.Title)
	}
	if got.ID != "https://shop.invalid" {
		t.Errorf("feed id = %q, want the base URL", got.ID)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("%d entries, want 1", len(got.Entries))
	}
	e := got.Entries[0]
	if !strings.HasPrefix(e.ID, "urn:uuid:") {
		t.Errorf("entry id = %q, want a urn:uuid", e.ID)
	}
	if e.Link.Href != "https://shop.invalid/blog/first" {
		t.Errorf("entry link = %q, want the absolute post URL", e.Link.Href)
	}
	if _, err := time.Parse(time.RFC3339, e.Updated); err != nil {
		t.Errorf("entry updated = %q, which is not RFC3339: %v", e.Updated, err)
	}
}

func TestRenderAtomDatesAreAlwaysPresentAndParseable(t *testing.T) { // Atom requires updated on the feed and every entry, so an unpublished post must still carry a valid timestamp
	posts := []Post{post("no-date", "Undated", nil)}
	body, _, err := renderAtom(posts, FeedOptions{BaseURL: "https://shop.invalid"})
	if err != nil {
		t.Fatalf("renderAtom: %v", err)
	}
	var got atomFeed
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, got.Updated); err != nil {
		t.Errorf("feed updated = %q, which is not RFC3339: %v", got.Updated, err)
	}
	if _, err := time.Parse(time.RFC3339, got.Entries[0].Updated); err != nil {
		t.Errorf("entry updated = %q, which is not RFC3339: %v", got.Entries[0].Updated, err)
	}
}

func TestRenderAtomFallsBackToAUrnWhenNoBaseURL(t *testing.T) { // Atom requires a feed id, so an unconfigured base URL cannot leave it blank
	body, _, err := renderAtom(nil, FeedOptions{})
	if err != nil {
		t.Fatalf("renderAtom: %v", err)
	}
	var got atomFeed
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID == "" {
		t.Error("feed id is empty, which makes the document invalid Atom")
	}
	if !strings.HasPrefix(got.ID, "urn:uuid:") {
		t.Errorf("feed id = %q, want a urn:uuid fallback", got.ID)
	}
}

func TestBuildCategoryTreeNests(t *testing.T) { // the blog nav is assembled from a flat query, same as the catalog's
	rootA, rootB := uuid.New(), uuid.New()
	child := uuid.New()
	all := []Category{
		{ID: rootA, Slug: "news"},
		{ID: child, ParentID: &rootA, Slug: "releases"},
		{ID: rootB, Slug: "guides"},
	}

	roots := buildCategoryTree(all, nil)
	if len(roots) != 2 {
		t.Fatalf("%d roots, want 2", len(roots))
	}
	if roots[0].Slug != "news" || len(roots[0].Children) != 1 || roots[0].Children[0].Slug != "releases" {
		t.Errorf("first root = %+v, want news with a releases child", roots[0])
	}
	if len(roots[1].Children) != 0 {
		t.Errorf("guides has %d children, want none", len(roots[1].Children))
	}
}

func TestBuildCategoryTreeDropsUnreachableBranches(t *testing.T) { // a category whose parent is gone must not be promoted to a top-level nav entry
	root, missing := uuid.New(), uuid.New()
	all := []Category{{ID: root, Slug: "news"}, {ID: uuid.New(), ParentID: &missing, Slug: "orphan"}}
	roots := buildCategoryTree(all, nil)
	if len(roots) != 1 || roots[0].Slug != "news" {
		t.Errorf("roots = %d entries, want only news - an orphan must not become a root", len(roots))
	}
}

func TestValidateSlug(t *testing.T) { // the slug is the post's public URL segment - lowercase, digits, and the three separators - _ . so a version-shaped slug like v1.2-release is legal
	for _, ok := range []string{"my-post", "post-2026", "a", "under_score", "v1.2-release"} {
		if err := validateSlug(ok); err != nil {
			t.Errorf("validateSlug(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "My-Post", "my post", "my/post", "pöst", "my:post", "my?post"} {
		if err := validateSlug(bad); err == nil {
			t.Errorf("validateSlug(%q) = nil, want it refused", bad)
		}
	}
}
