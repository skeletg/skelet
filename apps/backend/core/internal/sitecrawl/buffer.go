package sitecrawl

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// buffer batches rows so a 20k-page crawl is a few hundred round trips rather than 20k
type buffer struct {
	crawlID uuid.UUID
	pages   int
	links   int

	paths, lookups, titles, canonicals, redirects []string
	metaDescs, robotsMetas, kinds                 []string
	headings                                      []string
	statuses, depths, fetchTimes, sizes           []int32
	wordCounts, internalLinks, externalLinks      []int32
	noindexes, nofollows                          []bool

	fromPaths, toPaths, anchors []string
	occurrences                 []int32
	linkNofollows               []bool
}

func newBuffer(crawlID uuid.UUID) *buffer { return &buffer{crawlID: crawlID} }

func (b *buffer) pending() int { return len(b.paths) + len(b.fromPaths) }

func (b *buffer) add(res Result, localePrefixes []string) {
	p := res.Page
	b.paths = append(b.paths, p.Path)
	b.lookups = append(b.lookups, LookupPath(p.Path, localePrefixes))
	b.statuses = append(b.statuses, int32(p.StatusCode))
	b.titles = append(b.titles, p.Title)
	b.metaDescs = append(b.metaDescs, p.MetaDesc)
	b.robotsMetas = append(b.robotsMetas, p.RobotsMeta)
	b.kinds = append(b.kinds, p.Kind)
	outline, err := json.Marshal(p.Headings)
	if err != nil || p.Headings == nil {
		outline = []byte("[]")
	}
	b.headings = append(b.headings, string(outline))
	b.wordCounts = append(b.wordCounts, int32(p.WordCount))
	b.internalLinks = append(b.internalLinks, int32(p.OutLinks))
	b.externalLinks = append(b.externalLinks, int32(p.ExtLinks))
	b.noindexes = append(b.noindexes, p.Noindex)
	b.nofollows = append(b.nofollows, p.Nofollow)
	b.canonicals = append(b.canonicals, p.Canonical)
	b.redirects = append(b.redirects, p.RedirectTo)
	b.depths = append(b.depths, int32(p.Depth))
	b.fetchTimes = append(b.fetchTimes, int32(p.FetchMS))
	b.sizes = append(b.sizes, int32(p.ByteSize))
	b.pages++

	for _, l := range res.Links {
		b.fromPaths = append(b.fromPaths, l.FromPath)
		b.toPaths = append(b.toPaths, l.ToPath)
		b.occurrences = append(b.occurrences, int32(l.Occurrences))
		b.linkNofollows = append(b.linkNofollows, l.Nofollow)
		b.anchors = append(b.anchors, l.Anchor)
		b.links++
	}
}

func (b *buffer) flushPages(ctx context.Context, q *db.Queries) error {
	if len(b.paths) == 0 {
		return nil
	}
	err := q.InsertSeoCrawlPages(ctx, db.InsertSeoCrawlPagesParams{
		CrawlID:          b.crawlID,
		Paths:            b.paths,
		LookupPaths:      b.lookups,
		StatusCodes:      b.statuses,
		Titles:           b.titles,
		MetaDescriptions: b.metaDescs,
		RobotsMetas:      b.robotsMetas,
		Headings:         b.headings,
		WordCounts:       b.wordCounts,
		Kinds:            b.kinds,
		InternalLinks:    b.internalLinks,
		ExternalLinks:    b.externalLinks,
		Noindexes:        b.noindexes,
		Nofollows:        b.nofollows,
		Canonicals:       b.canonicals,
		Redirects:        b.redirects,
		Depths:           b.depths,
		FetchTimes:       b.fetchTimes,
		ByteSizes:        b.sizes,
	})
	if err != nil {
		return err
	}
	b.paths, b.lookups, b.titles, b.canonicals, b.redirects = nil, nil, nil, nil, nil
	b.metaDescs, b.robotsMetas, b.kinds, b.headings = nil, nil, nil, nil
	b.statuses, b.depths, b.fetchTimes, b.sizes = nil, nil, nil, nil
	b.wordCounts, b.internalLinks, b.externalLinks = nil, nil, nil
	b.noindexes, b.nofollows = nil, nil
	return nil
}

// Postgres refuses an ON CONFLICT DO UPDATE that touches one row twice, so a pair is only ever written once per batch
func (b *buffer) flushLinks(ctx context.Context, q *db.Queries) error {
	if len(b.fromPaths) == 0 {
		return nil
	}
	seen := map[string]int{}
	from := make([]string, 0, len(b.fromPaths))
	to := make([]string, 0, len(b.toPaths))
	occ := make([]int32, 0, len(b.occurrences))
	nof := make([]bool, 0, len(b.linkNofollows))
	anchors := make([]string, 0, len(b.anchors))
	for i := range b.fromPaths {
		key := b.fromPaths[i] + "\x00" + b.toPaths[i]
		if at, dup := seen[key]; dup {
			occ[at] += b.occurrences[i]
			continue
		}
		seen[key] = len(from)
		from = append(from, b.fromPaths[i])
		to = append(to, b.toPaths[i])
		occ = append(occ, b.occurrences[i])
		nof = append(nof, b.linkNofollows[i])
		anchors = append(anchors, b.anchors[i])
	}
	err := q.InsertSeoCrawlLinks(ctx, db.InsertSeoCrawlLinksParams{
		CrawlID:          b.crawlID,
		FromPaths:        from,
		ToPaths:          to,
		OccurrenceCounts: occ,
		Nofollows:        nof,
		Anchors:          anchors,
	})
	if err != nil {
		return err
	}
	b.fromPaths, b.toPaths, b.anchors = nil, nil, nil
	b.occurrences = nil
	b.linkNofollows = nil
	return nil
}
