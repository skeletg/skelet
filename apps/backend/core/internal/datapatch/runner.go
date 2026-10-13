package datapatch

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Keeps the run lock apart from every other advisory lock in the app - pluginmigrate's class is "skpm"
const advisoryLockClass = 0x736b6470 // "skdp" - skelet data patch

// Result reports what a Run did
type Result struct {
	Applied []string // patch IDs applied this run, in order
	Skipped int      // already-applied patches
}

// Run applies every pending patch in dependency order - Each patch and its data_patches record commit together in one
// transaction, so a failing patch rolls back cleanly and is retried on the next run
func Run(ctx context.Context, queries *db.Queries, txMgr *txrunner.Manager) (Result, error) {
	return run(ctx, queries, txMgr, Registered())
}

func run(ctx context.Context, queries *db.Queries, txMgr *txrunner.Manager, all []Patch) (Result, error) {
	pending, err := pendingOf(ctx, queries, all)
	if err != nil {
		return Result{}, err
	}
	res := Result{Skipped: len(all) - len(pending)}
	for _, p := range pending {
		ran := false
		if err := txMgr.InTransaction(ctx, func(t *sql.Tx) error {
			q := queries.WithTx(t)
			// A second runner that planned the same patch waits here, then finds the marker and moves on
			if err := q.LockDataPatches(ctx, advisoryLockClass); err != nil {
				return fmt.Errorf("lock data patches: %w", err)
			}
			done, err := q.DataPatchApplied(ctx, p.ID)
			if err != nil || done {
				return err
			}
			if err := apply(ctx, p, &Context{Tx: t, Queries: q}); err != nil {
				return err
			}
			ran = true
			return q.InsertDataPatch(ctx, db.InsertDataPatchParams{ID: p.ID, Namespace: p.Namespace})
		}); err != nil {
			return res, fmt.Errorf("apply data patch %q: %w", p.ID, err)
		}
		if ran {
			res.Applied = append(res.Applied, p.ID)
		} else {
			res.Skipped++
		}
	}
	return res, nil
}

func apply(ctx context.Context, p Patch, pc *Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return p.Apply(ctx, pc)
}

// Status returns the applied + pending patch IDs for reporting
func Status(ctx context.Context, queries *db.Queries) (appliedIDs, pendingIDs []string, err error) {
	appliedIDs, err = queries.ListAppliedDataPatches(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load applied patches: %w", err)
	}
	pending, err := plan(Registered(), set(appliedIDs))
	if err != nil {
		return nil, nil, err
	}
	for _, p := range pending {
		pendingIDs = append(pendingIDs, p.ID)
	}
	return appliedIDs, pendingIDs, nil
}

func pendingOf(ctx context.Context, queries *db.Queries, all []Patch) ([]Patch, error) {
	appliedIDs, err := queries.ListAppliedDataPatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("load applied patches: %w", err)
	}
	return plan(all, set(appliedIDs))
}

func set(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}
