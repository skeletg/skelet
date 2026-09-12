package archivefile

import (
	"context"
	"fmt"
	"io"

	"github.com/skeletg/skelet/apps/backend/internal/media"
)

// StorageSink writes an archive object to a media store under a fixed prefix, and reads it back
// before reporting success - retention deletes the rows once the sink returns, so an upload that
// silently wrote nothing would lose them
// Lives beside the interface because both retention sweeps use it, the spine's and the send log's
type StorageSink struct {
	store  *media.Storage
	prefix string
}

func NewStorageSink(store *media.Storage, prefix string) StorageSink {
	return StorageSink{store: store, prefix: prefix}
}

func (a StorageSink) Put(ctx context.Context, key, contentType string, body io.Reader) error {
	full := a.prefix + key
	if err := a.store.UploadStream(ctx, full, contentType, body); err != nil {
		return err
	}
	size, _, exists, err := a.store.Head(ctx, full)
	if err == nil && exists && size > 0 {
		return nil
	}
	_ = a.store.Delete(ctx, full)
	if err != nil {
		return fmt.Errorf("archive object %s failed post-upload verification: %w", full, err)
	}
	return fmt.Errorf("archive object %s missing or empty after upload (size=%d)", full, size)
}
