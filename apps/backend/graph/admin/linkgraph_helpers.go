package admin // gqlgen relocates non-resolver decls out of *.resolvers.go on every run, so they live here

import (
	"strings"

	"github.com/skeletg/skelet/graph/admin/model"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/linkgraph"
)

var linkNodeTypes = map[string]model.LinkNodeType{
	"product":       model.LinkNodeTypeProduct,
	"category":      model.LinkNodeTypeCategory,
	"collection":    model.LinkNodeTypeCollection,
	"cms_page":      model.LinkNodeTypeCmsPage,
	"blog_post":     model.LinkNodeTypeBlogPost,
	"blog_category": model.LinkNodeTypeBlogCategory,
	"blog_tag":      model.LinkNodeTypeBlogTag,
	"blog_author":   model.LinkNodeTypeBlogAuthor,
}

func nodeType(domain string) model.LinkNodeType {
	if mapped, ok := linkNodeTypes[domain]; ok {
		return mapped
	}
	return model.LinkNodeTypeOther
}

func toSiteCrawl(row db.SeoCrawl) *model.SiteCrawl {
	out := &model.SiteCrawl{
		ID:            row.ID.String(),
		BaseURL:       row.BaseUrl,
		Status:        model.SiteCrawlStatus(strings.ToUpper(row.Status)),
		MaxPages:      row.MaxPages,
		MaxDepth:      row.MaxDepth,
		RespectRobots: row.RespectRobots,
		PagesCrawled:  row.PagesCrawled,
		LinksFound:    row.LinksFound,
		HitBudget:     row.HitBudget,
		Error:         row.Error,
		StartedAt:     row.StartedAt,
	}
	if row.FinishedAt.Valid {
		out.FinishedAt = &row.FinishedAt.Time
	}
	return out
}

var linkStates = map[linkgraph.State]model.LinkNodeState{
	linkgraph.StateCrawled:    model.LinkNodeStateCrawled,
	linkgraph.StateDiscovered: model.LinkNodeStateDiscovered,
	linkgraph.StateUnseen:     model.LinkNodeStateUnseen,
}

func toLinkStats(s linkgraph.Stats) *model.LinkGraphStats {
	return &model.LinkGraphStats{
		TotalNodes:    int32(s.TotalNodes),
		TotalEdges:    int32(s.TotalEdges),
		Crawled:       int32(s.Crawled),
		Discovered:    int32(s.Discovered),
		Unseen:        int32(s.Unseen),
		Orphans:       int32(s.Orphans),
		Noindexed:     int32(s.Noindexed),
		Broken:        int32(s.Broken),
		Redirected:    int32(s.Redirected),
		Boilerplate:   int32(s.Boilerplate),
		Contextual:    int32(s.Contextual),
		MaxDepth:      int32(s.MaxDepth),
		ReturnedNodes: int32(s.ReturnedNode),
		Dropped:       int32(s.Dropped),
		Complete:      s.Complete,
	}
}

func crawlFromInfo(info linkgraph.CrawlInfo) *model.SiteCrawl {
	return &model.SiteCrawl{
		ID:            info.ID.String(),
		BaseURL:       info.BaseURL,
		Status:        model.SiteCrawlStatus(strings.ToUpper(info.Status)),
		MaxPages:      int32(info.MaxPages),
		MaxDepth:      int32(info.MaxDepth),
		RespectRobots: info.RespectRobots,
		PagesCrawled:  int32(info.PagesCrawled),
		LinksFound:    int32(info.LinksFound),
		HitBudget:     info.HitBudget,
		Error:         info.Error,
		StartedAt:     info.StartedAt,
		FinishedAt:    info.FinishedAt,
	}
}
