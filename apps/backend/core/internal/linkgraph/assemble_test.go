package linkgraph

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func crawled(path string, depth, in int32) db.SeoCrawlPagesRow {
	return db.SeoCrawlPagesRow{Path: path, LookupPath: lookupOf(path), StatusCode: 200, Depth: depth, InLinks: in, Kind: "page"}
}

func lookupOf(path string) string {
	if path == "/" {
		return ""
	}
	return path[1:]
}

func link(from, to string) db.SeoCrawlLinksRow {
	return db.SeoCrawlLinksRow{FromPath: from, ToPath: to}
}

var completed = db.SeoCrawl{Status: "completed"}

func byKey(g Graph) map[string]Node {
	out := map[string]Node{}
	for _, n := range g.Nodes {
		out[n.Key] = n
	}
	return out
}

// A link target the crawl never fetched is still a page something links to - it joins as discovered, one click deeper than its shallowest linker
func TestALinkedButUnfetchedPageIsDiscoveredAtTheRightDepth(t *testing.T) {
	g := assemble(completed,
		[]db.SeoCrawlPagesRow{crawled("/", 0, 0), crawled("/shoes", 1, 1), crawled("/shoes/deep", 3, 1)},
		[]db.SeoCrawlLinksRow{link("/", "/shoes"), link("/shoes", "/boots"), link("/shoes/deep", "/boots")},
		nil, nil, 100)
	boots, ok := byKey(g)["p:/boots"]
	if !ok {
		t.Fatalf("the unfetched link target is missing: %+v", g.Nodes)
	}
	if boots.State != StateDiscovered || boots.Depth != 2 || boots.InLinks != 2 {
		t.Errorf("boots = %+v, want discovered at depth 2 with both in-links", boots)
	}
	if g.Stats.Discovered != 1 || g.Stats.Crawled != 3 {
		t.Errorf("stats %+v", g.Stats)
	}
}

// A catalogue entity no crawled or linked url resolves to is unseen - never an orphan, the crawl simply has nothing to say about it
func TestAnUnreachedEntityIsUnseenNotOrphaned(t *testing.T) {
	id := uuid.New()
	g := assemble(completed,
		[]db.SeoCrawlPagesRow{crawled("/", 0, 0), crawled("/shoes", 1, 1)},
		[]db.SeoCrawlLinksRow{link("/", "/shoes")},
		[]db.LinkGraphNodesRow{
			{NodeType: "product", NodeID: id, Label: "Hidden boot", Path: "hidden-boot"},
			{NodeType: "category", NodeID: uuid.New(), Label: "Shoes", Path: "shoes"},
			{NodeType: "menu", NodeID: uuid.New(), Label: "Main", Path: "menu-main"},
		}, nil, 100)
	n, ok := byKey(g)["product:"+id.String()]
	if !ok || n.State != StateUnseen || n.Path != "/hidden-boot" {
		t.Fatalf("unseen entity = %+v (%v)", n, ok)
	}
	if g.Stats.Unseen != 1 || g.Stats.Orphans != 0 {
		t.Errorf("stats %+v - want one unseen, the crawled category matched and the menu skipped, no orphans", g.Stats)
	}
}

func TestOrphansBrokenAndRedirectsAreCounted(t *testing.T) {
	broken := crawled("/gone", 1, 1)
	broken.StatusCode = 404
	moved := crawled("/moved", 1, 1)
	moved.StatusCode = 301
	g := assemble(completed,
		[]db.SeoCrawlPagesRow{crawled("/", 0, 0), crawled("/lonely", 2, 0), broken, moved},
		nil, nil, nil, 100)
	if g.Stats.Orphans != 1 || g.Stats.Broken != 1 || g.Stats.Redirected != 1 {
		t.Errorf("stats %+v - want the homepage spared, one orphan, one broken, one redirect", g.Stats)
	}
}

// Header and footer links repeat everywhere and drown the real structure - only flagged once the sample is big enough to say so
func TestSiteWideLinksAreBoilerplate(t *testing.T) {
	var pages []db.SeoCrawlPagesRow
	var links []db.SeoCrawlLinksRow
	for i := range 30 {
		p := fmt.Sprintf("/p%d", i)
		pages = append(pages, crawled(p, 1, 1))
		links = append(links, link(p, "/cart"))
		if i == 0 {
			links = append(links, link(p, "/p1"))
		}
	}
	pages = append(pages, crawled("/cart", 2, 30))
	g := assemble(completed, pages, links, nil, nil, 100)
	if g.Stats.Boilerplate != 30 || g.Stats.Contextual != 1 {
		t.Errorf("boilerplate %d contextual %d, want 30 and 1", g.Stats.Boilerplate, g.Stats.Contextual)
	}
	small := assemble(completed, pages[:5], links[:5], nil, nil, 100)
	if small.Stats.Boilerplate != 0 {
		t.Errorf("a five-page sample flagged %d boilerplate links", small.Stats.Boilerplate)
	}
}

// The budget keeps the skeleton first, so ten thousand unlinked products can't push every category off the map
func TestTheNodeBudgetKeepsTheSkeleton(t *testing.T) {
	pages := []db.SeoCrawlPagesRow{crawled("/", 0, 0)}
	cat := crawled("/shoes", 1, 1)
	cat.EntityType, cat.EntityID = sql.NullString{String: "category", Valid: true}, uuid.NullUUID{UUID: uuid.New(), Valid: true}
	pages = append(pages, cat)
	for i := range 80 {
		p := crawled(fmt.Sprintf("/shoes/p%d", i), 2, 1)
		p.EntityType, p.EntityID = sql.NullString{String: "product", Valid: true}, uuid.NullUUID{UUID: uuid.New(), Valid: true}
		pages = append(pages, p)
	}
	g := assemble(completed, pages, []db.SeoCrawlLinksRow{link("/", "/shoes"), link("/shoes", "/shoes/p0")}, nil, nil, MinMaxNodes)
	keys := byKey(g)
	if _, ok := keys["p:/"]; !ok {
		t.Error("the homepage was dropped")
	}
	if _, ok := keys["p:/shoes"]; !ok {
		t.Error("the category was dropped for products")
	}
	if g.Stats.ReturnedNode != MinMaxNodes || g.Stats.Dropped != len(pages)-MinMaxNodes {
		t.Errorf("returned %d dropped %d", g.Stats.ReturnedNode, g.Stats.Dropped)
	}
	for _, e := range g.Edges {
		if _, ok := keys[e.From]; !ok {
			t.Errorf("edge from a dropped node %s", e.From)
		}
	}
}

// A failed or still-running crawl that never hit its budget used to read as complete, so its partial map passed for the truth
func TestOnlyACompletedCrawlWithinBudgetIsComplete(t *testing.T) {
	for _, tc := range []struct {
		crawl db.SeoCrawl
		want  bool
	}{
		{db.SeoCrawl{Status: "completed"}, true},
		{db.SeoCrawl{Status: "completed", HitBudget: true}, false},
		{db.SeoCrawl{Status: "failed"}, false},
		{db.SeoCrawl{Status: "running"}, false},
	} {
		if got := assemble(tc.crawl, []db.SeoCrawlPagesRow{crawled("/", 0, 0)}, nil, nil, nil, 100).Stats.Complete; got != tc.want {
			t.Errorf("%s budget=%v: complete = %v, want %v", tc.crawl.Status, tc.crawl.HitBudget, got, tc.want)
		}
	}
}
