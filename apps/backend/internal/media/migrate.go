// Store-to-store migration: copies every object from one S3-compatible
// backend to another (Amazon S3 ↔ Garage ↔ anything speaking S3) - Used
// by the `skelet media-migrate` CLI when a deployment moves between the
// managed and self-hosted storage profiles - Objects stream through this
// process (GetObject → PutObject) because CopyObject can't cross
// endpoints; Content-Type is preserved - Idempotent - a re-run skips
// objects that already exist at the destination with the same size, so
// an interrupted migration just resumes

package media

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

// Copies all objects under opts.Prefix from src's bucket to dst's - the first copy error cancels the remaining work and is returned, and since completed objects are skipped on re-run, retrying after a failure is safe and cheap
func Migrate(ctx context.Context, src, dst *Storage, opts MigrateOptions) (MigrateStats, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 8
	}

	items, err := listWithSizes(ctx, src, opts.Prefix)
	if err != nil {
		return MigrateStats{}, fmt.Errorf("media: migrate: list source: %w", err)
	}

	stats := MigrateStats{Scanned: len(items)}
	logf("source %q: %d objects under prefix %q", src.cfg.Bucket, len(items), opts.Prefix)

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
func migrateOne(ctx context.Context, src, dst *Storage, item migrateItem, opts MigrateOptions) (bool, error) {
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

	out, err := src.internalClient.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(src.cfg.Bucket),
		Key:    aws.String(item.key),
	})
	if err != nil {
		return false, fmt.Errorf("get source: %w", err)
	}
	body, err := io.ReadAll(out.Body)
	out.Body.Close()
	if err != nil {
		return false, fmt.Errorf("read source body: %w", err)
	}
	contentType := "application/octet-stream"
	if out.ContentType != nil && *out.ContentType != "" {
		contentType = *out.ContentType
	}
	if err := dst.PutBytes(ctx, item.key, body, contentType); err != nil {
		return false, fmt.Errorf("put destination: %w", err)
	}
	return true, nil
}

// ListPrefix plus object sizes, needed for the skip-if-identical check without a Head per source object
func listWithSizes(ctx context.Context, s *Storage, prefix string) ([]migrateItem, error) {
	items := []migrateItem{}
	var token *string
	for {
		out, err := s.internalClient.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.cfg.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range out.Contents {
			if obj.Key == nil {
				continue
			}
			item := migrateItem{key: *obj.Key}
			if obj.Size != nil {
				item.size = *obj.Size
			}
			items = append(items, item)
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	return items, nil
}
