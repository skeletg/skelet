package review

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
)

type Filter struct {
	Start       time.Time
	End         time.Time
	AllChannels bool
	ChannelIDs  []uuid.UUID
}

// One category's count for the window plus the prior equal-length one, for deltas - an empty Slug is the uncategorized bucket, and a multi-category review counts toward each, so counts can sum past the review total
type CategoryStat struct {
	Slug      string
	Name      string
	Count     int64
	PrevCount int64
}

// One (period, category slug) bucket for the category trend - multi-category reviews appear in each of their bands
type TimePoint struct {
	Period time.Time
	Slug   string
	Count  int64
}

// The true review count for one period, one row per review with no category unnest - drives the volume trend correctly under multi-category
type VolumePoint struct {
	Period time.Time
	Count  int64
}

type StatusStat struct {
	Status string
	Count  int64
}

// One moderation tag's flag count for the current window plus the prior equal-length one - the Slug may be unresolved, with Name equal to Slug, when the tag was deleted after being assigned
type TagStat struct {
	Slug      string
	Name      string
	Count     int64
	PrevCount int64
}

// The full review-analytics payload - category and tag breakdowns with prior-window deltas, category and true-volume trends, totals, and the status breakdown
type Insights struct {
	ByCategory []CategoryStat
	Timeseries []TimePoint
	Volume     []VolumePoint
	ByStatus   []StatusStat
	ByTag      []TagStat
	Total      int64
	PrevTotal  int64
}

func (r *postgresRepository) reviewCategoryCounts(ctx context.Context, f Filter) (map[string]int64, error) {
	rows, err := r.queries.ReviewCategoryCounts(ctx, db.ReviewCategoryCountsParams{
		StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("review category counts: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.Slug.Valid {
			out[row.Slug.String] = row.ReviewCount
		}
	}
	return out, nil
}

func (r *postgresRepository) reviewUncategorizedCount(ctx context.Context, f Filter) (int64, error) {
	n, err := r.queries.ReviewUncategorizedCount(ctx, db.ReviewUncategorizedCountParams{
		StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return 0, fmt.Errorf("review uncategorized count: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) reviewCategoryTimeseries(ctx context.Context, f Filter, bucket string) ([]TimePoint, error) {
	rows, err := r.queries.ReviewCategoryTimeseries(ctx, db.ReviewCategoryTimeseriesParams{
		Bucket: bucket, StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("review category timeseries: %w", err)
	}
	out := make([]TimePoint, 0, len(rows))
	for _, row := range rows {
		if !row.Slug.Valid {
			continue
		}
		out = append(out, TimePoint{Period: row.Period, Slug: row.Slug.String, Count: row.ReviewCount})
	}
	return out, nil
}

func (r *postgresRepository) reviewVolumeTimeseries(ctx context.Context, f Filter, bucket string) ([]VolumePoint, error) {
	rows, err := r.queries.ReviewVolumeTimeseries(ctx, db.ReviewVolumeTimeseriesParams{
		Bucket: bucket, StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("review volume timeseries: %w", err)
	}
	out := make([]VolumePoint, len(rows))
	for i, row := range rows {
		out[i] = VolumePoint{Period: row.Period, Count: row.ReviewCount}
	}
	return out, nil
}

func (r *postgresRepository) reviewStatusCounts(ctx context.Context, f Filter) ([]StatusStat, error) {
	rows, err := r.queries.ReviewStatusCounts(ctx, db.ReviewStatusCountsParams{
		StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("review status counts: %w", err)
	}
	out := make([]StatusStat, len(rows))
	for i, row := range rows {
		out[i] = StatusStat{Status: row.Status, Count: row.ReviewCount}
	}
	return out, nil
}

func (r *postgresRepository) reviewTagCounts(ctx context.Context, f Filter) (map[string]int64, error) {
	rows, err := r.queries.ReviewTagCounts(ctx, db.ReviewTagCountsParams{
		StartAt: f.Start, EndAt: f.End, AllChannels: f.AllChannels, ChannelIds: f.ChannelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("review tag counts: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.Tag.Valid {
			out[row.Tag.String] = row.ReviewCount
		}
	}
	return out, nil
}

// The breakdowns and trends for f, each category and tag carrying its prior-window count for deltas - bucket is the date_trunc unit, defaulting to "day"
func (s *Service) Insights(ctx context.Context, f Filter, bucket string) (Insights, error) {
	if bucket == "" {
		bucket = "day"
	}
	// The prior equal-length window immediately before [Start, End]
	prev := Filter{
		Start:       f.Start.Add(-f.End.Sub(f.Start)),
		End:         f.Start,
		AllChannels: f.AllChannels,
		ChannelIDs:  f.ChannelIDs,
	}

	var out Insights

	// True volume, one row per review, drives the chart and totals - correct under multi-category, unlike summing category counts
	curVol, err := s.repo.reviewVolumeTimeseries(ctx, f, bucket)
	if err != nil {
		return Insights{}, err
	}
	out.Volume = curVol
	for _, v := range curVol {
		out.Total += v.Count
	}
	prevVol, err := s.repo.reviewVolumeTimeseries(ctx, prev, bucket)
	if err != nil {
		return Insights{}, err
	}
	for _, v := range prevVol {
		out.PrevTotal += v.Count
	}

	curCat, err := s.repo.reviewCategoryCounts(ctx, f)
	if err != nil {
		return Insights{}, err
	}
	prevCat, err := s.repo.reviewCategoryCounts(ctx, prev)
	if err != nil {
		return Insights{}, err
	}
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return Insights{}, err
	}
	catName, catOrder := slugNameOrder(cats)
	for slug := range curCat {
		catName, catOrder = ensureSlug(catName, catOrder, slug)
	}
	for slug := range prevCat {
		catName, catOrder = ensureSlug(catName, catOrder, slug)
	}
	for _, slug := range catOrder {
		c, p := curCat[slug], prevCat[slug]
		if c == 0 && p == 0 {
			continue
		}
		out.ByCategory = append(out.ByCategory, CategoryStat{Slug: slug, Name: catName[slug], Count: c, PrevCount: p})
	}
	curUncat, err := s.repo.reviewUncategorizedCount(ctx, f)
	if err != nil {
		return Insights{}, err
	}
	prevUncat, err := s.repo.reviewUncategorizedCount(ctx, prev)
	if err != nil {
		return Insights{}, err
	}
	if curUncat > 0 || prevUncat > 0 {
		out.ByCategory = append(out.ByCategory, CategoryStat{Slug: "", Name: "Uncategorized", Count: curUncat, PrevCount: prevUncat})
	}

	if out.Timeseries, err = s.repo.reviewCategoryTimeseries(ctx, f, bucket); err != nil {
		return Insights{}, err
	}

	// Status breakdown, current window only - a distribution rather than a trend
	if out.ByStatus, err = s.repo.reviewStatusCounts(ctx, f); err != nil {
		return Insights{}, err
	}

	// Moderation-tag breakdown with prior-window deltas - resolve slugs to names
	curTags, err := s.repo.reviewTagCounts(ctx, f)
	if err != nil {
		return Insights{}, err
	}
	prevTags, err := s.repo.reviewTagCounts(ctx, prev)
	if err != nil {
		return Insights{}, err
	}
	modTags, err := s.repo.ListModerationTags(ctx)
	if err != nil {
		return Insights{}, err
	}
	tagName, tagOrder := slugNameOrderTags(modTags)
	for slug := range curTags {
		tagName, tagOrder = ensureSlug(tagName, tagOrder, slug)
	}
	for slug := range prevTags {
		tagName, tagOrder = ensureSlug(tagName, tagOrder, slug)
	}
	for _, slug := range tagOrder {
		c, p := curTags[slug], prevTags[slug]
		if c == 0 && p == 0 {
			continue
		}
		out.ByTag = append(out.ByTag, TagStat{Slug: slug, Name: tagName[slug], Count: c, PrevCount: p})
	}
	return out, nil
}

// slugNameOrder seeds a slug→name map + ordered slug list from the category
// taxonomy. slugNameOrderTags is the same for moderation tags
func slugNameOrder(cats []Category) (map[string]string, []string) {
	name := make(map[string]string, len(cats))
	order := make([]string, 0, len(cats))
	for _, c := range cats {
		name[c.Slug] = c.Name
		order = append(order, c.Slug)
	}
	return name, order
}

func slugNameOrderTags(tags []ModerationTag) (map[string]string, []string) {
	name := make(map[string]string, len(tags))
	order := make([]string, 0, len(tags))
	for _, t := range tags {
		name[t.Slug] = t.Name
		order = append(order, t.Slug)
	}
	return name, order
}

// ensureSlug appends an unseen slug, name defaulting to the slug itself (for slugs assigned then deleted)
func ensureSlug(name map[string]string, order []string, slug string) (map[string]string, []string) {
	if _, ok := name[slug]; ok {
		return name, order
	}
	name[slug] = slug
	return name, append(order, slug)
}
