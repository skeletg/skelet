package signalsboot

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

// Settings where the main Lens window can't be read
type lensSettings struct{ broken string }

func (s lensSettings) Int(_ context.Context, path string, _ *uuid.UUID) (int64, error) {
	if path == s.broken {
		return 0, errors.New("settings store unreachable")
	}
	return 1, nil
}

func (lensSettings) String(context.Context, string, *uuid.UUID) (string, error) { return "prune", nil }

// Replay is the most sensitive data on the shortest window, so a step failing ahead of it must not keep recordings past theirs
func TestAFailingLensStepStillPrunesReplay(t *testing.T) {
	conn := testdb.RequireSignals(t)
	sid := uuid.New()
	old := time.Now().AddDate(0, 0, -10)
	if _, err := conn.Exec(`INSERT INTO lens_sessions (id, channel_id, anon_id, started_at, last_event_at) VALUES ($1, $2, 'a', $3, $3)`, sid, uuid.New(), old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM lens_sessions WHERE id = $1`, sid) })
	svc := lens.NewService(signalsdb.New(conn), nil, nil, nil)

	err := LensPruneJob(lensSettings{broken: "signals/lens/retention_days"}, svc)(context.Background())
	if err == nil {
		t.Error("the failed step went unreported")
	}
	var left int
	if err := conn.QueryRow(`SELECT count(*) FROM lens_sessions WHERE id = $1`, sid).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Error("a recording outlived its window because an earlier step failed")
	}
}

// The fallbacks only apply when a read fails, and they must be the registry's own defaults
func TestLifecycleDefaultsMatchTheRegistry(t *testing.T) {
	want := map[string]int{
		"signals/synapse/lifecycle_new_days":     analytics.DefaultLifecycleWindows.NewDays,
		"signals/synapse/lifecycle_active_days":  analytics.DefaultLifecycleWindows.ActiveDays,
		"signals/synapse/lifecycle_at_risk_days": analytics.DefaultLifecycleWindows.AtRiskDays,
	}
	for _, d := range config.AllDefinitions() {
		if n, ok := want[d.Path]; ok {
			if d.Default != strconv.Itoa(n) {
				t.Errorf("%s: registry default %q, code fallback %d", d.Path, d.Default, n)
			}
			delete(want, d.Path)
		}
	}
	if len(want) != 0 {
		t.Errorf("not registered: %v", want)
	}
}
