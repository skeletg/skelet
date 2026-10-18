// Package lens is web behaviour analytics - the second product on the analytics spine after Pulse
// Page-views and custom events land on the append-only spine (source='web') while the high-frequency signals (clicks, cursor-moves, scroll) bin into bounded aggregate tables - Writes are best-effort and the package owns no transactional state
package lens

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Registered in register.go under pulse/lens
const (
	cfgEnabled            = "signals/lens/enabled"
	cfgSamplingRate       = "signals/lens/sampling_rate"
	cfgCaptureMoves       = "signals/lens/capture_moves"
	cfgReplayEnabled      = "signals/lens/replay_enabled"
	cfgReplaySamplingRate = "signals/lens/replay_sampling_rate"
	cfgReplayMaxMinutes   = "signals/lens/replay_max_minutes"
)

// Object storage for the large gzip blobs, replay chunks and DOM snapshots - the rows keep only the key - satisfied by *objstore.Storage
type BlobStore interface {
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// Bounds one /t/rec flush's payload so a runaway recording can't blow up a chunk row - oversized flushes are dropped whole
const MaxReplayChunkBytes = 5 << 20 // 5 MiB

// Heatmap grid resolution in CSS px at a viewport bucket's fixed render width - document coordinates floor into cells of this size
const cellSize = 10

// Reserved custom-event names the storefront emits for UX signals - rage and dead clicks, JS errors, web vitals - routed out of the generic interaction path
const (
	sigRageClick = "$rage_click"
	sigDeadClick = "$dead_click"
	sigJSError   = "$js_error"
	sigWebVitals = "$web_vitals"
	sigSearch    = "$search"
)

// q is the primary read-write pool taking ingest upserts, snapshots and prunes; qr is an optional replica or bulkheaded pool used only by read aggregations, never for writes
type Service struct {
	q      *signalsdb.Queries // signals db: lens_* + browse_seen/browse_notified (ingest + reads)
	qr     *signalsdb.Queries // optional signals read pool for the aggregations
	rec    *analytics.Recorder
	cfg    Settings
	rollup *analytics.TrafficRollup
	blobs  BlobStore // nil only on the CLI and test paths, where ingest drops blobs instead
}

// Settings is the slice of the store's settings Lens reads, declared here so the package depends on the shape rather than on whoever holds them
// A nil channel asks for the store-wide value, and a nil Settings falls back to permissive defaults
type Settings interface {
	Int(ctx context.Context, path string, channelID *uuid.UUID) (int64, error)
	Bool(ctx context.Context, path string, channelID *uuid.UUID) (bool, error)
	String(ctx context.Context, path string, channelID *uuid.UUID) (string, error)
}

func NewService(q *signalsdb.Queries, rec *analytics.Recorder, cfg Settings, rollup *analytics.TrafficRollup) *Service {
	return &Service{q: q, rec: rec, cfg: cfg, rollup: rollup}
}

// Lens retention edge, zero when retention is off or unreadable
func (s *Service) retentionEdge(ctx context.Context) time.Time {
	if s.cfg == nil {
		return time.Time{}
	}
	days, err := s.cfg.Int(ctx, "signals/lens/retention_days", nil)
	if err != nil || days <= 0 {
		return time.Time{}
	}
	return time.Now().UTC().AddDate(0, 0, -int(days))
}

// Optional - when unset, reads use the primary pool
func (s *Service) SetReadQueries(qr *signalsdb.Queries) { s.qr = qr }

func (s *Service) SetBlobStore(b BlobStore) { s.blobs = b }

// Object keys carry a random part, since the bucket can be served publicly and a key built only from a session, a channel or a
// page could be guessed - each row stores its key, which is what every read, prune and retirement goes by
// The snapshot's identity is hashed into one path-safe segment, since page_key contains slashes
func chunkBlobKey(sessionID uuid.UUID, seq int32) string {
	return fmt.Sprintf("lens/replay/%s/%d-%s.gz", sessionID, seq, blobNonce())
}
func snapshotBlobKey(channelID uuid.UUID, pageKey, viewport, state, layoutHash string) string {
	sum := sha256.Sum256([]byte(pageKey + "|" + viewport + "|" + state + "|" + layoutHash))
	return fmt.Sprintf("lens/snapshot/%s/%s-%s.gz", channelID, hex.EncodeToString(sum[:16]), blobNonce())
}

func blobNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Read-only aggregations only - ingest, snapshot and prune always use s.q directly
func (s *Service) read() *signalsdb.Queries {
	if s.qr != nil {
		return s.qr
	}
	return s.q
}

func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

func gunzipBytes(b []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b // not gzipped (defensive) - return as-is
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		return b
	}
	return out
}

func orEventID(id string) string {
	if id == "" {
		return uuid.NewString()
	}
	return id
}

// A tracker timestamp held to the server clock window - rollup day keys and spine partitions both come from it
func msToTime(ms int64) time.Time {
	var t time.Time
	if ms > 0 {
		t = time.UnixMilli(ms)
	}
	return analytics.ClientTime(t, time.Now())
}

// Unwraps the interface{} columns sqlc emits for COALESCE and jsonb-to-text projections, which lib/pq returns as string or []byte
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}
