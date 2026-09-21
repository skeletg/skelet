package admin

import (
	"fmt"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/linkgraph"
)

func hasIssue(issues []linkgraph.Issue, want model.SeoIssue) bool {
	for _, i := range issues {
		if model.SeoIssue(i) == want {
			return true
		}
	}
	return false
}

func paginate(pages []linkgraph.AuditPage, limit, offset *int32) []linkgraph.AuditPage {
	take := 100
	if limit != nil && *limit > 0 {
		take = int(*limit)
	}
	if take > 1000 {
		take = 1000
	}
	skip := 0
	if offset != nil && *offset > 0 {
		skip = int(*offset)
	}
	if skip >= len(pages) {
		return nil
	}
	end := min(skip+take, len(pages))
	return pages[skip:end]
}

func toAuditPage(p linkgraph.AuditPage) *model.CrawlAuditPage {
	headings := make([]*model.CrawlHeading, 0, len(p.Headings))
	for _, h := range p.Headings {
		headings = append(headings, &model.CrawlHeading{Level: int32(h.Level), Text: h.Text})
	}
	counts := make([]int32, 0, len(p.HeadingCounts))
	for _, c := range p.HeadingCounts {
		counts = append(counts, int32(c))
	}
	issues := make([]model.SeoIssue, 0, len(p.Issues))
	for _, i := range p.Issues {
		issues = append(issues, model.SeoIssue(i))
	}
	out := &model.CrawlAuditPage{
		Path: p.Path, StatusCode: int32(p.StatusCode), Kind: crawlKind(p.Kind),
		Title: p.Title, TitleLength: int32(p.TitleLength),
		MetaDesc: p.MetaDesc, MetaLength: int32(p.MetaLength),
		RobotsMeta: p.RobotsMeta, Canonical: p.Canonical, RedirectTo: p.RedirectTo,
		Noindex: p.Noindex, Nofollow: p.Nofollow,
		H1: p.H1, H1Count: int32(p.H1Count), HeadingCounts: counts, Headings: headings,
		WordCount: int32(p.WordCount), InternalLinks: int32(p.InternalLinks),
		ExternalLinks: int32(p.ExternalLinks), InLinks: int32(p.InLinks), Depth: int32(p.Depth),
		FetchMs: int32(p.FetchMS), ByteSize: int32(p.ByteSize), Issues: issues,
	}
	if p.EntityType != "" {
		t := nodeType(p.EntityType)
		out.EntityType = &t
	}
	if p.EntityID != (uuid.UUID{}) {
		id := p.EntityID.String()
		out.EntityID = &id
	}
	return out
}

var crawlKinds = map[string]model.CrawlPageKind{
	"page":   model.CrawlPageKindPage,
	"filter": model.CrawlPageKindFilter,
	"param":  model.CrawlPageKindParam,
}

func crawlKind(domain string) model.CrawlPageKind {
	if mapped, ok := crawlKinds[domain]; ok {
		return mapped
	}
	return model.CrawlPageKindPage
}

func optCrawlID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(*raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return &parsed, nil
}

func toCrawlDiff(d linkgraph.Diff, regressionsOnly bool, limit *int32) *model.CrawlDiff {
	take := 300
	if limit != nil && *limit > 0 && *limit <= 2000 {
		take = int(*limit)
	}
	changes := make([]*model.CrawlChange, 0, take)
	for _, c := range d.Changes {
		if regressionsOnly && !c.Kind.IsRegression() {
			continue
		}
		if len(changes) >= take {
			break
		}
		changes = append(changes, &model.CrawlChange{
			Path: c.Path, Kind: model.CrawlChangeKind(c.Kind),
			Before: c.Before, After: c.After, IsRegression: c.Kind.IsRegression(),
		})
	}
	counts := make([]*model.CrawlChangeCount, 0, len(d.Counts))
	for kind, n := range d.Counts {
		counts = append(counts, &model.CrawlChangeCount{Kind: model.CrawlChangeKind(kind), Count: int32(n)})
	}
	return &model.CrawlDiff{
		Base: crawlFromInfo(d.Base), Current: crawlFromInfo(d.Current),
		Changes: changes, Counts: counts,
		Compared: int32(d.Compared), Regressions: int32(d.Regressions),
		CoverageComparable: d.CoverageComparable,
	}
}
