package backup

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/internal/media"
)

// ArchiveStorage opens the BACKUP_* destination for retention archives - the identity-stripped exports the retention jobs write before deleting aged rows
// It rides the backup destination on purpose: same exfiltration concern as dumps, so the target stays env-pinned (see env.go), and the dump pruner ignores non-.dump keys so archives never fall under BACKUP_KEEP
// A nil store with a nil error means backups simply aren't configured
func ArchiveStorage(ctx context.Context) (*media.Storage, string, error) {
	cfg := loadEnv()
	if cfg.Bucket == "" {
		return nil, "", nil
	}
	store, err := media.New(ctx, media.Config{
		Bucket:           cfg.Bucket,
		Region:           cfg.Region,
		InternalEndpoint: cfg.Endpoint,
		UsePathStyle:     cfg.PathStyle,
		AccessKeyID:      cfg.AccessKey,
		SecretAccessKey:  cfg.SecretKey,
	})
	if err != nil {
		return nil, "", fmt.Errorf("archive: init storage: %w", err)
	}
	return store, cfg.Prefix + "archive/", nil
}
