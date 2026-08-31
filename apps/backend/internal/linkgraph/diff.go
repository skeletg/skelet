package linkgraph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

var ErrNoBaseline = errors.New("no earlier completed crawl to compare against")

type ChangeKind string

const (
	ChangeNowBroken    ChangeKind = "NOW_BROKEN"
	ChangeFixed        ChangeKind = "FIXED"
	ChangeNowRedirect  ChangeKind = "NOW_REDIRECT"
	ChangeNowNoindex   ChangeKind = "NOW_NOINDEX"
	ChangeNowIndexable ChangeKind = "NOW_INDEXABLE"
	ChangeNowOrphan    ChangeKind = "NOW_ORPHAN"
	ChangeLostInLinks  ChangeKind = "LOST_IN_LINKS"
	ChangeDeeper       ChangeKind = "DEEPER"
	ChangeTitle        ChangeKind = "TITLE_CHANGED"
	ChangeMeta         ChangeKind = "META_CHANGED"
	ChangeCanonical    ChangeKind = "CANONICAL_CHANGED"
	ChangeH1           ChangeKind = "H1_CHANGED"
	ChangeGone         ChangeKind = "GONE"
	ChangeNew          ChangeKind = "NEW"
)

// What is worth waking someone up for - a title edit is information, a page that started 404ing is a problem
var regressions = map[ChangeKind]bool{
	ChangeNowBroken:   true,
	ChangeNowRedirect: true,
	ChangeNowNoindex:  true,
	ChangeNowOrphan:   true,
	ChangeLostInLinks: true,
	ChangeGone:        true,
}

func (k ChangeKind) IsRegression() bool { return regressions[k] }

type Change struct {
	Path   string
	Kind   ChangeKind
	Before string
	After  string
}

type Diff struct {
	Base    CrawlInfo
	Current CrawlInfo
	Changes []Change
	Counts  map[ChangeKind]int
	// Pages both crawls actually fetched - the only population a comparison can speak about
	Compared    int
	Regressions int
	// False when either crawl stopped on its page budget, which makes GONE and NEW unsafe to infer
	CoverageComparable bool
}

// Thresholds chosen so ordinary churn stays quiet: a link count wobbling by one is noise, half of them vanishing is not
const (
	lostLinkShare = 0.5
	lostLinkFloor = 3
	deeperBy      = 2
)

func (s *Service) Diff(ctx context.Context, channelID uuid.UUID, baseID, currentID *uuid.UUID) (Diff, error) {
	current, err := s.pickCrawl(ctx, channelID, currentID)
	if err != nil {
		return Diff{}, err
	}
	var base db.SeoCrawl
	if baseID != nil {
		base, err = s.queries.GetSeoCrawl(ctx, *baseID)
		if err == nil && base.ChannelID != channelID {
			err = ErrNoBaseline
		}
	} else {
		base, err = s.queries.PreviousSeoCrawl(ctx, db.PreviousSeoCrawlParams{
			ChannelID: channelID, Before: current.StartedAt,
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrNoBaseline
		}
	}
	if err != nil {
		return Diff{}, err
	}

	beforeRows, err := s.queries.SeoCrawlDiffRows(ctx, base.ID)
	if err != nil {
		return Diff{}, err
	}
	afterRows, err := s.queries.SeoCrawlDiffRows(ctx, current.ID)
	if err != nil {
		return Diff{}, err
	}

	before := make(map[string]db.SeoCrawlDiffRowsRow, len(beforeRows))
	for _, r := range beforeRows {
		before[r.Path] = r
	}
	after := make(map[string]db.SeoCrawlDiffRowsRow, len(afterRows))
	for _, r := range afterRows {
		after[r.Path] = r
	}

	out := diffPages(before, after, !base.HitBudget && !current.HitBudget)
	out.Base, out.Current = toCrawlInfo(base), toCrawlInfo(current)
	sort.SliceStable(out.Changes, func(i, j int) bool {
		a, b := out.Changes[i], out.Changes[j]
		if a.Kind.IsRegression() != b.Kind.IsRegression() {
			return a.Kind.IsRegression()
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Path < b.Path
	})
	return out, nil
}

// diffPages is the whole comparison, kept free of the database so the thresholds can be pinned by tests
func diffPages(before, after map[string]db.SeoCrawlDiffRowsRow, coverageComparable bool) Diff {
	out := Diff{Counts: map[ChangeKind]int{}, CoverageComparable: coverageComparable}
	add := func(path string, kind ChangeKind, was, now string) {
		out.Changes = append(out.Changes, Change{Path: path, Kind: kind, Before: was, After: now})
		out.Counts[kind]++
		if kind.IsRegression() {
			out.Regressions++
		}
	}

	for path, now := range after {
		was, existed := before[path]
		if !existed {
			// A crawl that ran out of budget simply stopped somewhere else, so "new" would be an artifact of where
			if coverageComparable {
				add(path, ChangeNew, "", statusLabel(now.StatusCode))
			}
			continue
		}
		out.Compared++

		wasOK := was.StatusCode >= 200 && was.StatusCode < 300
		nowOK := now.StatusCode >= 200 && now.StatusCode < 300
		switch {
		case wasOK && (now.StatusCode >= 400 || now.StatusCode == 0):
			add(path, ChangeNowBroken, statusLabel(was.StatusCode), statusLabel(now.StatusCode))
		case wasOK && now.StatusCode >= 300 && now.StatusCode < 400:
			add(path, ChangeNowRedirect, statusLabel(was.StatusCode), statusLabel(now.StatusCode))
		case !wasOK && nowOK:
			add(path, ChangeFixed, statusLabel(was.StatusCode), statusLabel(now.StatusCode))
		}
		if !nowOK {
			continue // a page with no content has nothing else worth comparing
		}

		if !was.Noindex && now.Noindex {
			add(path, ChangeNowNoindex, "indexable", "noindex")
		}
		if was.Noindex && !now.Noindex {
			add(path, ChangeNowIndexable, "noindex", "indexable")
		}
		if was.Title != now.Title {
			add(path, ChangeTitle, was.Title, now.Title)
		}
		if was.MetaDescription != now.MetaDescription {
			add(path, ChangeMeta, was.MetaDescription, now.MetaDescription)
		}
		if was.Canonical != now.Canonical {
			add(path, ChangeCanonical, was.Canonical, now.Canonical)
		}
		if was.H1 != now.H1 {
			add(path, ChangeH1, was.H1, now.H1)
		}
		switch {
		case was.InLinks > 0 && now.InLinks == 0:
			add(path, ChangeNowOrphan, fmt.Sprintf("%d in", was.InLinks), "0 in")
		case was.InLinks >= lostLinkFloor && float64(now.InLinks) < float64(was.InLinks)*lostLinkShare:
			add(path, ChangeLostInLinks, fmt.Sprintf("%d in", was.InLinks), fmt.Sprintf("%d in", now.InLinks))
		}
		if was.Depth >= 0 && now.Depth >= was.Depth+deeperBy {
			add(path, ChangeDeeper, fmt.Sprintf("%d clicks", was.Depth), fmt.Sprintf("%d clicks", now.Depth))
		}
	}

	if coverageComparable {
		for path, was := range before {
			if _, still := after[path]; !still {
				add(path, ChangeGone, statusLabel(was.StatusCode), "not reached")
			}
		}
	}
	return out
}

func statusLabel(code int32) string {
	if code == 0 {
		return "no response"
	}
	return fmt.Sprintf("HTTP %d", code)
}
