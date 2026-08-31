package linkgraph

import "testing"

import db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"

func page(status int32, opts ...func(*db.SeoCrawlDiffRowsRow)) db.SeoCrawlDiffRowsRow {
	row := db.SeoCrawlDiffRowsRow{StatusCode: status, Title: "T", MetaDescription: "M", Canonical: "/c", H1: "H", InLinks: 10, Depth: 1}
	for _, o := range opts {
		o(&row)
	}
	return row
}

func kinds(d Diff) map[ChangeKind]int { return d.Counts }

func TestDiffStatusTransitions(t *testing.T) {
	before := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200), "/b": page(500), "/c": page(200)}
	after := map[string]db.SeoCrawlDiffRowsRow{"/a": page(404), "/b": page(200), "/c": page(301)}
	got := kinds(diffPages(before, after, true))
	for kind, want := range map[ChangeKind]int{ChangeNowBroken: 1, ChangeFixed: 1, ChangeNowRedirect: 1} {
		if got[kind] != want {
			t.Errorf("%s = %d, want %d", kind, got[kind], want)
		}
	}
}

// A page with no content must not also be reported as having lost its title and h1
func TestDiffSkipsContentChecksOnBrokenPages(t *testing.T) {
	before := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200)}
	after := map[string]db.SeoCrawlDiffRowsRow{"/a": page(500, func(r *db.SeoCrawlDiffRowsRow) {
		r.Title, r.MetaDescription, r.Canonical, r.H1, r.InLinks = "", "", "", "", 0
	})}
	got := kinds(diffPages(before, after, true))
	if got[ChangeNowBroken] != 1 {
		t.Fatalf("expected the break to be reported, got %v", got)
	}
	for _, noisy := range []ChangeKind{ChangeTitle, ChangeMeta, ChangeCanonical, ChangeH1, ChangeNowOrphan} {
		if got[noisy] != 0 {
			t.Errorf("%s should not fire on a broken page", noisy)
		}
	}
}

func TestDiffLostInLinksThresholds(t *testing.T) {
	cases := []struct {
		name     string
		was, now int32
		wantKind ChangeKind
	}{
		{"all links gone is an orphan", 10, 0, ChangeNowOrphan},
		{"more than half gone", 10, 4, ChangeLostInLinks},
		{"exactly half kept is quiet", 10, 5, ""},
		{"one link lost is noise", 10, 9, ""},
		{"too few links to judge", 2, 1, ""},
	}
	for _, tc := range cases {
		before := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.InLinks = tc.was })}
		after := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.InLinks = tc.now })}
		got := kinds(diffPages(before, after, true))
		if tc.wantKind == "" {
			if got[ChangeLostInLinks]+got[ChangeNowOrphan] != 0 {
				t.Errorf("%s: expected silence, got %v", tc.name, got)
			}
			continue
		}
		if got[tc.wantKind] != 1 {
			t.Errorf("%s: expected %s, got %v", tc.name, tc.wantKind, got)
		}
	}
}

// The whole point of the guard: a shifting page budget must not read as pages appearing and vanishing
func TestDiffWithholdsGoneAndNewWhenCoverageDiffers(t *testing.T) {
	before := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200), "/gone": page(200)}
	after := map[string]db.SeoCrawlDiffRowsRow{"/a": page(200), "/new": page(200)}

	guarded := kinds(diffPages(before, after, false))
	if guarded[ChangeGone] != 0 || guarded[ChangeNew] != 0 {
		t.Fatalf("expected gone/new withheld on incomparable coverage, got %v", guarded)
	}
	comparable := kinds(diffPages(before, after, true))
	if comparable[ChangeGone] != 1 || comparable[ChangeNew] != 1 {
		t.Fatalf("expected gone/new reported when both crawls are complete, got %v", comparable)
	}
}

func TestDiffDepthAndIndexability(t *testing.T) {
	before := map[string]db.SeoCrawlDiffRowsRow{
		"/deeper": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.Depth = 1 }),
		"/nudged": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.Depth = 1 }),
		"/hidden": page(200),
	}
	after := map[string]db.SeoCrawlDiffRowsRow{
		"/deeper": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.Depth = 3 }),
		"/nudged": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.Depth = 2 }),
		"/hidden": page(200, func(r *db.SeoCrawlDiffRowsRow) { r.Noindex = true }),
	}
	got := kinds(diffPages(before, after, true))
	if got[ChangeDeeper] != 1 {
		t.Errorf("expected only the two-click move to count as deeper, got %d", got[ChangeDeeper])
	}
	if got[ChangeNowNoindex] != 1 {
		t.Errorf("expected the noindex flip, got %v", got)
	}
	if !ChangeNowNoindex.IsRegression() || ChangeTitle.IsRegression() {
		t.Error("regression classification is wrong")
	}
}
