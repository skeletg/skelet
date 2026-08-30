package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/linkgraph"
	"github.com/skeletg/skelet/internal/sitecrawl"
)

func (r *mutationResolver) StartSiteCrawl(ctx context.Context, input model.StartSiteCrawlInput) (*model.SiteCrawl, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.manage"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", nil)
	}
	in := sitecrawl.StartInput{ChannelID: cid, RespectRobots: true}
	if input.BaseURL != nil {
		in.BaseURL = *input.BaseURL
	}
	if input.MaxPages != nil {
		in.MaxPages = int(*input.MaxPages)
	}
	if input.MaxDepth != nil {
		in.MaxDepth = int(*input.MaxDepth)
	}
	if input.RespectRobots != nil {
		in.RespectRobots = *input.RespectRobots
	}
	if claims, ok := auth.AdminFromContext(ctx); ok {
		adminID := claims.AdminID
		in.AdminID = &adminID
	}
	row, err := r.SiteCrawlSvc.Start(ctx, in)
	if err != nil {
		if errors.Is(err, sitecrawl.ErrAlreadyRunning) {
			return toSiteCrawl(row), nil
		}
		return nil, err
	}
	return toSiteCrawl(row), nil
}

func (r *mutationResolver) CancelSiteCrawl(ctx context.Context, channelID string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return false, errInvalidID("channelId", nil)
	}
	return r.SiteCrawlSvc.Cancel(cid), nil
}

func (r *queryResolver) LinkGraph(ctx context.Context, channelID string, crawlID *string, maxNodes *int32) (*model.LinkGraph, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", nil)
	}
	var pinned *uuid.UUID
	if crawlID != nil && *crawlID != "" {
		parsed, err := uuid.Parse(*crawlID)
		if err != nil {
			return nil, errInvalidID("crawlId", nil)
		}
		pinned = &parsed
	}
	limit := 0
	if maxNodes != nil {
		limit = int(*maxNodes)
	}

	g, err := r.LinkGraphSvc.Build(ctx, cid, pinned, limit)
	if err != nil {
		if errors.Is(err, linkgraph.ErrNoCrawl) {
			return nil, nil
		}
		return nil, err
	}

	nodes := make([]*model.LinkNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		node := &model.LinkNode{
			Key:        n.Key,
			Type:       nodeType(n.Type),
			Label:      n.Label,
			Path:       n.Path,
			StatusCode: int32(n.StatusCode),
			Kind:       crawlKind(n.Kind),
			State:      linkStates[n.State],
			Noindex:    n.Noindex,
			IsHomepage: n.IsHomepage,
			InLinks:    int32(n.InLinks),
			OutLinks:   int32(n.OutLinks),
			Depth:      int32(n.Depth),
		}
		if n.EntityID != (uuid.UUID{}) {
			id := n.EntityID.String()
			node.EntityID = &id
		}
		nodes = append(nodes, node)
	}
	edges := make([]*model.LinkEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, &model.LinkEdge{From: e.From, To: e.To, Nofollow: e.Nofollow, Boilerplate: e.Boilerplate})
	}

	return &model.LinkGraph{
		Nodes: nodes,
		Edges: edges,
		Stats: toLinkStats(g.Stats),
		Crawl: crawlFromInfo(g.Crawl),
	}, nil
}

func (r *queryResolver) SiteCrawls(ctx context.Context, channelID string, limit *int32) ([]*model.SiteCrawl, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", nil)
	}
	var want int32
	if limit != nil {
		want = *limit
	}
	rows, err := r.SiteCrawlSvc.List(ctx, cid, want)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SiteCrawl, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSiteCrawl(row))
	}
	return out, nil
}

func (r *queryResolver) CrawlAudit(ctx context.Context, channelID string, crawlID *string, issue *model.SeoIssue, kind *model.CrawlPageKind, search *string, limit *int32, offset *int32) (*model.CrawlAudit, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", nil)
	}
	var pinned *uuid.UUID
	if crawlID != nil && *crawlID != "" {
		parsed, err := uuid.Parse(*crawlID)
		if err != nil {
			return nil, errInvalidID("crawlId", nil)
		}
		pinned = &parsed
	}

	audit, err := r.LinkGraphSvc.Audit(ctx, cid, pinned)
	if err != nil {
		if errors.Is(err, linkgraph.ErrNoCrawl) {
			return nil, nil
		}
		return nil, err
	}

	needle := ""
	if search != nil {
		needle = strings.ToLower(strings.TrimSpace(*search))
	}
	matched := make([]linkgraph.AuditPage, 0, len(audit.Pages))
	for _, p := range audit.Pages {
		if issue != nil && !hasIssue(p.Issues, *issue) {
			continue
		}
		if kind != nil && crawlKind(p.Kind) != *kind {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(p.Path), needle) && !strings.Contains(strings.ToLower(p.Title), needle) {
			continue
		}
		matched = append(matched, p)
	}

	page := paginate(matched, limit, offset)
	rows := make([]*model.CrawlAuditPage, 0, len(page))
	for _, p := range page {
		rows = append(rows, toAuditPage(p))
	}
	counts := make([]*model.SeoIssueCount, 0, len(audit.IssueCounts))
	for i, c := range audit.IssueCounts {
		counts = append(counts, &model.SeoIssueCount{Issue: model.SeoIssue(i), Count: int32(c)})
	}
	return &model.CrawlAudit{
		TotalPages:  int32(len(matched)),
		Pages:       rows,
		IssueCounts: counts,
		Crawl:       crawlFromInfo(audit.Crawl),
	}, nil
}

func (r *queryResolver) CrawlDiff(ctx context.Context, channelID string, baseID *string, currentID *string, regressionsOnly *bool, limit *int32) (*model.CrawlDiff, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "urlrewrite.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", nil)
	}
	base, err := optCrawlID(baseID, "baseId")
	if err != nil {
		return nil, err
	}
	current, err := optCrawlID(currentID, "currentId")
	if err != nil {
		return nil, err
	}

	diff, err := r.LinkGraphSvc.Diff(ctx, cid, base, current)
	if err != nil {
		if errors.Is(err, linkgraph.ErrNoCrawl) || errors.Is(err, linkgraph.ErrNoBaseline) {
			return nil, nil
		}
		return nil, err
	}
	return toCrawlDiff(diff, regressionsOnly != nil && *regressionsOnly, limit), nil
}
