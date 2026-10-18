// Store-to-store migration behind `skelet media-migrate` - every object streams through this process, since CopyObject cannot cross endpoints
// Idempotent: a re-run skips objects already at the destination with the same size, so an interrupted move resumes

package objstore

import (
	"context"
	"fmt"
	"sync"
)

// MigrateOptions tunes a Migrate run - Zero value = copy everything,
// 8 workers, skip objects already present with a matching size
type MigrateOptions struct {
	// Prefix restricts the copy to keys under this prefix ("" = all)
	Prefix string
	// DryRun lists and diffs but writes nothing to the destination
	DryRun bool
	// Copies every object even when the destination already has the key at the same size
	Overwrite bool
	Workers   int
	Log       func(format string, args ...any)
}

type MigrateStats struct {
	Scanned int   // objects listed at the source
	Copied  int   // objects written to the destination (or would-copy in DryRun)
	Skipped int   // objects already at the destination with a matching size
	Bytes   int64 // payload bytes copied (source sizes; 0 in DryRun)
}

type migrateItem struct {
	key  string
	size int64
}

// Copies all objects under opts.Prefix from src to dst - the first copy error cancels the remaining work and is returned, and since completed objects are skipped on re-run, retrying after a failure is safe and cheap
func Migrate(ctx context.Context, src, dst Storage, opts MigrateOptions) (MigrateStats, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 8
	}

	objs, err := src.ListObjects(ctx, opts.Prefix)
	if err != nil {
		return MigrateStats{}, fmt.Errorf("media: migrate: list source: %w", err)
	}
	items := make([]migrateItem, len(objs))
	for i, o := range objs {
		items[i] = migrateItem{key: o.Key, size: o.Size}
	}

	stats := MigrateStats{Scanned: len(items)}
	logf("source %q: %d objects under prefix %q", src.Bucket(), len(items), opts.Prefix)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
		work     = make(chan migrateItem)
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}

	for range workers {
		wg.Go(func() {
			for item := range work {
				copied, err := migrateOne(ctx, src, dst, item, opts)
				if err != nil {
					fail(fmt.Errorf("media: migrate %q: %w", item.key, err))
					return
				}
				mu.Lock()
				if copied {
					stats.Copied++
					if !opts.DryRun {
						stats.Bytes += item.size
					}
				} else {
					stats.Skipped++
				}
				if done := stats.Copied + stats.Skipped; done%100 == 0 {
					logf("progress: %d/%d (%d copied, %d skipped)", done, stats.Scanned, stats.Copied, stats.Skipped)
				}
				mu.Unlock()
			}
		})
	}

feed:
	for _, item := range items {
		select {
		case work <- item:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	if firstErr != nil {
		return stats, firstErr
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	return stats, nil
}

// Copies a single object, reporting whether it was copied - false means skipped as already present at a matching size
func migrateOne(ctx context.Context, src, dst Storage, item migrateItem, opts MigrateOptions) (bool, error) {
	if !opts.Overwrite {
		size, _, exists, err := dst.Head(ctx, item.key)
		if err != nil {
			return false, fmt.Errorf("head destination: %w", err)
		}
		if exists && size == item.size {
			return false, nil
		}
	}
	if opts.DryRun {
		return true, nil
	}

	body, contentType, err := src.GetBytesTyped(ctx, item.key)
	if err != nil {
		return false, fmt.Errorf("get source: %w", err)
	}
	if err := dst.PutBytes(ctx, item.key, body, contentType); err != nil {
		return false, fmt.Errorf("put destination: %w", err)
	}
	return true, nil
}
