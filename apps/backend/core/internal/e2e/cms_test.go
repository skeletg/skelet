package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Scenario: a published page listed on no channel stays hidden whatever channel header the caller sends - a bogus id resolves no store, and a storeless read must not fall back to the global flag
func TestAnUnlistedPageIsHiddenFromEveryHeader(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	sfx := uuid.NewString()[:8]
	var typeID, unlisted, listed uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO cms_page_types (slug) VALUES ($1) RETURNING id`, "e2e-"+sfx).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		id   *uuid.UUID
		slug string
	}{{&unlisted, "unlisted-" + sfx}, {&listed, "listed-" + sfx}} {
		if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO cms_pages (page_type_id, slug, base_path, url_path, is_published) VALUES ($1, $2, $2, $2, TRUE) RETURNING id`, typeID, p.slug).Scan(p.id); err != nil {
			t.Fatal(err)
		}
		if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO cms_page_content (page_id, locale, title) VALUES ($1, 'en', $2)`, *p.id, p.slug); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.D.DB.ExecContext(h.Ctx, `INSERT INTO cms_page_channel_listings (page_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, listed, f.Channel.ID); err != nil {
		t.Fatal(err)
	}

	for _, header := range []string{uuid.NewString(), "not-a-uuid", f.Channel.ID.String()} {
		data, _ := h.gqlPost(t, ts, header, fmt.Sprintf(`{ cmsPage(slug: %q) { id } cmsPages(limit: 500) { id } }`, "unlisted-"+sfx))
		if data["cmsPage"] != nil {
			t.Errorf("header %q: cmsPage returned the unlisted page", header)
		}
		pages, _ := data["cmsPages"].([]any)
		for _, p := range pages {
			if p.(map[string]any)["id"] == unlisted.String() {
				t.Errorf("header %q: cmsPages listed the unlisted page", header)
			}
		}
	}

	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ cmsPage(slug: %q) { id } }`, "listed-"+sfx))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := gqlField(t, data, "cmsPage", "id"); got != listed.String() {
		t.Errorf("the listed page = %q, want it served on its channel", got)
	}
}
