// Package migrate applies dbmate migrations as a process boots, in dbmate's own schema_migrations so dbmate still reads the same history
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

const lockClass = 0x736b6d67

func Postgres(ctx context.Context, db *sql.DB, stream string, files fs.FS) ([]string, error) {
	all, err := migfile.ListFS(files)
	if err != nil {
		return nil, fmt.Errorf("read the %s migrations: %w", stream, err)
	}
	txm := txrunner.New(db)
	if err := locked(ctx, txm, stream, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version varchar NOT NULL PRIMARY KEY);
			ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum text`)
		return err
	}); err != nil {
		return nil, fmt.Errorf("prepare the %s schema_migrations: %w", stream, err)
	}
	applied, err := recorded(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("read the %s schema_migrations: %w", stream, err)
	}

	var done []string
	for _, f := range all {
		name, sum := filepath.Base(f.Path), checksum(f)
		if was, ok := applied[f.Version]; ok {
			if was.Valid && was.String != sum {
				return done, fmt.Errorf("%s changed after it was applied - restore the file, or roll it back and boot again to re-apply it", name)
			}
			continue
		}
		if !f.HasSQL() {
			slog.Warn("migration skipped until its up half has SQL", "stream", stream, "file", name)
			continue
		}
		if f.NoTransaction {
			return done, fmt.Errorf("%s: transaction:false isn't supported - the transaction holds the lock that keeps booting processes apart", name)
		}
		ran := false
		if err := locked(ctx, txm, stream, func(tx *sql.Tx) error {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, f.Version).Scan(&exists); err != nil || exists {
				return err
			}
			if _, err := tx.ExecContext(ctx, f.Up()); err != nil {
				return err
			}
			// A dump-style file opens with session SETs - search_path, row_security - and none may outlive the commit on a pooled server connection
			if _, err := tx.ExecContext(ctx, `RESET ALL`); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, f.Version, sum); err != nil {
				return err
			}
			ran = true
			return nil
		}); err != nil {
			return done, fmt.Errorf("apply %s: %w", name, err)
		}
		if ran {
			done = append(done, f.Version)
		}
	}
	return done, nil
}

func WithLock(ctx context.Context, db *sql.DB, stream string, fn func(context.Context) error) error {
	return locked(ctx, txrunner.New(db), stream, func(*sql.Tx) error { return fn(ctx) })
}

func locked(ctx context.Context, txm *txrunner.Manager, stream string, fn func(*sql.Tx) error) error {
	return txm.InTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, int32(lockClass), lockID(stream)); err != nil {
			return fmt.Errorf("lock %s: %w", stream, err)
		}
		return fn(tx)
	})
}

func lockID(stream string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(stream))
	return int32(h.Sum32())
}

func checksum(f migfile.File) string {
	sum := sha256.Sum256([]byte(f.Up()))
	return hex.EncodeToString(sum[:])
}

func recorded(ctx context.Context, db *sql.DB) (map[string]sql.NullString, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]sql.NullString{}
	for rows.Next() {
		var v string
		var sum sql.NullString
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, err
		}
		out[v] = sum
	}
	return out, rows.Err()
}
