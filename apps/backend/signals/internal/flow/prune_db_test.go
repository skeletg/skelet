package flow

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestPruneFinishedRunsKeepsLiveAndRecentRuns(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}
	var flowID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO flows (channel_id, name, trigger_type, status) VALUES ($1, 'prune-test', 'password_reset', 'active') RETURNING id`,
		uuid.New()).Scan(&flowID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM flows WHERE id = $1`, flowID) })

	seed := func(status string, age time.Duration) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(
			`INSERT INTO flow_runs (flow_id, customer_id, current_node_id, status, updated_at) VALUES ($1, $2, 'n', $3, NOW() - make_interval(secs => $4)) RETURNING id`,
			flowID, uuid.New(), status, age.Seconds()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldDone, oldFailed := seed(RunCompleted, 400*24*time.Hour), seed(RunFailed, 400*24*time.Hour)
	oldLive, recentDone := seed(RunActive, 400*24*time.Hour), seed(RunCompleted, time.Hour)

	n, err := r.PruneFinishedRuns(ctx, time.Now().Add(-365*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	exists := func(id uuid.UUID) bool {
		var ok bool
		_ = conn.QueryRow(`SELECT EXISTS (SELECT 1 FROM flow_runs WHERE id = $1)`, id).Scan(&ok)
		return ok
	}
	if n != 2 || exists(oldDone) || exists(oldFailed) {
		t.Errorf("pruned %d, old finished runs still there: %v %v - want both gone", n, exists(oldDone), exists(oldFailed))
	}
	if !exists(oldLive) || !exists(recentDone) {
		t.Errorf("live run kept=%v, recent finished run kept=%v - want both kept", exists(oldLive), exists(recentDone))
	}
}
