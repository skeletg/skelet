package linkgraph

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Thresholds Google's own guidance and every crawler settle around - short enough to render, long enough to say something
const (
	TitleMin   = 15
	TitleMax   = 60
	MetaMin    = 50
	MetaMax    = 160
	ThinWords  = 120
	DeepClicks = 4
)

type Issue string

const (
	IssueBroken         Issue = "BROKEN"
	IssueRedirect       Issue = "REDIRECT"
	IssueTitleMissing   Issue = "TITLE_MISSING"
	IssueTitleLong      Issue = "TITLE_LONG"
	IssueTitleShort     Issue = "TITLE_SHORT"
	IssueTitleDuplicate Issue = "TITLE_DUPLICATE"
	IssueMetaMissing    Issue = "META_MISSING"
	IssueMetaLong       Issue = "META_LONG"
	IssueMetaShort      Issue = "META_SHORT"
	IssueMetaDuplicate  Issue = "META_DUPLICATE"
	IssueH1Missing      Issue = "H1_MISSING"
	IssueH1Multiple     Issue = "H1_MULTIPLE"
	IssueHeadingSkipped Issue = "HEADING_SKIPPED"
	IssueNoindex        Issue = "NOINDEX"
	IssueNofollow       Issue = "NOFOLLOW"
	IssueCanonicalOther Issue = "CANONICAL_OTHER"
	IssueCanonicalNone  Issue = "CANONICAL_NONE"
	IssueOrphan         Issue = "ORPHAN"
	IssueDeep           Issue = "DEEP"
	IssueThin           Issue = "THIN"
	IssueNoLinksOut     Issue = "NO_LINKS_OUT"
)

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

type AuditPage struct {
	Path          string
	StatusCode    int
	Kind          string
	Title         string
	TitleLength   int
	MetaDesc      string
	MetaLength    int
	RobotsMeta    string
	Canonical     string
	RedirectTo    string
	Noindex       bool
	Nofollow      bool
	H1            string
	H1Count       int
	HeadingCounts []int // index 0 is h1
	Headings      []Heading
	WordCount     int
	InternalLinks int
	ExternalLinks int
	InLinks       int
	Depth         int
	EntityType    string
	EntityID      uuid.UUID
	FetchMS       int
	ByteSize      int
	Issues        []Issue
}

type Audit struct {
	Pages       []AuditPage
	IssueCounts map[Issue]int
	Crawl       CrawlInfo
}

func (s *Service) Audit(ctx context.Context, channelID uuid.UUID, crawlID *uuid.UUID) (Audit, error) {
	crawl, err := s.pickCrawl(ctx, channelID, crawlID)
	if err != nil {
		return Audit{}, err
	}
	rows, err := s.queries.SeoCrawlAudit(ctx, crawl.ID)
	if err != nil {
		return Audit{}, err
	}

	pages := make([]AuditPage, 0, len(rows))
	titleSeen := map[string]int{}
	metaSeen := map[string]int{}
	for _, r := range rows {
		p := AuditPage{
			Path: r.Path, StatusCode: int(r.StatusCode), Kind: r.Kind,
			Title: r.Title, TitleLength: len([]rune(r.Title)),
			MetaDesc: r.MetaDescription, MetaLength: len([]rune(r.MetaDescription)),
			RobotsMeta: r.RobotsMeta, Canonical: r.Canonical, RedirectTo: r.RedirectTo,
			Noindex: r.Noindex, Nofollow: r.Nofollow, WordCount: int(r.WordCount),
			InternalLinks: int(r.InternalLinks), ExternalLinks: int(r.ExternalLinks),
			InLinks: int(r.InLinks), Depth: int(r.Depth),
			FetchMS: int(r.FetchMs), ByteSize: int(r.ByteSize),
		}
		if r.EntityType.Valid {
			p.EntityType = r.EntityType.String
		}
		if r.EntityID.Valid {
			p.EntityID = r.EntityID.UUID
		}
		var outline []Heading
		if len(r.Headings) > 0 {
			_ = json.Unmarshal(r.Headings, &outline)
		}
		p.Headings = outline
		p.HeadingCounts = make([]int, 6)
		for _, h := range outline {
			if h.Level >= 1 && h.Level <= 6 {
				p.HeadingCounts[h.Level-1]++
			}
			if h.Level == 1 {
				p.H1Count++
				if p.H1 == "" {
					p.H1 = h.Text
				}
			}
		}
		if r.Title != "" {
			titleSeen[strings.ToLower(r.Title)]++
		}
		if r.MetaDescription != "" {
			metaSeen[strings.ToLower(r.MetaDescription)]++
		}
		pages = append(pages, p)
	}

	counts := map[Issue]int{}
	for i := range pages {
		pages[i].Issues = flagIssues(&pages[i], titleSeen, metaSeen)
		for _, issue := range pages[i].Issues {
			counts[issue]++
		}
	}
	sort.SliceStable(pages, func(a, b int) bool {
		if len(pages[a].Issues) != len(pages[b].Issues) {
			return len(pages[a].Issues) > len(pages[b].Issues)
		}
		return pages[a].Path < pages[b].Path
	})
	return Audit{Pages: pages, IssueCounts: counts, Crawl: toCrawlInfo(crawl)}, nil
}

// A broken or redirecting page gets no content findings - it has no content, and listing ten of them per url buries
// the one thing that matters
func flagIssues(p *AuditPage, titleSeen, metaSeen map[string]int) []Issue {
	out := []Issue{}
	switch {
	case p.StatusCode >= 400 || p.StatusCode == 0:
		return append(out, IssueBroken)
	case p.StatusCode >= 300:
		return append(out, IssueRedirect)
	}

	switch {
	case p.TitleLength == 0:
		out = append(out, IssueTitleMissing)
	case p.TitleLength > TitleMax:
		out = append(out, IssueTitleLong)
	case p.TitleLength < TitleMin:
		out = append(out, IssueTitleShort)
	}
	if p.Title != "" && titleSeen[strings.ToLower(p.Title)] > 1 {
		out = append(out, IssueTitleDuplicate)
	}

	switch {
	case p.MetaLength == 0:
		out = append(out, IssueMetaMissing)
	case p.MetaLength > MetaMax:
		out = append(out, IssueMetaLong)
	case p.MetaLength < MetaMin:
		out = append(out, IssueMetaShort)
	}
	if p.MetaDesc != "" && metaSeen[strings.ToLower(p.MetaDesc)] > 1 {
		out = append(out, IssueMetaDuplicate)
	}

	switch {
	case p.H1Count == 0:
		out = append(out, IssueH1Missing)
	case p.H1Count > 1:
		out = append(out, IssueH1Multiple)
	}
	if skippedHeadingLevel(p.Headings) {
		out = append(out, IssueHeadingSkipped)
	}

	if p.Noindex {
		out = append(out, IssueNoindex)
	}
	if p.Nofollow {
		out = append(out, IssueNofollow)
	}
	switch {
	case p.Canonical == "":
		out = append(out, IssueCanonicalNone)
	case !canonicalPointsHere(p.Canonical, p.Path):
		out = append(out, IssueCanonicalOther)
	}

	if p.InLinks == 0 && p.Path != "/" {
		out = append(out, IssueOrphan)
	}
	if p.Depth >= DeepClicks {
		out = append(out, IssueDeep)
	}
	if p.WordCount < ThinWords {
		out = append(out, IssueThin)
	}
	if p.InternalLinks == 0 {
		out = append(out, IssueNoLinksOut)
	}
	return out
}

// An h3 directly under an h1 breaks the outline that assistive tech and extraction both read
func skippedHeadingLevel(headings []Heading) bool {
	previous := 0
	for _, h := range headings {
		if previous > 0 && h.Level > previous+1 {
			return true
		}
		if h.Level > 0 {
			previous = h.Level
		}
	}
	return false
}

// The tag is usually absolute while the crawl works in paths, so compare the tail rather than the whole string
func canonicalPointsHere(canonical, path string) bool {
	canonical = strings.TrimSuffix(canonical, "/")
	if idx := strings.Index(canonical, "://"); idx >= 0 {
		if slash := strings.Index(canonical[idx+3:], "/"); slash >= 0 {
			canonical = canonical[idx+3+slash:]
		} else {
			canonical = "/"
		}
	}
	if canonical == "" {
		canonical = "/"
	}
	return canonical == strings.TrimSuffix(path, "/") || (canonical == "/" && path == "/")
}
