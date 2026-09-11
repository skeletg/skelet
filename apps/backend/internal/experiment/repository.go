package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	db "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

type Repository interface {
	Create(ctx context.Context, e Experiment) (Experiment, error)
	Update(ctx context.Context, e Experiment) (bool, error)
	Get(ctx context.Context, id uuid.UUID) (Experiment, bool, error)
	List(ctx context.Context, channelID uuid.UUID) ([]Experiment, error)
	ListRunning(ctx context.Context, channelID uuid.UUID) ([]Experiment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Results(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error)
	GoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error)
}

type VariationResult struct {
	Variation string
	Exposed   int
	Converted int
	Revenue   int64 // cents from matched goal events (commerce goals)
}

// The spine reads this needs, narrowed so the engine stays pluggable - satisfied by *analytics.Recorder
type SpineReader interface {
	ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error)
	ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]analytics.VariationResult, error)
}

// Definitions and the spine that scores them are both the signals database's, so a results query never crosses over
type repo struct {
	q     *db.Queries
	spine SpineReader
}

// NewRepository wires the sqlc-backed repository
func NewRepository(q *db.Queries, spine SpineReader) Repository {
	return &repo{q: q, spine: spine}
}

func (r *repo) Create(ctx context.Context, e Experiment) (Experiment, error) {
	vars, err := json.Marshal(e.Variations)
	if err != nil {
		return Experiment{}, err
	}
	row, err := r.q.CreateExperiment(ctx, db.CreateExperimentParams{
		ChannelID: e.ChannelID, Key: e.Key, Name: e.Name, Status: e.Status,
		Coverage: int32(e.Coverage), GoalEvent: e.GoalEvent, Variations: vars, SegmentSlug: e.SegmentSlug,
	})
	if err != nil {
		return Experiment{}, err
	}
	return toExperiment(row)
}

func (r *repo) Update(ctx context.Context, e Experiment) (bool, error) {
	vars, err := json.Marshal(e.Variations)
	if err != nil {
		return false, err
	}
	n, err := r.q.UpdateExperiment(ctx, db.UpdateExperimentParams{
		Name: e.Name, Status: e.Status, Coverage: int32(e.Coverage),
		GoalEvent: e.GoalEvent, Variations: vars, SegmentSlug: e.SegmentSlug, ID: e.ID,
	})
	return n > 0, err
}

func (r *repo) Get(ctx context.Context, id uuid.UUID) (Experiment, bool, error) {
	row, err := r.q.GetExperiment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Experiment{}, false, nil
	}
	if err != nil {
		return Experiment{}, false, err
	}
	e, err := toExperiment(row)
	return e, err == nil, err
}

func (r *repo) List(ctx context.Context, channelID uuid.UUID) ([]Experiment, error) {
	rows, err := r.q.ListExperiments(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return toExperiments(rows)
}

func (r *repo) ListRunning(ctx context.Context, channelID uuid.UUID) ([]Experiment, error) {
	rows, err := r.q.ListRunningExperiments(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return toExperiments(rows)
}

func (r *repo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteExperiment(ctx, id)
}

func (r *repo) Results(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error) {
	rows, err := r.spine.ExperimentResults(ctx, channelID, key, goalEvent, since)
	if err != nil {
		return nil, err
	}
	out := make([]VariationResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, VariationResult{Variation: row.Variation, Exposed: row.Exposed, Converted: row.Converted, Revenue: row.Revenue})
	}
	return out, nil
}

func (r *repo) GoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error) {
	return r.spine.ExperimentGoalEvents(ctx, channelID, since)
}

func toExperiments(rows []db.Experiment) ([]Experiment, error) {
	out := make([]Experiment, 0, len(rows))
	for _, row := range rows {
		e, err := toExperiment(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func toExperiment(row db.Experiment) (Experiment, error) {
	e := Experiment{
		ID: row.ID, ChannelID: row.ChannelID, Key: row.Key, Name: row.Name,
		Status: row.Status, Coverage: int(row.Coverage), GoalEvent: row.GoalEvent,
		SegmentSlug: row.SegmentSlug, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if len(row.Variations) > 0 {
		if err := json.Unmarshal(row.Variations, &e.Variations); err != nil {
			return Experiment{}, err
		}
	}
	return e, nil
}
