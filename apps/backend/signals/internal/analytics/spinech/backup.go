package spinech

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// The BACKUP_* bucket as ClickHouse's BACKUP statement wants it - the server uploads directly, so the endpoint has to resolve from its container
type BackupDestination struct {
	Endpoint, Bucket, Prefix string // the object prefix the Postgres dumps share
	AccessKey, SecretKey     string
	Keep                     int
}

// The listing and deleting half of pruning, satisfied by objstore.Storage
type BackupObjects interface {
	ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error)
	ListPrefix(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
}

type BackupResult struct {
	Prefix string   // the directory the copy landed under
	Pruned []string // older copies retention removed
}

const backupFamily = "signals-clickhouse-"

// Backup writes a full copy of the spine database under <prefix>signals-clickhouse-<stamp>/ and prunes copies past Keep - ClickHouse
// backups are directories of parts, so pruning removes every object under an old stamp
func (s *Store) Backup(ctx context.Context, dest BackupDestination, stamp string, objects BackupObjects) (BackupResult, error) {
	var db string
	if err := s.conn.QueryRow(ctx, "SELECT currentDatabase()").Scan(&db); err != nil {
		return BackupResult{}, fmt.Errorf("clickhouse backup: %w", err)
	}
	res := BackupResult{Prefix: dest.Prefix + backupFamily + stamp + "/"}
	url := strings.TrimRight(dest.Endpoint, "/") + "/" + dest.Bucket + "/" + res.Prefix
	stmt := fmt.Sprintf("BACKUP DATABASE `%s` TO S3('%s', '%s', '%s') SETTINGS compression_method = 'zstd'",
		strings.ReplaceAll(db, "`", ""), sqlString(url), sqlString(dest.AccessKey), sqlString(dest.SecretKey))
	if err := s.conn.Exec(ctx, stmt); err != nil {
		return BackupResult{}, fmt.Errorf("clickhouse backup: %w", err)
	}
	pruned, err := pruneBackups(ctx, objects, dest.Prefix, dest.Keep)
	if err != nil {
		return res, fmt.Errorf("clickhouse backup: wrote %s but retention pruning failed: %w", res.Prefix, err)
	}
	res.Pruned = pruned
	return res, nil
}

func pruneBackups(ctx context.Context, objects BackupObjects, prefix string, keep int) ([]string, error) {
	if keep < 1 {
		return nil, nil
	}
	dirs, err := objects.ListImmediatePrefixes(ctx, prefix+backupFamily)
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs) // fixed-width UTC stamps: lexicographic = chronological
	if len(dirs) <= keep {
		return nil, nil
	}
	var pruned []string
	for _, dir := range dirs[:len(dirs)-keep] {
		keys, err := objects.ListPrefix(ctx, dir)
		if err != nil {
			return pruned, err
		}
		for _, k := range keys {
			if err := objects.Delete(ctx, k); err != nil {
				return pruned, err
			}
		}
		pruned = append(pruned, dir)
	}
	return pruned, nil
}

func sqlString(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), "'", `\'`)
}

// BackupJob binds the store to its destination, so the maintenance handler makes one call
type BackupJob struct {
	Store   *Store
	Dest    BackupDestination
	Objects BackupObjects
}

func (j *BackupJob) Run(ctx context.Context, stamp string) (BackupResult, error) {
	return j.Store.Backup(ctx, j.Dest, stamp, j.Objects)
}
