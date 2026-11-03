package pluginmigrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"unicode/utf8"
)

// The canonical SHA256-hex of a migration's up SQL - stored at apply time and re-compared on later runs to detect edits to already-applied migrations
func migrationChecksum(m Migration) string {
	sum := sha256.Sum256([]byte(m.UpSQL))
	return hex.EncodeToString(sum[:])
}

// Classid keeps plugin migrations from colliding with any other advisory lock in the app - the pair form leaves the whole 32-bit hash usable for the namespace
const advisoryLockClass = 0x736b706d // "skpm" - skelet plugin migrate

// FNV-1a into int32 because pg_advisory_xact_lock's second arg is 32-bit - must stay stable across processes or two servers derive different locks and stop blocking each other
func advisoryLockID(namespace string) int32 {
	const (
		offset = 2166136261
		prime  = 16777619
	)
	h := uint32(offset)
	for i := 0; i < len(namespace); i++ {
		h ^= uint32(namespace[i])
		h *= prime
	}
	return int32(h)
}

// Blocks until any concurrent runner for the same plugin commits or rolls back - two `plugin-migrate up` runs would otherwise both execute the up SQL, since the (namespace, version) primary key stops the double record but not the double side effect of non-idempotent DDL
// Transaction-scoped, so it releases on commit or rollback with no unlock path to forget
func lockNamespace(ctx context.Context, tx *sql.Tx, namespace string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, $2)`,
		int32(advisoryLockClass), advisoryLockID(namespace),
	); err != nil {
		return fmt.Errorf("lock namespace %s: %w", namespace, err)
	}
	return nil
}

// Applies the given namespace order first, registered ones only, then any remaining namespace in registration order - so manifest-aware callers get dependency safety without listing everything
// Pass plugin.OrderedManifestSlugs(), or namespaces directly when a slug diverges from its namespace
func (r *Runner) UpInOrder(ctx context.Context, order []string) error {
	all := AllRegistrations()
	seen := make(map[string]bool, len(order))
	for _, ns := range order {
		if _, ok := FindRegistration(ns); !ok {
			// Plugin declared a manifest but no migrations - fine, skip
			seen[ns] = true
			continue
		}
		if err := r.UpNamespace(ctx, ns); err != nil {
			return fmt.Errorf("up %s: %w", ns, err)
		}
		seen[ns] = true
	}
	// Apply any remaining registrations not covered by `order`
	for _, reg := range all {
		if seen[reg.Namespace] {
			continue
		}
		if err := r.UpNamespace(ctx, reg.Namespace); err != nil {
			return fmt.Errorf("up %s: %w", reg.Namespace, err)
		}
	}
	return nil
}

// UpNamespace applies every pending migration for one plugin
func (r *Runner) UpNamespace(ctx context.Context, namespace string) error {
	reg, ok := FindRegistration(namespace)
	if !ok {
		return ErrNamespaceNotRegistered
	}
	migrations, err := loadMigrations(reg)
	if err != nil {
		return fmt.Errorf("load migrations for %s: %w", namespace, err)
	}
	applied, err := r.appliedVersions(ctx, namespace)
	if err != nil {
		return fmt.Errorf("read applied versions for %s: %w", namespace, err)
	}
	// Index migrations by version once, used three times below - the applied-row existence check, the already-applied skip, and the out-of-order guard - Replaces an O(N^2) nested loop
	migByVersion := make(map[int]Migration, len(migrations))
	for _, m := range migrations {
		migByVersion[m.Version] = m
	}
	appliedSet := make(map[int]struct{}, len(applied))
	highestApplied := 0
	for _, v := range applied {
		appliedSet[v] = struct{}{}
		if v > highestApplied {
			highestApplied = v
		}
	}
	// Reject "applied N but the file for N is missing" - that's data corruption, not a recoverable state, and the operator fixes it by restoring the file or force-removing the row
	for _, v := range applied {
		if _, ok := migByVersion[v]; !ok {
			return fmt.Errorf("plugin %s has applied version %d but no migration file with that version exists in FS - refusing to proceed", namespace, v)
		}
	}

	// Checksum drift detection - for every applied version with a recorded checksum, rows with none aside, compare against the current file's hash
	// A mismatch means the file was edited after apply, so refuse to proceed rather than silently roll forward against half-applied state
	checksums, err := r.appliedChecksums(ctx, namespace)
	if err != nil {
		return fmt.Errorf("read checksums for %s: %w", namespace, err)
	}
	for v, recorded := range checksums {
		if recorded == "" {
			// A row with no recorded checksum - skip, there's nothing to compare against
			continue
		}
		m, ok := migByVersion[v]
		if !ok {
			// The earlier loop already returns for this case - keep the guard so we never index a missing entry
			continue
		}
		if got := migrationChecksum(m); got != recorded {
			return fmt.Errorf("plugin %s migration %04d_%s checksum mismatch (file=%s, db=%s) - the file was modified after it was applied; revert the change or update plugin_migrations.checksum manually if the edit is verified safe",
				namespace, m.Version, m.Name, got, recorded)
		}
	}

	for _, m := range migrations {
		if _, alreadyApplied := appliedSet[m.Version]; alreadyApplied {
			continue
		}
		if m.Version <= highestApplied {
			// Out-of-order migrations are forbidden, since they'd interleave with already-applied state - usually it means someone rebased a plugin branch and added 0002 after 0003 was deployed
			return fmt.Errorf("plugin %s: cannot apply version %d, higher version %d is already applied", namespace, m.Version, highestApplied)
		}
		if err := r.applyOne(ctx, namespace, m); err != nil {
			return err
		}
	}
	return nil
}

// Rolls back the last `steps` migrations for one plugin, stopping when none remain - ErrNamespaceNotRegistered when the plugin isn't loaded, since operators can't roll back SQL they don't have
func (r *Runner) Down(ctx context.Context, namespace string, steps int) error {
	if steps <= 0 {
		return nil
	}
	reg, ok := FindRegistration(namespace)
	if !ok {
		return ErrNamespaceNotRegistered
	}
	migrations, err := loadMigrations(reg)
	if err != nil {
		return err
	}
	migByVersion := map[int]Migration{}
	for _, m := range migrations {
		migByVersion[m.Version] = m
	}
	applied, err := r.appliedVersions(ctx, namespace)
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.IntSlice(applied)))
	plan := make([]Migration, 0, min(steps, len(applied)))
	for _, v := range applied[:min(steps, len(applied))] {
		m, ok := migByVersion[v]
		if !ok {
			return fmt.Errorf("plugin %s: cannot roll back version %d - file missing from FS", namespace, v)
		}
		if m.DownSQL == "" {
			return fmt.Errorf("plugin %s: migration %d has no .down.sql; refusing to roll back", namespace, v)
		}
		plan = append(plan, m)
	}
	// every step is checked before the first runs, so an uninstall never stops halfway with a mix of rolled-back and live schema
	for _, m := range plan {
		if err := r.rollbackOne(ctx, namespace, m); err != nil {
			return err
		}
	}
	return nil
}

// One row per registered plugin, for `skelet cli plugin-migrate status`
func (r *Runner) Status(ctx context.Context) ([]Status, error) {
	regs := AllRegistrations()
	out := make([]Status, 0, len(regs))
	for _, reg := range regs {
		s := Status{Namespace: reg.Namespace}
		migrations, err := loadMigrations(reg)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", reg.Namespace, err)
		}
		s.HasMigrations = len(migrations) > 0
		for _, m := range migrations {
			s.PendingVersions = append(s.PendingVersions, m.Version)
			if m.Version > s.HighestAvailable {
				s.HighestAvailable = m.Version
			}
		}
		applied, err := r.appliedVersions(ctx, reg.Namespace)
		if err != nil {
			return nil, fmt.Errorf("read applied for %s: %w", reg.Namespace, err)
		}
		s.AppliedVersions = applied
		appliedSet := map[int]struct{}{}
		for _, v := range applied {
			appliedSet[v] = struct{}{}
			if v > s.HighestApplied {
				s.HighestApplied = v
			}
		}
		// Drop applied versions from pending
		pending := s.PendingVersions[:0]
		for _, v := range s.PendingVersions {
			if _, ok := appliedSet[v]; !ok {
				pending = append(pending, v)
			}
		}
		s.PendingVersions = pending
		out = append(out, s)
	}
	return out, nil
}

// Runs a single migration in a transaction and records the version on success
// The whole UpSQL body goes in one Exec, since Postgres' wire protocol parses multiple statements natively - naively splitting on ';' trips over semicolons in comments and DO $$...$$ blocks, and writing a correct SQL tokenizer isn't the runner's job
func (r *Runner) applyOne(ctx context.Context, namespace string, m Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockNamespace(ctx, tx, namespace); err != nil {
		return err
	}
	// Re-check under the lock - a concurrent runner may have applied this version since our pending-list read, and its commit is only visible now
	applied, err := r.versionApplied(ctx, tx, namespace, m.Version)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	if _, err := tx.ExecContext(ctx, m.UpSQL); err != nil {
		return fmt.Errorf("plugin %s migration %04d_%s: %w\nSQL: %s",
			namespace, m.Version, m.Name, err, truncateForLog(m.UpSQL))
	}
	// Boot runs this through the app's pooler, so a session SET in the body must not outlive the commit
	if _, err := tx.ExecContext(ctx, `RESET ALL`); err != nil {
		return fmt.Errorf("reset after %s/%d: %w", namespace, m.Version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO plugin_migrations (namespace, version, checksum) VALUES ($1, $2, $3)`,
		namespace, m.Version, migrationChecksum(m),
	); err != nil {
		return fmt.Errorf("record %s/%d: %w", namespace, m.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s/%d: %w", namespace, m.Version, err)
	}
	return nil
}

func (r *Runner) rollbackOne(ctx context.Context, namespace string, m Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockNamespace(ctx, tx, namespace); err != nil {
		return err
	}
	// a concurrent down may have rolled this version back while we waited on the lock
	if applied, err := r.versionApplied(ctx, tx, namespace, m.Version); err != nil || !applied {
		return err
	}

	if _, err := tx.ExecContext(ctx, m.DownSQL); err != nil {
		return fmt.Errorf("plugin %s rollback %04d_%s: %w\nSQL: %s",
			namespace, m.Version, m.Name, err, truncateForLog(m.DownSQL))
	}
	if _, err := tx.ExecContext(ctx, `RESET ALL`); err != nil {
		return fmt.Errorf("reset after rolling back %s/%d: %w", namespace, m.Version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM plugin_migrations WHERE namespace = $1 AND version = $2`,
		namespace, m.Version,
	); err != nil {
		return fmt.Errorf("unrecord %s/%d: %w", namespace, m.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollback %s/%d: %w", namespace, m.Version, err)
	}
	return nil
}

// Read inside the caller's tx so it sees whatever a concurrent runner committed before releasing the namespace lock
func (r *Runner) versionApplied(ctx context.Context, tx *sql.Tx, namespace string, version int) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM plugin_migrations WHERE namespace = $1 AND version = $2)`,
		namespace, version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("re-check %s/%d: %w", namespace, version, err)
	}
	return exists, nil
}

func (r *Runner) appliedVersions(ctx context.Context, namespace string) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT version FROM plugin_migrations WHERE namespace = $1 ORDER BY version`,
		namespace,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// version to checksum for the namespace - a NULL checksum maps to the empty string, and the caller skips verification for those so upgrade-in-place keeps working
func (r *Runner) appliedChecksums(ctx context.Context, namespace string) (map[int]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT version, checksum FROM plugin_migrations WHERE namespace = $1`,
		namespace,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]string)
	for rows.Next() {
		var v int
		var cs sql.NullString
		if err := rows.Scan(&v, &cs); err != nil {
			return nil, err
		}
		out[v] = cs.String
	}
	return out, rows.Err()
}

// Clips a long SQL statement for error messages - multi-line bodies usually give a useful error from the first hundred chars
func truncateForLog(s string) string {
	const limit = 240
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…(truncated)"
}
