package front

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
)

func TestAStorelessRequestSeesNoCMSPages(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	slug := "summer-2026"
	if p, err := q.CmsPage(context.Background(), &slug, nil, nil); p != nil || err != nil {
		t.Errorf("cmsPage = %v, %v, want nothing without a store", p, err)
	}
	if ps, err := q.CmsPages(context.Background(), nil, nil, nil); len(ps) != 0 || err != nil {
		t.Errorf("cmsPages = %d, %v, want none without a store", len(ps), err)
	}
}

func TestPreparedBodyDropsAnUnparseableDocument(t *testing.T) {
	ec := blockkit.EvalContext{Now: time.Now(), Segments: map[string]bool{}}
	bad := json.RawMessage(`{"version":"1","blocks":[{"id":"x","type":"html","props":{"html":"<img src=x onerror=alert(1)>"}}]}`)
	if got := preparedCMSBody(bad, nil, nil, nil, nil, nil, ec); got != nil {
		t.Errorf("an unparseable document shipped as %s", got)
	}
	freeform := json.RawMessage(`{"root":{"children":[]}}`)
	if got := preparedCMSBody(freeform, nil, nil, nil, nil, nil, ec); string(got) != string(freeform) {
		t.Errorf("a freeform body changed to %s", got)
	}
	ok := json.RawMessage(`{"version":1,"blocks":[{"id":"x","type":"html","props":{"html":"<img src=x onerror=alert(1)>"}}]}`)
	var doc blockkit.Document
	if err := json.Unmarshal(preparedCMSBody(ok, nil, nil, nil, nil, nil, ec), &doc); err != nil || len(doc.Blocks) != 1 {
		t.Fatalf("a valid document = %+v, %v", doc, err)
	}
	if html, _ := doc.Blocks[0].Props["html"].(string); html != `<img src="x">` {
		t.Errorf("html = %q, want it sanitized", html)
	}
}

func TestTheChannelListingsNoindexReachesTheStorefront(t *testing.T) {
	ch, other := uuid.New(), uuid.New()
	p := cms.Page{ID: uuid.New(), ChannelListings: []cms.PageChannelListing{{ChannelID: ch, IsPublished: true, SEONoindex: true}, {ChannelID: other, IsPublished: true}}}
	if m := toFrontCMSPageModel(p, &ch, nil); !m.SeoNoindex {
		t.Error("the listing's noindex was ignored")
	}
	if m := toFrontCMSPageModel(p, &other, nil); m.SeoNoindex {
		t.Error("another channel's listing noindexed this one")
	}
}

func TestThePublicPageWindowIsBounded(t *testing.T) {
	n := func(v int32) *int32 { return &v }
	for name, tc := range map[string]struct {
		limit, offset *int32
		wantL, wantO  int32
	}{
		"defaults":        {nil, nil, cmsPagesDefaultLimit, 0},
		"within":          {n(20), n(40), 20, 40},
		"over the cap":    {n(100000), nil, cmsPagesMaxLimit, 0},
		"zero":            {n(0), nil, cmsPagesDefaultLimit, 0},
		"negative offset": {n(10), n(-5), 10, 0},
		"negative limit":  {n(-1), nil, cmsPagesDefaultLimit, 0},
	} {
		if l, o := cmsPageWindow(tc.limit, tc.offset); l != tc.wantL || o != tc.wantO {
			t.Errorf("%s: window = %d, %d, want %d, %d", name, l, o, tc.wantL, tc.wantO)
		}
	}
}
