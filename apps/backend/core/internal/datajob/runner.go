package datajob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

const (
	tickInterval = time.Second
	// How often the runner renews its lease, which is also how quickly a cancellation is noticed
	renewEvery = 2 * time.Second
	// Long enough that a slow 250k-row import isn't stolen mid-run - the runner renews as rows land, so a crash still frees it within this
	leaseSeconds = 120
	claimBatch   = 2
	// A job that kills the process on every attempt must stop being retried, or it takes the runner down forever
	maxAttempts = 3
)

// Run is the claim loop - one process or twenty, SKIP LOCKED keeps them off each other's rows
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("data job tick failed", "error", err)
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) error {
	rows, err := s.q.ClaimDueDataJobs(ctx, db.ClaimDueDataJobsParams{
		LeaseSeconds: leaseSeconds,
		MaxAttempts:  maxAttempts,
		BatchSize:    claimBatch,
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		job := toJob(row)
		if err := s.execute(ctx, job); err != nil {
			if errors.Is(err, context.Canceled) {
				return err // shutting down - the lease lapses and another process picks it up
			}
			slog.Error("data job failed", "job", job.ID, "resource", job.Resource, "direction", job.Direction, "error", err)
			s.fail(ctx, job, err)
		}
	}
	return nil
}

func (s *Service) execute(ctx context.Context, job Job) error {
	if s.blobs == nil {
		return ErrNoStorage
	}
	// Its own cancel so a cancellation noticed mid-run unwinds the driver without touching the tick's ctx
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	var cancelled atomic.Bool

	run := dataio.Run{
		Resource: job.Resource,
		Format:   job.Format,
		Opts:     s.opts(job),
		Progress: s.progress(runCtx, job.ID, stop, &cancelled),
	}
	var err error
	if job.Direction == dataio.DirectionExport {
		err = s.runExport(runCtx, job, run)
	} else {
		err = s.runImport(runCtx, job, run)
	}
	// CancelDataJob already wrote the terminal row, so a run that stopped because of it is not a failure
	if cancelled.Load() {
		slog.Info("data job cancelled mid-run", "job", job.ID, "resource", job.Resource)
		return nil
	}
	return err
}

func (s *Service) opts(job Job) dataio.Opts {
	values := map[string]string{}
	for k, v := range job.Options {
		if k != localeOption {
			values[k] = v
		}
	}
	o := dataio.NewOpts(values)
	o.ChannelID = job.ChannelID
	o.Locale = job.Options[localeOption]
	o.DryRun = job.DryRun
	o.ActorID = job.AdminID
	return o
}

// Doubles as the lease renewal AND the cancellation check - the renewal is scoped to status='running', so a
// cancelled job renews 0 rows and that is the stop signal. Cancelling therefore lands within one interval
func (s *Service) progress(ctx context.Context, id uuid.UUID, stop context.CancelFunc, cancelled *atomic.Bool) func(processed, total int) {
	var last time.Time
	return func(processed, total int) {
		if s.now().Sub(last) < renewEvery {
			return
		}
		last = s.now()
		n, err := s.q.RenewDataJobLease(ctx, db.RenewDataJobLeaseParams{
			ID:            id,
			LeaseSeconds:  leaseSeconds,
			ProcessedRows: int32(processed),
			TotalRows:     int32(total),
		})
		if err != nil {
			slog.Warn("data job lease renewal failed", "job", id, "error", err)
			return
		}
		if n == 0 {
			cancelled.Store(true)
			stop()
		}
	}
}

// The export streams straight into the object store through a pipe, so the file never exists in memory
func (s *Service) runExport(ctx context.Context, job Job, run dataio.Run) error {
	codec, err := dataio.CodecFor(job.Format)
	if err != nil {
		return err
	}
	key := exportKey(job.ID, job.Resource, codec.Extension())

	pr, pw := io.Pipe()
	type outcome struct {
		res dataio.ExportResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		var res dataio.ExportResult
		var err error
		func() {
			// a panicking export resource would kill the whole process from here - convert it to a
			// failed run and a poisoned pipe instead
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("export panicked: %v", rec)
					slog.Error("datajob export panicked", "resource", run.Resource, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				}
			}()
			res, err = s.reg.Export(ctx, run, pw)
		}()
		// CloseWithError is load-bearing: a plain Close finalises a TRUNCATED object that looks complete
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
		done <- outcome{res, err}
	}()

	counted := &countingReader{r: pr}
	uploadErr := s.blobs.UploadStream(ctx, key, codec.ContentType(), counted)
	// Drain so the producer can never block in Write after the upload gave up
	_, _ = io.Copy(io.Discard, pr)
	pr.CloseWithError(uploadErr)
	out := <-done

	if out.err != nil {
		_ = s.blobs.Delete(ctx, key)
		return out.err
	}
	if uploadErr != nil {
		_ = s.blobs.Delete(ctx, key)
		return fmt.Errorf("store export: %w", uploadErr)
	}

	_, err = s.q.FinishDataJob(ctx, db.FinishDataJobParams{
		ID:            job.ID,
		Status:        string(StatusSucceeded),
		TotalRows:     int32(out.res.Rows),
		ProcessedRows: int32(out.res.Rows),
		Warnings:      []string{},
		ObjectKey:     key,
		ByteSize:      counted.n, // counted on the way past, so the size costs nothing extra
	})
	return err
}

func (s *Service) runImport(ctx context.Context, job Job, run dataio.Run) error {
	src, err := s.blobs.GetStream(ctx, job.ObjectKey)
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}
	defer src.Close()

	res, err := s.reg.Import(ctx, run, src)
	if err != nil {
		return err
	}
	if err := s.saveErrors(ctx, job.ID, res.Errors); err != nil {
		slog.Warn("data job error rows could not be stored", "job", job.ID, "error", err)
	}

	// A file where every row failed is a failed job, not a successful one that happens to have done nothing
	status := StatusSucceeded
	if res.Failed > 0 && res.Created == 0 && res.Updated == 0 {
		status = StatusFailed
	}
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	_, err = s.q.FinishDataJob(ctx, db.FinishDataJobParams{
		ID:              job.ID,
		Status:          string(status),
		TotalRows:       int32(res.Total),
		ProcessedRows:   int32(res.Total),
		CreatedRows:     int32(res.Created),
		UpdatedRows:     int32(res.Updated),
		SkippedRows:     int32(res.Skipped),
		FailedRows:      int32(res.Failed),
		ErrorSummary:    summarise(res.Errors),
		Warnings:        warnings,
		ErrorsTruncated: res.Truncated,
		ObjectKey:       job.ObjectKey,
		ByteSize:        job.ByteSize,
	})
	return err
}

func (s *Service) saveErrors(ctx context.Context, jobID uuid.UUID, errs []dataio.RowError) error {
	if len(errs) == 0 {
		return nil
	}
	lines := make([]int32, len(errs))
	refs := make([]string, len(errs))
	messages := make([]string, len(errs))
	for i, e := range errs {
		lines[i], refs[i], messages[i] = int32(e.Line), e.Ref, e.Message
	}
	return s.q.InsertDataJobErrors(ctx, db.InsertDataJobErrorsParams{
		JobID:    jobID,
		Lines:    lines,
		Refs:     refs,
		Messages: messages,
	})
}

// A failure still writes whatever counts the run reached - "it died at row 40,000" is the useful part
func (s *Service) fail(ctx context.Context, job Job, cause error) {
	if _, err := s.q.FinishDataJob(ctx, db.FinishDataJobParams{
		ID:           job.ID,
		Status:       string(StatusFailed),
		ErrorSummary: cause.Error(),
		Warnings:     []string{},
		ObjectKey:    job.ObjectKey,
		ByteSize:     job.ByteSize,
	}); err != nil {
		slog.Error("could not mark data job failed", "job", job.ID, "error", err)
	}
}

// Sweep deletes artifacts past their retention and prunes long-finished rows - registered as a scheduled job
func (s *Service) Sweep(ctx context.Context) error {
	if s.blobs == nil {
		return nil
	}
	expired, err := s.q.ExpireDataJobArtifacts(ctx)
	if err != nil {
		return err
	}
	for _, row := range expired {
		if err := s.blobs.Delete(ctx, row.ObjectKey); err != nil {
			slog.Warn("could not delete an expired data job artifact", "job", row.ID, "key", row.ObjectKey, "error", err)
		}
	}
	// A job whose runner died past its retry budget would otherwise read as running forever
	if n, err := s.q.ReapStuckDataJobs(ctx, maxAttempts); err != nil {
		slog.Warn("could not reap stuck data jobs", "error", err)
	} else if n > 0 {
		slog.Warn("marked stuck data jobs failed", "count", n)
	}

	// The rows outlive their files by design, but not forever
	cutoff := s.now().Add(-s.historyRetention(ctx))
	if _, err := s.q.DeleteDataJobsBefore(ctx, cutoff); err != nil {
		return err
	}
	return nil
}

// Tallies what passes through on the way to the store, so byte_size costs no extra round trip
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
