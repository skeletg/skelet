package signalsboot

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

var errErasureQueue = errors.New("erasure queue unreadable")

type brokenDB struct{}

func (brokenDB) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errErasureQueue
}
func (brokenDB) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errErasureQueue
}
func (brokenDB) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errErasureQueue
}
func (brokenDB) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

type prunedSpine struct {
	analytics.SpineStore
	pruned int
}

func (s *prunedSpine) PruneIdentities(context.Context, time.Time) (int64, error) { return 0, nil }
func (s *prunedSpine) PruneEvents(context.Context, time.Time) (int64, error) {
	s.pruned++
	return 0, nil
}

type spineRetention struct{}

func (spineRetention) Int(_ context.Context, path string, _ *uuid.UUID) (int64, error) {
	if path == "signals/analytics/spine_retention_days" {
		return 30, nil
	}
	return 0, nil
}
func (spineRetention) String(context.Context, string, *uuid.UUID) (string, error) {
	return "prune", nil
}

func TestSpinePruneReportsAFailedErasureSweepAfterStillPruning(t *testing.T) {
	spine := &prunedSpine{}
	err := SpinePruneJob(spineRetention{}, analytics.NewRecorder(signalsdb.New(brokenDB{}), spine))(context.Background())
	if !errors.Is(err, errErasureQueue) {
		t.Errorf("job err = %v, want the erasure sweep's failure", err)
	}
	if spine.pruned != 1 {
		t.Errorf("retention ran %d times, want once despite the failed sweep", spine.pruned)
	}
}
