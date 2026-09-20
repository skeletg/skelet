// Package backup streams pg_dump into dedicated S3-compatible object storage, so nothing touches local disk
// Destination, credentials and retention come from BACKUP_* environment variables only, and the image needs a pg_dump at or above the server major
package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// Family names which database a dump is of - each process backs up its own, and the two are pruned
// as separate families so one cannot halve the other's retention window
type Family string

const (
	FamilyCommerce Family = "skelet"
	FamilySignals  Family = "skelet-signals"
)

type Service struct {
	cfg    envConfig
	dsn    string
	family Family
}

// New is one database's backup. The DSN should be DIRECT, since pg_dump holds a long session and
// must never point at a transaction-pooling PgBouncer alias
func New(family Family, dsn string) *Service {
	return &Service{cfg: loadEnv(), dsn: dsn, family: family}
}

func (s *Service) Enabled() bool { // setting BACKUP_S3_BUCKET is the switch
	return s.cfg.Bucket != "" && s.dsn != ""
}

func (s *Service) Encrypted() bool { // surfaced so boot can warn when dumps aren't encrypted
	return s.cfg.Passphrase != ""
}

type Result struct {
	Key    string
	Bytes  int64
	Pruned []string
}

// Stamp is the timestamp a run writes into its object key - the commerce job mints one and hands it
// to the suite, so the two dumps of one night pair up on restore
func Stamp(at time.Time) string { return at.UTC().Format("20060102-150405") }

// Run is one backup of this database: stream pg_dump to the bucket under the stamp, then prune this
// family past BACKUP_KEEP. The heartbeat is the caller's, once every half of the night has landed
func (s *Service) Run(ctx context.Context, at time.Time) (Result, error) {
	store, err := s.storage(ctx)
	if err != nil {
		return Result{}, err
	}
	if s.dsn == "" {
		return Result{}, fmt.Errorf("backup: no database to dump - the direct DSN is unset")
	}
	suffix := ".dump"
	if s.cfg.Passphrase != "" {
		suffix += encSuffix
	}
	res := Result{Key: s.cfg.Prefix + string(s.family) + "-" + Stamp(at) + suffix}
	res.Bytes, err = s.dump(ctx, store, res.Key, s.dsn)
	if err != nil {
		return Result{}, err
	}
	pruned, err := s.prune(ctx, store)
	if err != nil {
		return res, fmt.Errorf("backup: uploaded %s but retention pruning failed: %w", res.Key, err) // the backup itself succeeded, so the retention failure can't discard that fact
	}
	res.Pruned = pruned
	return res, nil
}

// Heartbeat pings only when every dump of the night succeeded, so a monitor watching for the pings to stop sees every failure mode
// A failed ping is a warning, not a failed backup - the dump is already in the bucket
func (s *Service) Heartbeat(ctx context.Context) {
	if s.cfg.HeartbeatURL == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.HeartbeatURL, nil)
	if err != nil {
		slog.Warn("backup: heartbeat URL is unusable", "err", err)
		return
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		slog.Warn("backup: heartbeat ping failed (the backup itself succeeded)", "err", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused rather than aborted
	if resp.StatusCode >= 300 {
		slog.Warn("backup: heartbeat ping rejected (the backup itself succeeded)", "status", resp.StatusCode)
	}
}

func (s *Service) dump(ctx context.Context, store *objstore.Storage, key, dsn string) (int64, error) {
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--compress=zstd", "--dbname", dsn) // zstd over the default zlib is faster AND tighter, which is the backup window at scale - pg_restore reads it transparently, both ends being PG16+ clients in the image
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	uploadErr, runErr := streamCommand(cmd, func(r io.Reader) error {
		if s.cfg.Passphrase != "" {
			enc := encryptStream(s.cfg.Passphrase, r)
			err := store.UploadStream(ctx, key, "application/octet-stream", enc)
			_ = enc.Close() // unblocks the encryption pump when the upload died before draining it
			return err
		}
		return store.UploadStream(ctx, key, "application/octet-stream", r)
	})
	if runErr != nil || uploadErr != nil { // a pg_dump that dies mid-stream EOFs the pipe and the upload happily finalizes a TRUNCATED object that looks complete - delete it and fail loudly instead
		_ = store.Delete(ctx, key)
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return 0, fmt.Errorf("backup: pg_dump=%w upload=%w stderr=%q (pg_dump missing? the image needs postgresql18-client)", runErr, uploadErr, msg)
	}

	size, _, exists, err := store.Head(ctx, key)
	if err != nil || !exists || size == 0 {
		_ = store.Delete(ctx, key)
		return 0, fmt.Errorf("backup: uploaded object missing or empty (size=%d, err=%w)", size, err)
	}
	return size, nil
}

// Restore is destructive (--clean --if-exists), so the CLI gates it behind a confirmation flag, and an .age dump decrypts in-flight where age authenticates it
// jobs > 1 restores data and indexes in parallel, which the spooled temp file is what makes possible
func (s *Service) Restore(ctx context.Context, key string, jobs int) error {
	if got := FamilyOf(key); got != s.family {
		return fmt.Errorf("backup: %s is a %s dump, and this process holds the %s database", key, got, s.family)
	}
	if s.dsn == "" {
		return fmt.Errorf("backup: no database to restore into - the direct DSN is unset")
	}
	store, err := s.storage(ctx)
	if err != nil {
		return err
	}
	dsn := s.dsn
	body, err := store.GetStream(ctx, key)
	if err != nil {
		return fmt.Errorf("backup: fetch %s: %w", key, err)
	}
	defer body.Close()

	var src io.Reader = body
	if strings.HasSuffix(key, encSuffix) {
		if s.cfg.Passphrase == "" {
			return fmt.Errorf("backup: %s is encrypted - set BACKUP_ENCRYPTION_PASSPHRASE", key)
		}
		if src, err = decryptStream(s.cfg.Passphrase, body); err != nil {
			return err
		}
	}

	// Spooled to a temp file rather than piped - pg_dump wrote to a pipe so the archive has no data offsets, and pg_restore scanning such an archive from stdin errors at EOF with no way to tell a clean finish from missing blocks, while a seekable file restores exactly; it costs one dump of temp disk during a rare manual restore, so point TMPDIR at a volume when the dump outgrows the container filesystem
	tmp, err := os.CreateTemp("", "skelet-restore-*.dump")
	if err != nil {
		return fmt.Errorf("backup: temp spool: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := io.Copy(tmp, src); err != nil {
		return fmt.Errorf("backup: spool %s: %w", key, err)
	}

	args := []string{"--clean", "--if-exists", "--no-owner"}
	if jobs > 1 {
		args = append(args, fmt.Sprintf("--jobs=%d", jobs))
	}
	args = append(args, "--dbname", dsn, tmp.Name())
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 800 {
			msg = msg[len(msg)-800:]
		}
		return fmt.Errorf("backup: pg_restore %s: %w stderr=%q", key, err, msg)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]string, error) { // the backup object keys under the configured prefix, oldest first
	store, err := s.storage(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := store.ListPrefix(ctx, s.cfg.Prefix)
	if err != nil {
		return nil, err
	}
	sort.Strings(keys) // fixed-width UTC timestamps: lexicographic = chronological
	return keys, nil
}

func (s *Service) storage(ctx context.Context) (*objstore.Storage, error) {
	if s.cfg.Bucket == "" {
		return nil, fmt.Errorf("backup: not configured - set the BACKUP_S3_* environment variables (destination is deliberately not admin config)")
	}
	store, err := objstore.New(ctx, objstore.Config{
		Bucket:           s.cfg.Bucket,
		Region:           s.cfg.Region,
		InternalEndpoint: s.cfg.Endpoint,
		UsePathStyle:     s.cfg.PathStyle,
		AccessKeyID:      s.cfg.AccessKey,
		SecretAccessKey:  s.cfg.SecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("backup: init storage: %w", err)
	}
	return store, nil
}

func isDump(key string) bool {
	return strings.HasSuffix(key, ".dump") || strings.HasSuffix(key, ".dump"+encSuffix)
}

func (s *Service) prune(ctx context.Context, store *objstore.Storage) ([]string, error) { // keeps the newest `keep` dumps of THIS family - counted together, signals dumps would halve the commerce retention window
	keys, err := store.ListPrefix(ctx, s.cfg.Prefix)
	if err != nil {
		return nil, err
	}
	var mine []string
	for _, k := range keys {
		if isDump(k) && FamilyOf(k) == s.family {
			mine = append(mine, k)
		}
	}
	sort.Strings(mine) // fixed-width UTC timestamps: lexicographic = chronological
	if len(mine) <= s.cfg.Keep {
		return nil, nil
	}
	var pruned []string
	for _, old := range mine[:len(mine)-s.cfg.Keep] {
		if err := store.Delete(ctx, old); err != nil {
			return pruned, err
		}
		pruned = append(pruned, old)
	}
	return pruned, nil
}

// FamilyOf reads which database a key is a dump of, from the name Run() gave it
func FamilyOf(key string) Family {
	name := key
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if strings.HasPrefix(name, string(FamilySignals)+"-") {
		return FamilySignals
	}
	return FamilyCommerce
}

// KeyTimestamp reads back the stamp Run() writes into an object key. It lives here so the parse can't drift from the format above, and it
// tolerates both families (commerce "skelet-<stamp>" and "skelet-signals-<stamp>") plus either suffix
func KeyTimestamp(key string) (time.Time, bool) {
	name := key
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimPrefix(name, "skelet-")
	name = strings.TrimPrefix(name, "signals-")
	name = strings.TrimSuffix(name, encSuffix)
	name = strings.TrimSuffix(name, ".dump")
	at, err := time.Parse("20060102-150405", name)
	if err != nil {
		return time.Time{}, false
	}
	return at.UTC(), true
}
