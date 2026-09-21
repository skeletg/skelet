package archivefile

import (
	"context"
	"fmt"
	"io"
)

// ObjectStore is the slice of an object store an archive needs - declared here so this package names
// the three operations rather than whoever implements them
type ObjectStore interface {
	UploadStream(ctx context.Context, key, contentType string, body io.Reader) error
	Head(ctx context.Context, key string) (size int64, contentType string, exists bool, err error)
	Delete(ctx context.Context, key string) error
}

// StorageSink writes an archive object under a fixed prefix and reads it back before reporting success, since retention deletes the rows once it returns
// Lives beside the interface because both retention sweeps use it, the spine's and the send log's
type StorageSink struct {
	store  ObjectStore
	prefix string
}

func NewStorageSink(store ObjectStore, prefix string) StorageSink {
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
