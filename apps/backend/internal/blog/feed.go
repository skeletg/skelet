package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type FeedKind string // selects the output format

const (
	FeedKindRSS  FeedKind = "rss"
	FeedKindAtom FeedKind = "atom"
)

type FeedOptions struct { // BaseURL is the absolute storefront URL (e.g. https://example.com) canonical post URLs are built from, and ChannelID scopes the feed to one store - nil yields the cross-store global feed, for admin-only or single-store deployments
	BaseURL     string
	Locale      string
	ChannelID   *uuid.UUID
	Title       string
	Description string
	Limit       int32
}

func (s *Service) RenderFeed(ctx context.Context, kind FeedKind, opts FeedOptions) ([]byte, string, error) { // a feed document of the latest published posts
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	posts, err := s.ListPosts(ctx, PostListFilter{
		OnlyPublished: true,
		ChannelID:     opts.ChannelID,
		Limit:         limit,
	}, opts.Locale)
	if err != nil {
		return nil, "", err
	}
	switch kind {
	case FeedKindAtom:
		return renderAtom(posts, opts)
	case FeedKindRSS, "":
		return renderRSS(posts, opts)
	}
	return nil, "", fmt.Errorf("unknown feed kind: %q", kind)
}

type rssEnvelope struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Language      string    `xml:"language,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate,omitempty"`
	Description string `xml:"description,omitempty"`
}

func renderRSS(posts []Post, opts FeedOptions) ([]byte, string, error) {
	channel := rssChannel{
		Title:       fallback(opts.Title, "Blog"),
		Link:        opts.BaseURL,
		Description: opts.Description,
		Language:    opts.Locale,
	}
	if len(posts) > 0 && posts[0].PublishedAt != nil {
		channel.LastBuildDate = posts[0].PublishedAt.Format(time.RFC1123Z)
	}
	for _, p := range posts {
		title := postDisplayTitle(p)
		channel.Items = append(channel.Items, rssItem{
			Title:       title,
			Link:        joinURL(opts.BaseURL, "blog/"+p.Slug),
			GUID:        p.ID.String(),
			PubDate:     pubDateRFC1123(p.PublishedAt),
			Description: postPlainExcerpt(p),
		})
	}
	env := rssEnvelope{Version: "2.0", Channel: channel}
	return marshalXML(env, "application/rss+xml; charset=utf-8")
}

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	Link    atomLink    `xml:"link"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	Link    atomLink `xml:"link"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary,omitempty"`
}

func renderAtom(posts []Post, opts FeedOptions) ([]byte, string, error) {
	updated := time.Now().Format(time.RFC3339)
	if len(posts) > 0 && posts[0].PublishedAt != nil {
		updated = posts[0].PublishedAt.Format(time.RFC3339)
	}
	feed := atomFeed{
		Title:   fallback(opts.Title, "Blog"),
		Link:    atomLink{Href: opts.BaseURL, Rel: "alternate"},
		ID:      fallback(opts.BaseURL, "urn:uuid:"+uuid.NewString()),
		Updated: updated,
	}
	for _, p := range posts {
		title := postDisplayTitle(p)
		entryUpdated := time.Now().Format(time.RFC3339)
		if p.PublishedAt != nil {
			entryUpdated = p.PublishedAt.Format(time.RFC3339)
		}
		feed.Entries = append(feed.Entries, atomEntry{
			Title:   title,
			Link:    atomLink{Href: joinURL(opts.BaseURL, "blog/"+p.Slug), Rel: "alternate"},
			ID:      "urn:uuid:" + p.ID.String(),
			Updated: entryUpdated,
			Summary: postPlainExcerpt(p),
		})
	}
	return marshalXML(feed, "application/atom+xml; charset=utf-8")
}

func marshalXML(v any, contentType string) ([]byte, string, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, "", err
	}
	if err := enc.Flush(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), contentType, nil
}

func postDisplayTitle(p Post) string {
	if p.Content != nil && p.Content.Title != "" {
		return p.Content.Title
	}
	return p.Slug
}

func postPlainExcerpt(p Post) string { // a short plain-text summary for feed readers - Lexical JSON isn't unwrapped here, since readers tolerate empty summaries and rendering Lexical to plain text is out of scope for V1
	if p.Content == nil {
		return ""
	}
	if p.Content.MetaDescription != nil && *p.Content.MetaDescription != "" {
		return *p.Content.MetaDescription
	}
	if len(p.Content.Excerpt) > 0 {
		if t := lexicalPlainText(p.Content.Excerpt); t != "" { // best-effort - pull text leaves out of a Lexical JSON tree
			return t
		}
	}
	return ""
}

func lexicalPlainText(raw json.RawMessage) string { // concatenates the string values under "text" keys, at most ~280 chars, ignoring errors
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	var sb strings.Builder
	walkLexical(v, &sb)
	out := strings.TrimSpace(sb.String())
	if len(out) > 280 {
		out = out[:280] + "…"
	}
	return out
}

func walkLexical(v any, sb *strings.Builder) {
	switch n := v.(type) {
	case map[string]any:
		if t, ok := n["text"].(string); ok {
			sb.WriteString(t)
			sb.WriteByte(' ')
		}
		if children, ok := n["children"].([]any); ok {
			for _, c := range children {
				walkLexical(c, sb)
			}
		}
		if root, ok := n["root"]; ok {
			walkLexical(root, sb)
		}
	case []any:
		for _, c := range n {
			walkLexical(c, sb)
		}
	}
}

func pubDateRFC1123(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC1123Z)
}

func joinURL(base, path string) string {
	if base == "" {
		return path
	}
	b := strings.TrimRight(base, "/")
	p := strings.TrimLeft(path, "/")
	return b + "/" + p
}

func fallback(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
