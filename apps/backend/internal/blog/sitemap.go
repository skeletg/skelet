package blog

import (
	"context"
	"encoding/xml"
	"time"

	"github.com/google/uuid"
)

// ChannelID set filters posts by that store's listings, otherwise all globally-published posts are emitted
type SitemapOptions struct {
	BaseURL   string
	ChannelID *uuid.UUID
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
}

// Published posts, categories and tags, each <loc> built off opts.BaseURL
func (s *Service) RenderSitemap(ctx context.Context, opts SitemapOptions) ([]byte, string, error) {
	var urls []sitemapURL

	posts, err := s.repo.ListPosts(ctx, PostListFilter{
		OnlyPublished: true,
		ChannelID:     opts.ChannelID,
		Limit:         5000,
	})
	if err != nil {
		return nil, "", err
	}
	for _, p := range posts {
		// Phase 1 respects the global flag only - per-store seo_noindex needs the listings loaded onto each post, and the store filter already excluded unlisted ones
		if p.SEONoindex {
			continue
		}
		urls = append(urls, sitemapURL{
			Loc:        joinURL(opts.BaseURL, "blog/"+p.Slug),
			LastMod:    formatSitemapTime(p.UpdatedAt),
			ChangeFreq: "weekly",
		})
	}

	cats, err := s.repo.ListAllCategories(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, c := range cats {
		if !c.IsPublished || c.SEONoindex {
			continue
		}
		urls = append(urls, sitemapURL{
			Loc:        joinURL(opts.BaseURL, c.URLPath),
			LastMod:    formatSitemapTime(c.UpdatedAt),
			ChangeFreq: "weekly",
		})
	}

	tags, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, t := range tags {
		if !t.IsPublished {
			continue
		}
		urls = append(urls, sitemapURL{
			Loc:        joinURL(opts.BaseURL, "blog/tag/"+t.Slug),
			LastMod:    formatSitemapTime(t.UpdatedAt),
			ChangeFreq: "weekly",
		})
	}

	return marshalXML(sitemapURLSet{URLs: urls}, "application/xml; charset=utf-8")
}

func formatSitemapTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
