// Package linkgraph turns a finished site crawl into a map - real pages, the links the HTML actually carried, and
// what the crawl could not account for
package linkgraph

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/sitecrawl"
)

var ErrNoCrawl = errors.New("no completed crawl for this channel yet")

const (
	DefaultMaxNodes = 1200
	MinMaxNodes     = 50
	MaxMaxNodes     = 6000
	typeOther       = "other"
)

// A page is only an orphan when the crawl actually went looking - anything it never reached because the budget
// ran out is unknown, not unlinked, and saying otherwise turns a truncated crawl into a wall of false findings
type State string

const (
	StateCrawled    State = "crawled"
	StateDiscovered State = "discovered"
	StateUnseen     State = "unseen"
)

type Node struct {
	Key        string
	Type       string
	EntityID   uuid.UUID
	Label      string
	Path       string
	Kind       string
	State      State
	StatusCode int
	Noindex    bool
	IsHomepage bool
	InLinks    int
	OutLinks   int
	Depth      int
}

type Edge struct {
	From     string
	To       string
	Nofollow bool
	// The target is linked from most of the site, so this is header/footer chrome rather than a structural link
	Boilerplate bool
}

type Stats struct {
	TotalNodes   int
	TotalEdges   int
	Crawled      int
	Discovered   int
	Unseen       int
	Orphans      int
	Noindexed    int
	Broken       int
	Redirected   int
	Boilerplate  int
	Contextual   int
	MaxDepth     int
	ReturnedNode int
	Dropped      int
	// False when the page budget ran out with links still queued - every "nothing links here" claim is unsafe
	Complete bool
}

type CrawlInfo struct {
	ID            uuid.UUID
	BaseURL       string
	Status        string
	MaxPages      int
	MaxDepth      int
	RespectRobots bool
	PagesCrawled  int
	LinksFound    int
	HitBudget     bool
	Error         string
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type Graph struct {
	Nodes []Node
	Edges []Edge
	Stats Stats
	Crawl CrawlInfo
}

type Service struct {
	queries *db.Queries
}

func New(queries *db.Queries) *Service { return &Service{queries: queries} }

// Facet chains and parameterised urls are their own animals for SEO, and the crawl only stamps a kind on pages it
// fetched - these two never were
func classify(path string) string {
	base, query, hasQuery := strings.Cut(path, "?")
	for seg := range strings.SplitSeq(strings.Trim(base, "/"), "/") {
		if seg == "f" || seg == "s" {
			return "filter"
		}
	}
	if hasQuery && query != "" {
		return "param"
	}
	return "page"
}

func (s *Service) Build(ctx context.Context, channelID uuid.UUID, crawlID *uuid.UUID, maxNodes int) (Graph, error) {
	switch {
	case maxNodes <= 0:
		maxNodes = DefaultMaxNodes
	case maxNodes < MinMaxNodes:
		maxNodes = MinMaxNodes
	case maxNodes > MaxMaxNodes:
		maxNodes = MaxMaxNodes
	}

	crawl, err := s.pickCrawl(ctx, channelID, crawlID)
	if err != nil {
		return Graph{}, err
	}
	pages, err := s.queries.SeoCrawlPages(ctx, crawl.ID)
	if err != nil {
		return Graph{}, err
	}
	links, err := s.queries.SeoCrawlLinks(ctx, crawl.ID)
	if err != nil {
		return Graph{}, err
	}

	locale := ""
	if ch, err := s.queries.GetChannelByID(ctx, channelID); err == nil {
		locale = ch.DefaultLocale
	}
	inventory, err := s.queries.LinkGraphNodes(ctx, db.LinkGraphNodesParams{ChannelID: channelID, Locale: locale})
	if err != nil {
		return Graph{}, err
	}
	localePrefixes := []string{}
	if rows, err := s.queries.ListChannelLocales(ctx, channelID); err == nil {
		for _, l := range rows {
			localePrefixes = append(localePrefixes, l.Code)
		}
	}

	labelByEntity := make(map[string]string, len(inventory))
	entityByPath := make(map[string]db.LinkGraphNodesRow, len(inventory))
	for _, e := range inventory {
		labelByEntity[e.NodeType+":"+e.NodeID.String()] = e.Label
		entityByPath[e.Path] = e
	}

	nodes := make([]*Node, 0, len(pages))
	byPath := make(map[string]*Node, len(pages))
	seenLookup := make(map[string]struct{}, len(pages))
	for _, p := range pages {
		nodeType := typeOther
		var entityID uuid.UUID
		if p.EntityType.Valid && p.EntityID.Valid {
			nodeType = p.EntityType.String
			entityID = p.EntityID.UUID
		}
		label := p.Title
		if fromEntity, ok := labelByEntity[nodeType+":"+entityID.String()]; ok && fromEntity != "" {
			label = fromEntity
		}
		if label == "" {
			label = p.Path
		}
		n := &Node{
			Key: "p:" + p.Path, Type: nodeType, EntityID: entityID, Label: label, Path: p.Path,
			Kind: p.Kind, State: StateCrawled, StatusCode: int(p.StatusCode), Noindex: p.Noindex,
			IsHomepage: p.Path == "/", Depth: int(p.Depth),
			InLinks: int(p.InLinks), OutLinks: int(p.OutLinks),
		}
		nodes = append(nodes, n)
		byPath[p.Path] = n
		seenLookup[p.LookupPath] = struct{}{}
	}

	// A link target the crawl ran out of budget for is still a page something links to - dropping it threw away
	// most of the graph and then re-reported those pages as unlinked
	for _, l := range links {
		if _, ok := byPath[l.ToPath]; ok {
			continue
		}
		lookup := sitecrawl.LookupPath(l.ToPath, localePrefixes)
		nodeType, label := typeOther, l.ToPath
		var entityID uuid.UUID
		if e, ok := entityByPath[lookup]; ok {
			nodeType, entityID = e.NodeType, e.NodeID
			if e.Label != "" {
				label = e.Label
			}
		}
		n := &Node{
			Key: "p:" + l.ToPath, Type: nodeType, EntityID: entityID, Label: label, Path: l.ToPath,
			Kind: classify(l.ToPath), State: StateDiscovered, Depth: -1,
		}
		nodes = append(nodes, n)
		byPath[l.ToPath] = n
		seenLookup[lookup] = struct{}{}
	}

	for _, e := range inventory {
		if e.NodeType == "menu" || e.Path == "" {
			continue
		}
		if _, seen := seenLookup[e.Path]; seen {
			continue
		}
		nodes = append(nodes, &Node{
			Key: e.NodeType + ":" + e.NodeID.String(), Type: e.NodeType, EntityID: e.NodeID,
			Label: e.Label, Path: "/" + e.Path, Kind: classify("/" + e.Path), State: StateUnseen,
			Noindex: e.Noindex, Depth: -1,
		})
	}

	boilerplate := boilerplateTargets(links, len(pages))
	edges := make([]Edge, 0, len(links))
	for _, l := range links {
		from, okFrom := byPath[l.FromPath]
		to, okTo := byPath[l.ToPath]
		if !okFrom || !okTo || from == to {
			continue
		}
		_, chrome := boilerplate[l.ToPath]
		edges = append(edges, Edge{From: from.Key, To: to.Key, Nofollow: l.Nofollow, Boilerplate: chrome})
		if to.State == StateDiscovered {
			to.InLinks++
			if from.Depth >= 0 && (to.Depth < 0 || from.Depth+1 < to.Depth) {
				to.Depth = from.Depth + 1
			}
		}
	}

	stats := Stats{TotalNodes: len(nodes), TotalEdges: len(edges), Complete: !crawl.HitBudget}
	for _, n := range nodes {
		if n.Depth > stats.MaxDepth {
			stats.MaxDepth = n.Depth
		}
		if n.Noindex {
			stats.Noindexed++
		}
		switch n.State {
		case StateCrawled:
			stats.Crawled++
			switch {
			case n.StatusCode >= 400 || n.StatusCode == 0:
				stats.Broken++
			case n.StatusCode >= 300:
				stats.Redirected++
			}
			if n.InLinks == 0 && !n.IsHomepage {
				stats.Orphans++
			}
		case StateDiscovered:
			stats.Discovered++
		case StateUnseen:
			stats.Unseen++
		}
	}

	for _, e := range edges {
		if e.Boilerplate {
			stats.Boilerplate++
		} else {
			stats.Contextual++
		}
	}

	kept := selectNodes(nodes, maxNodes)
	stats.ReturnedNode = len(kept)
	stats.Dropped = len(nodes) - len(kept)

	keptSet := make(map[string]struct{}, len(kept))
	for _, n := range kept {
		keptSet[n.Key] = struct{}{}
	}
	keptEdges := make([]Edge, 0, len(edges))
	for _, e := range edges {
		_, okFrom := keptSet[e.From]
		_, okTo := keptSet[e.To]
		if okFrom && okTo {
			keptEdges = append(keptEdges, e)
		}
	}

	return Graph{Nodes: kept, Edges: keptEdges, Stats: stats, Crawl: toCrawlInfo(crawl)}, nil
}

// Nav and footer links repeat on nearly every page, so they connect everything to everything and drown the real
// structure - half the edges on a typical storefront. Needs a decent sample before the share means anything
func boilerplateTargets(links []db.SeoCrawlLinksRow, crawledPages int) map[string]struct{} {
	out := map[string]struct{}{}
	if crawledPages < 20 {
		return out
	}
	fromPages := make(map[string]int, len(links))
	for _, l := range links {
		fromPages[l.ToPath]++
	}
	threshold := crawledPages / 2
	for path, count := range fromPages {
		if count > threshold {
			out[path] = struct{}{}
		}
	}
	return out
}

func (s *Service) pickCrawl(ctx context.Context, channelID uuid.UUID, crawlID *uuid.UUID) (db.SeoCrawl, error) {
	if crawlID != nil {
		crawl, err := s.queries.GetSeoCrawl(ctx, *crawlID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return db.SeoCrawl{}, ErrNoCrawl
			}
			return db.SeoCrawl{}, err
		}
		if crawl.ChannelID != channelID {
			return db.SeoCrawl{}, ErrNoCrawl
		}
		return crawl, nil
	}
	crawl, err := s.queries.LatestSeoCrawl(ctx, channelID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.SeoCrawl{}, ErrNoCrawl
	}
	return crawl, err
}

func toCrawlInfo(c db.SeoCrawl) CrawlInfo {
	out := CrawlInfo{
		ID: c.ID, BaseURL: c.BaseUrl, Status: c.Status,
		MaxPages: int(c.MaxPages), MaxDepth: int(c.MaxDepth), RespectRobots: c.RespectRobots,
		PagesCrawled: int(c.PagesCrawled), LinksFound: int(c.LinksFound), HitBudget: c.HitBudget,
		Error: c.Error, StartedAt: c.StartedAt,
	}
	if c.FinishedAt.Valid {
		finished := c.FinishedAt.Time
		out.FinishedAt = &finished
	}
	return out
}

// Skeleton first so the map stays legible, then the findings - a site with 10k unlinked products would otherwise
// push every category and landing page out of the picture
func selectNodes(nodes []*Node, maxNodes int) []Node {
	ranked := make([]*Node, len(nodes))
	copy(ranked, nodes)
	sort.SliceStable(ranked, func(i, j int) bool {
		return priority(ranked[i]) < priority(ranked[j])
	})
	limit := min(maxNodes, len(ranked))
	out := make([]Node, 0, limit)
	for _, n := range ranked[:limit] {
		out = append(out, *n)
	}
	return out
}

func priority(n *Node) int {
	structural := n.Type != "product" && n.Type != typeOther
	crawled := n.State == StateCrawled
	switch {
	case n.IsHomepage:
		return 0
	case crawled && structural:
		return 1
	case crawled && (n.StatusCode >= 400 || n.StatusCode == 0):
		return 2
	case crawled && n.InLinks == 0:
		return 3
	case structural:
		return 4
	case crawled && n.StatusCode >= 300:
		return 5
	case crawled && strings.Count(strings.Trim(n.Path, "/"), "/") == 0:
		return 6
	case crawled:
		return 7
	case n.State == StateDiscovered:
		return 8
	default:
		return 9
	}
}
