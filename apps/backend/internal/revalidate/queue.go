package revalidate

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
)

// Tuning for the rebuild worker - conservative by default, so a big backlog drains steadily instead of stampeding the renderer
const (
	rebuildMaxAttempts  = 4
	rebuildLeaseSeconds = 120
	rebuildBackoffSecs  = 30
)

// The durable, coalescing rebuild queue - producers Enqueue paths instantly, and the cache-rebuild scheduler job drains it through ProcessDue with bounded concurrency, retries and at-least-once lease semantics
type Queue struct {
	q      *db.Queries
	client *Client
}

func NewQueue(q *db.Queries, client *Client) *Queue { return &Queue{q: q, client: client} }

func dedupKey(surface, path string) string { return surface + "|" + path }

// Adds one rebuild task, with a duplicate PENDING task for the same surface and path coalesced by the partial unique index - returns whether a new task was actually inserted
func (qu *Queue) Enqueue(ctx context.Context, surface, path string, tags []string, reason string) (bool, error) {
	if qu == nil || path == "" {
		return false, nil
	}
	n, err := qu.q.EnqueueRebuildTask(ctx, db.EnqueueRebuildTaskParams{
		Surface:  surface,
		Path:     path,
		Tags:     nonNil(tags),
		Reason:   reason,
		DedupKey: dedupKey(surface, path),
	})
	return n > 0, err
}

// Enqueues many tasks for one surface and records a single activity-log entry, returning the count submitted
func (qu *Queue) EnqueuePaths(ctx context.Context, surface string, paths, tags []string, reason string, actor *uuid.UUID) (int, error) {
	if qu == nil {
		return 0, nil
	}
	n := 0
	for _, p := range paths {
		if p == "" {
			continue
		}
		inserted, err := qu.Enqueue(ctx, surface, p, tags, reason)
		if err != nil {
			return n, err
		}
		if inserted {
			n++
		}
	}
	if n > 0 && qu.client != nil && qu.client.store != nil {
		// Log a SUMMARY only - the per-task table holds the individual paths, so recording the whole slice here would bloat one row at bulk scale
		_ = qu.client.store.Record(ctx, LogEntry{
			Surface: surface,
			Reason:  fmt.Sprintf("queued %d path(s) for rebuild: %s", n, reason),
			ActorID: actor,
		})
	}
	return n, nil
}

// Claims a batch of due tasks and warms them with bounded concurrency, returning how many were processed - called by the cache-rebuild job
func (qu *Queue) ProcessDue(ctx context.Context, batch, concurrency int) (int, error) {
	if qu == nil {
		return 0, nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	tasks, err := qu.q.ClaimDueRebuildTasks(ctx, db.ClaimDueRebuildTasksParams{
		LeaseSeconds: rebuildLeaseSeconds,
		BatchSize:    int32(batch),
	})
	if err != nil || len(tasks) == 0 {
		return 0, err
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	// A warm failure is normal - it retries with backoff and eventually dead-letters.
	// The BOOKKEEPING write failing is not: a dropped FailRebuildTask never increments
	// attempts, so the dead-letter branch becomes unreachable and a permanently broken
	// path is re-warmed forever, while a dropped MarkRebuildTaskDone re-warms a page
	// that already succeeded. Those go back to the scheduler; warm failures don't
	var (
		mu       sync.Mutex
		bookErrs []error
	)
	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(t db.CacheRebuildTask) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[revalidate] warm task %s panicked: %v", t.ID, rec)
				}
			}()
			record := func(err error, what string) {
				if err == nil {
					return
				}
				mu.Lock()
				bookErrs = append(bookErrs, fmt.Errorf("%s %s: %w", what, t.Path, err))
				mu.Unlock()
			}
			if warmErr := qu.client.warmPath(ctx, t.Tags, t.Path); warmErr != nil {
				status := "pending" // retry with backoff …
				if int(t.Attempts) >= rebuildMaxAttempts {
					status = "failed" // … unless attempts are exhausted → dead-letter
				}
				record(qu.q.FailRebuildTask(ctx, db.FailRebuildTaskParams{
					ID: t.ID, Status: status, LastError: warmErr.Error(), BackoffSeconds: rebuildBackoffSecs,
				}), "mark failed")
				return
			}
			record(qu.q.MarkRebuildTaskDone(ctx, t.ID), "mark done")
		}(t)
	}
	wg.Wait()
	return len(tasks), errors.Join(bookErrs...)
}

// status to number of tasks, for the admin queue panel
func (qu *Queue) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := qu.q.RebuildTaskStatusCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Status] = int(r.Count)
	}
	return out, nil
}

// Recent returns the most-recently-touched tasks
func (qu *Queue) Recent(ctx context.Context, limit int) ([]db.CacheRebuildTask, error) {
	if limit <= 0 {
		limit = 50
	}
	return qu.q.ListRecentRebuildTasks(ctx, int32(limit))
}

// Prune deletes finished (done/failed) tasks older than the cutoff
func (qu *Queue) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	return qu.q.DeleteFinishedRebuildTasksBefore(ctx, time.Now().Add(-olderThan))
}
