package flow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Repository is the persistence seam for flows + their runs
type Repository interface {
	Create(ctx context.Context, f Flow) (Flow, error)
	Get(ctx context.Context, id uuid.UUID) (Flow, bool, error)
	List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Flow, error)
	ListActiveByTrigger(ctx context.Context, channelID uuid.UUID, trigger string) ([]Flow, error)
	Update(ctx context.Context, f Flow) (bool, error)
	SetStatus(ctx context.Context, id uuid.UUID, status string) (bool, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Restore(ctx context.Context, id uuid.UUID) error

	CountLiveRuns(ctx context.Context, flowID, customerID uuid.UUID) (int64, error)
	CountLiveGuestRuns(ctx context.Context, flowID uuid.UUID, email string) (int64, error)
	HasRunForKey(ctx context.Context, flowID uuid.UUID, key string) (bool, error)
	// ErrMomentTaken when the run's EnrollmentKey already names a run in the flow, ErrAlreadyInFlow when the customer holds a live one
	CreateRun(ctx context.Context, r FlowRun) (FlowRun, error)
	ClaimRunnable(ctx context.Context, limit int32, lease time.Duration) ([]FlowRun, error)
	UpdateRun(ctx context.Context, r FlowRun) error
	ListRuns(ctx context.Context, flowID uuid.UUID, limit int32) ([]FlowRun, error)
	RunsForCustomer(ctx context.Context, customerID uuid.UUID) ([]CustomerRun, error)
	PruneFinishedRuns(ctx context.Context, before time.Time) (int64, error)
}

type repo struct{ q *db.Queries }

// NewRepository wires the sqlc-backed repository
func NewRepository(q *db.Queries) Repository { return &repo{q: q} }

func (r *repo) Create(ctx context.Context, f Flow) (Flow, error) {
	graph, err := json.Marshal(f.Graph)
	if err != nil {
		return Flow{}, err
	}
	tc := f.TriggerConfig
	if len(tc) == 0 {
		tc = json.RawMessage("{}")
	}
	row, err := r.q.CreateFlow(ctx, db.CreateFlowParams{
		ChannelID:     f.ChannelID,
		Name:          f.Name,
		TriggerType:   f.TriggerType,
		TriggerConfig: tc,
		Graph:         graph,
	})
	if err != nil {
		return Flow{}, err
	}
	return toFlow(row)
}

func (r *repo) Get(ctx context.Context, id uuid.UUID) (Flow, bool, error) {
	row, err := r.q.GetFlow(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Flow{}, false, nil
	}
	if err != nil {
		return Flow{}, false, err
	}
	f, err := toFlow(row)
	return f, err == nil, err
}

func (r *repo) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Flow, error) {
	rows, err := r.q.ListFlows(ctx, db.ListFlowsParams{
		ChannelID:      channelID,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return nil, err
	}
	return toFlows(rows)
}

func (r *repo) ListActiveByTrigger(ctx context.Context, channelID uuid.UUID, trigger string) ([]Flow, error) {
	rows, err := r.q.ListActiveFlowsByTrigger(ctx, db.ListActiveFlowsByTriggerParams{ChannelID: channelID, TriggerType: trigger})
	if err != nil {
		return nil, err
	}
	return toFlows(rows)
}

func (r *repo) Update(ctx context.Context, f Flow) (bool, error) {
	graph, err := json.Marshal(f.Graph)
	if err != nil {
		return false, err
	}
	tc := f.TriggerConfig
	if len(tc) == 0 {
		tc = json.RawMessage("{}")
	}
	n, err := r.q.UpdateFlow(ctx, db.UpdateFlowParams{
		Name:          f.Name,
		TriggerType:   f.TriggerType,
		TriggerConfig: tc,
		Graph:         graph,
		ID:            f.ID,
	})
	return n > 0, err
}

func (r *repo) SetStatus(ctx context.Context, id uuid.UUID, status string) (bool, error) {
	n, err := r.q.SetFlowStatus(ctx, db.SetFlowStatusParams{Status: status, ID: id})
	return n > 0, err
}

// SoftDelete marks the row deleted: flow_runs keep pointing at a real flow, and the
// enrollment query skips deleted rows so nobody new enters it
func (r *repo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.SoftDeleteFlow(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("flow not found")
	}
	return nil
}

func (r *repo) Restore(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.RestoreFlow(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("flow not found")
	}
	return nil
}

func (r *repo) CountLiveRuns(ctx context.Context, flowID, customerID uuid.UUID) (int64, error) {
	return r.q.CountLiveRunsForCustomer(ctx, db.CountLiveRunsForCustomerParams{FlowID: flowID, CustomerID: uuid.NullUUID{UUID: customerID, Valid: true}})
}

func (r *repo) CountLiveGuestRuns(ctx context.Context, flowID uuid.UUID, email string) (int64, error) {
	return r.q.CountLiveGuestRuns(ctx, db.CountLiveGuestRunsParams{FlowID: flowID, Email: email})
}

func (r *repo) HasRunForKey(ctx context.Context, flowID uuid.UUID, key string) (bool, error) {
	return r.q.FlowRunExistsForKey(ctx, db.FlowRunExistsForKeyParams{FlowID: flowID, EnrollmentKey: key})
}

func (r *repo) CreateRun(ctx context.Context, rn FlowRun) (FlowRun, error) {
	ctxBytes := rn.Context
	if len(ctxBytes) == 0 {
		ctxBytes = json.RawMessage("{}")
	}
	row, err := r.q.CreateFlowRun(ctx, db.CreateFlowRunParams{
		FlowID:        rn.FlowID,
		CustomerID:    uuid.NullUUID{UUID: rn.CustomerID, Valid: rn.CustomerID != uuid.Nil},
		Email:         rn.Email,
		CurrentNodeID: rn.CurrentNodeID,
		Status:        rn.Status,
		ResumeAt:      nullTime(rn.ResumeAt),
		Context:       ctxBytes,
		EnrollmentKey: rn.EnrollmentKey,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return FlowRun{}, ErrMomentTaken // the keyed insert hit its unique index and inserted nothing
	}
	if isLiveRunConflict(err) {
		return FlowRun{}, ErrAlreadyInFlow
	}
	if err != nil {
		return FlowRun{}, err
	}
	return toRun(row), nil
}

func (r *repo) ClaimRunnable(ctx context.Context, limit int32, lease time.Duration) ([]FlowRun, error) {
	rows, err := r.q.ClaimRunnableFlowRuns(ctx, db.ClaimRunnableFlowRunsParams{Lim: limit, LeaseSeconds: lease.Seconds()})
	if err != nil {
		return nil, err
	}
	out := make([]FlowRun, len(rows))
	for i, row := range rows {
		out[i] = toRun(row)
	}
	return out, nil
}

func (r *repo) UpdateRun(ctx context.Context, rn FlowRun) error {
	ctxBytes := rn.Context
	if len(ctxBytes) == 0 {
		ctxBytes = json.RawMessage("{}")
	}
	return r.q.UpdateFlowRun(ctx, db.UpdateFlowRunParams{
		CurrentNodeID: rn.CurrentNodeID,
		Status:        rn.Status,
		ResumeAt:      nullTime(rn.ResumeAt),
		Context:       ctxBytes,
		Error:         rn.Error,
		SendAttempts:  int32(rn.SendAttempts),
		ID:            rn.ID,
	})
}

func (r *repo) ListRuns(ctx context.Context, flowID uuid.UUID, limit int32) ([]FlowRun, error) {
	rows, err := r.q.ListFlowRunsByFlow(ctx, db.ListFlowRunsByFlowParams{FlowID: flowID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]FlowRun, len(rows))
	for i, row := range rows {
		out[i] = toRun(row)
	}
	return out, nil
}

func (r *repo) RunsForCustomer(ctx context.Context, customerID uuid.UUID) ([]CustomerRun, error) {
	rows, err := r.q.ListFlowRunsForCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]CustomerRun, len(rows))
	for i, row := range rows {
		out[i] = CustomerRun{
			RunID:     row.ID,
			FlowID:    row.FlowID,
			FlowName:  row.FlowName,
			Trigger:   row.TriggerType,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

// --- mapping ---

func toFlows(rows []db.Flow) ([]Flow, error) {
	out := make([]Flow, 0, len(rows))
	for _, row := range rows {
		f, err := toFlow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func toFlow(row db.Flow) (Flow, error) {
	f := Flow{
		ID:            row.ID,
		ChannelID:     row.ChannelID,
		Name:          row.Name,
		TriggerType:   row.TriggerType,
		TriggerConfig: json.RawMessage(row.TriggerConfig),
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		DeletedAt:     nullTimePtr(row.DeletedAt),
	}
	if len(row.Graph) > 0 {
		if err := json.Unmarshal(row.Graph, &f.Graph); err != nil {
			return Flow{}, err
		}
	}
	return f, nil
}

func toRun(row db.FlowRun) FlowRun {
	r := FlowRun{
		ID:            row.ID,
		FlowID:        row.FlowID,
		Email:         row.Email,
		CurrentNodeID: row.CurrentNodeID,
		Status:        row.Status,
		Context:       json.RawMessage(row.Context),
		Error:         row.Error,
		EnrollmentKey: row.EnrollmentKey,
		SendAttempts:  int(row.SendAttempts),
	}
	if row.CustomerID.Valid {
		r.CustomerID = row.CustomerID.UUID
	}
	if row.ResumeAt.Valid {
		t := row.ResumeAt.Time
		r.ResumeAt = &t
	}
	return r
}

func isLiveRunConflict(err error) bool {
	var constraint string
	var pgErr *pgconn.PgError
	var pqErr *pq.Error
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		constraint = pgErr.ConstraintName
	case errors.As(err, &pqErr) && pqErr.Code == "23505":
		constraint = pqErr.Constraint
	default:
		return false
	}
	return constraint == "flow_runs_one_live" || constraint == "flow_runs_one_live_guest"
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullTimePtr converts a nullable SQL timestamp into the *time.Time the
// domain structs use
func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func (r *repo) PruneFinishedRuns(ctx context.Context, before time.Time) (int64, error) {
	return r.q.PruneFinishedFlowRuns(ctx, before)
}
