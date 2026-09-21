package boot

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

// The seams the suite packages declare, filled from core's public leaves

// The SSRF guard destinations are held to - the create-time check and the transport the forwarder
// posts through, which is where a host that turns private at request time is caught
type urlGuard struct{}

func (urlGuard) ValidateTargetURL(raw string, allowPrivate bool) error {
	return urlguard.ValidateTargetURL(raw, allowPrivate)
}

func (urlGuard) SafeTransport(allowPrivate bool) *http.Transport {
	return urlguard.SafeTransport(allowPrivate)
}

// The BACKUP_* destination the retention sweeps archive to before deleting
// A broken one only warns: the sweep fails closed without a sink rather than deleting what it could not archive
func archiveDestination(ctx context.Context) (archivefile.ObjectStore, string) {
	store, prefix := backupBucket(ctx)
	if store == nil {
		return nil, ""
	}
	return store, prefix + "archive/"
}

// The BACKUP_* bucket and prefix the retention archive writes under - nil when it is not configured
func backupBucket(ctx context.Context) (*objstore.Storage, string) {
	bucket := strings.TrimSpace(os.Getenv("BACKUP_S3_BUCKET"))
	if bucket == "" {
		return nil, ""
	}
	pathStyle := true
	if v, err := strconv.ParseBool(os.Getenv("BACKUP_S3_PATH_STYLE")); err == nil {
		pathStyle = v
	}
	store, err := objstore.New(ctx, objstore.Config{
		Bucket:           bucket,
		Region:           strings.TrimSpace(os.Getenv("BACKUP_S3_REGION")),
		InternalEndpoint: strings.TrimSpace(os.Getenv("BACKUP_S3_ENDPOINT_URL")),
		UsePathStyle:     pathStyle,
		AccessKeyID:      os.Getenv("BACKUP_S3_ACCESS_KEY_ID"),
		SecretAccessKey:  os.Getenv("BACKUP_S3_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		slog.Warn("backup destination unavailable", "err", err)
		return nil, ""
	}
	prefix := os.Getenv("BACKUP_S3_PREFIX")
	if prefix == "" {
		prefix = "db/"
	}
	return store, prefix
}

// Lens keeps replay chunks and DOM snapshots in the store's own bucket - the same S3_* variables core's
// media reads. Without one the rest of Lens still works, so this warns rather than refusing to boot
func wireLensBlobStore(ctx context.Context, lensSvc *lens.Service) error {
	bucket := os.Getenv("S3_BUCKET_NAME")
	if bucket == "" {
		slog.Warn("S3 not configured - session replays and page snapshots are dropped at ingest, so replay and heatmaps stay empty")
		return nil
	}
	store, err := objstore.New(ctx, objstore.Config{
		Bucket:           bucket,
		Region:           os.Getenv("AWS_REGION"),
		InternalEndpoint: os.Getenv("S3_ENDPOINT_URL"),
		PresignEndpoint:  os.Getenv("S3_PRESIGN_ENDPOINT_URL"),
		PublicURLBase:    os.Getenv("S3_PUBLIC_URL_BASE"),
		UsePathStyle:     os.Getenv("S3_USE_PATH_STYLE") == "true",
		MaxUploadBytes:   EnvInt64("S3_MAX_UPLOAD_BYTES", 10*1024*1024),
	})
	if err != nil {
		return err
	}
	lensSvc.SetBlobStore(store)
	return nil
}
