// Package txrunner is the shared transaction runner - keeping it neutral (owned by neither cart nor order) lets any service span multiple repositories
package txrunner

import (
	"context"
	"database/sql"
	"fmt"
)

type Manager struct {
	db *sql.DB
}

func New(db *sql.DB) *Manager { return &Manager{db: db} }

// Hand the *sql.Tx to each participating repo via its WithTx
func (m *Manager) InTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = t.Rollback()
		}
	}()
	if err := fn(t); err != nil {
		return err
	}
	if err := t.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return nil
}
