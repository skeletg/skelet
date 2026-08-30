package datapatch

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/tx"
)

// Result reports what a Run did
type Result struct {
	Applied []string // patch IDs applied this run, in order
	Skipped int      // already-applied patches
}

// Run applies every pending patch in dependency order - Each patch and its data_patches record commit together in one
// transaction, so a failing patch rolls back cleanly and is retried on the next run
func Run(ctx context.Context, queries *db.Queries, txMgr *tx.Manager) (Result, error) {
	appliedIDs, err := queries.ListAppliedDataPatches(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load applied patches: %w", err)
	}
	applied := make(map[string]struct{}, len(appliedIDs))
	for _, id := range appliedIDs {
		applied[id] = struct{}{}
	}

	all := Registered()
	pending, err := plan(all, applied)
	if err != nil {
		return Result{}, err
	}

	res := Result{Skipped: len(all) - len(pending)}
	for _, p := range pending {
		if err := txMgr.InTransaction(ctx, func(t *sql.Tx) error {
			q := queries.WithTx(t)
			if err := p.Apply(ctx, &Context{Tx: t, Queries: q}); err != nil {
				return err
			}
			return q.InsertDataPatch(ctx, db.InsertDataPatchParams{ID: p.ID, Namespace: p.Namespace})
		}); err != nil {
			return res, fmt.Errorf("apply data patch %q: %w", p.ID, err)
		}
		res.Applied = append(res.Applied, p.ID)
	}
	return res, nil
}

// Status returns the applied + pending patch IDs for reporting
func Status(ctx context.Context, queries *db.Queries) (appliedIDs, pendingIDs []string, err error) {
	appliedIDs, err = queries.ListAppliedDataPatches(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load applied patches: %w", err)
	}
	applied := make(map[string]struct{}, len(appliedIDs))
	for _, id := range appliedIDs {
		applied[id] = struct{}{}
	}
	pending, err := plan(Registered(), applied)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range pending {
		pendingIDs = append(pendingIDs, p.ID)
	}
	return appliedIDs, pendingIDs, nil
}
