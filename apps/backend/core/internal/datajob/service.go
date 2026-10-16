package datajob

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type Service struct {
	q     *db.Queries
	reg   *dataio.Registry
	blobs Blobs
	cfg   *config.Service
	now   func() time.Time

	renewEvery   time.Duration
	leaseSeconds int32
}

func New(q *db.Queries, reg *dataio.Registry, blobs Blobs) *Service {
	return &Service{q: q, reg: reg, blobs: blobs, now: time.Now, renewEvery: defaultRenewEvery, leaseSeconds: defaultLeaseSeconds}
}

const (
	defaultListLimit   = 50
	maxListLimit       = 200
	defaultErrorsLimit = 100
	maxErrorsLimit     = 1000
)

func (s *Service) Registry() *dataio.Registry { return s.reg }

type ExportRequest struct {
	Resource      string
	Format        dataio.Format
	Options       map[string]string
	ChannelID     uuid.UUID
	Locale        string
	AdminID       uuid.UUID
	IntegrationID uuid.UUID
}

// Queues an export. The file is produced by the runner and parked in the object store, so the caller
// gets a job back immediately rather than holding a connection open for the whole catalog
func (s *Service) QueueExport(ctx context.Context, in ExportRequest) (Job, error) {
	if s.blobs == nil {
		return Job{}, ErrNoStorage
	}
	res, err := s.reg.Get(in.Resource)
	if err != nil {
		return Job{}, err
	}
	if !res.CanExport() {
		return Job{}, fmt.Errorf("%w: %s", dataio.ErrNotExportable, in.Resource)
	}
	codec, err := dataio.CodecFor(in.Format)
	if err != nil {
		return Job{}, err
	}
	opts := withLocale(in.Options, in.Locale)
	// Rejected now rather than as a queued job that is already doomed - a missing required option is knowable here
	if err := s.reg.ValidateOptions(in.Resource, dataio.NewOpts(opts), dataio.DirectionExport); err != nil {
		return Job{}, err
	}
	row, err := s.insert(ctx, insertParams{
		direction:     dataio.DirectionExport,
		resource:      in.Resource,
		format:        in.Format,
		status:        StatusQueued,
		options:       opts,
		channelID:     in.ChannelID,
		adminID:       in.AdminID,
		integrationID: in.IntegrationID,
		filename:      fmt.Sprintf("%s-%s.%s", in.Resource, s.now().UTC().Format("20060102-150405"), codec.Extension()),
	})
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

type ImportRequest struct {
	Resource      string
	Format        dataio.Format
	Options       map[string]string
	ChannelID     uuid.UUID
	Locale        string
	AdminID       uuid.UUID
	IntegrationID uuid.UUID
	DryRun        bool
	Filename      string
}

// Parks the uploaded file in the object store and queues the run - the source is kept so a failed import
// can be re-read against the error report instead of re-uploaded
func (s *Service) QueueImport(ctx context.Context, in ImportRequest, src io.Reader) (Job, error) {
	if s.blobs == nil {
		return Job{}, ErrNoStorage
	}
	res, err := s.reg.Get(in.Resource)
	if err != nil {
		return Job{}, err
	}
	if !res.CanImport() {
		return Job{}, fmt.Errorf("%w: %s", dataio.ErrNotImportable, in.Resource)
	}
	codec, err := dataio.CodecFor(in.Format)
	if err != nil {
		return Job{}, err
	}

	opts := withLocale(in.Options, in.Locale)
	if err := s.reg.ValidateOptions(in.Resource, dataio.NewOpts(opts), dataio.DirectionImport); err != nil {
		return Job{}, err // before the upload, so a bad request never leaves a file behind
	}

	id := uuid.New()
	key := importKey(id, in.Resource, codec.Extension())
	if err := s.blobs.UploadStream(ctx, key, codec.ContentType(), src); err != nil {
		return Job{}, fmt.Errorf("store upload: %w", err)
	}
	filename := in.Filename
	if filename == "" {
		filename = fmt.Sprintf("%s.%s", in.Resource, codec.Extension())
	}
	row, err := s.insert(ctx, insertParams{
		direction:     dataio.DirectionImport,
		resource:      in.Resource,
		format:        in.Format,
		status:        StatusQueued,
		options:       opts,
		dryRun:        in.DryRun,
		channelID:     in.ChannelID,
		adminID:       in.AdminID,
		integrationID: in.IntegrationID,
		filename:      filename,
		objectKey:     key,
	})
	if err != nil {
		// The row is what the sweeper walks, so an orphaned upload has to go now or it leaks forever
		_ = s.blobs.Delete(ctx, key)
		return Job{}, err
	}
	return toJob(row), nil
}

type insertParams struct {
	direction     dataio.Direction
	resource      string
	format        dataio.Format
	status        Status
	options       map[string]string
	dryRun        bool
	channelID     uuid.UUID
	adminID       uuid.UUID
	integrationID uuid.UUID
	filename      string
	objectKey     string
}

func (s *Service) insert(ctx context.Context, p insertParams) (db.DataJob, error) {
	raw, err := json.Marshal(p.options)
	if err != nil {
		return db.DataJob{}, err
	}
	return s.q.CreateDataJob(ctx, db.CreateDataJobParams{
		Direction:     string(p.direction),
		Resource:      p.resource,
		Format:        string(p.format),
		Status:        string(p.status),
		Options:       raw,
		DryRun:        p.dryRun,
		ChannelID:     sqlnull.UUIDValueNonZero(p.channelID),
		AdminID:       sqlnull.UUIDValueNonZero(p.adminID),
		IntegrationID: sqlnull.UUIDValueNonZero(p.integrationID),
		Filename:      p.filename,
		ObjectKey:     p.objectKey,
		ExpiresAt:     sql.NullTime{Time: s.now().Add(s.retention()), Valid: true},
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := s.q.GetDataJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

type ListFilter struct {
	Resources []string
	Direction dataio.Direction
	Statuses  []string
	Limit     int
	Offset    int
	// Nil lists every channel's runs; set, a run shows only where its resource is visible on its channel
	Visibility *Visibility
}

type Visibility struct {
	AnyChannel []string
	Pairs      []VisiblePair
}

type VisiblePair struct {
	Resource  string
	ChannelID uuid.UUID
}

func (v *Visibility) params() (bool, []string, []string) {
	if v == nil {
		return false, nil, nil
	}
	pairs := make([]string, len(v.Pairs))
	for i, p := range v.Pairs {
		pairs[i] = p.Resource + "|" + p.ChannelID.String()
	}
	return true, append([]string{}, v.AnyChannel...), pairs
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Job, int, error) {
	if f.Limit <= 0 {
		f.Limit = defaultListLimit
	}
	f.Limit = min(f.Limit, maxListLimit)
	scoped, anyRes, pairs := f.Visibility.params()
	rows, err := s.q.ListDataJobs(ctx, db.ListDataJobsParams{
		AllResources:        len(f.Resources) == 0,
		Resources:           f.Resources,
		AllDirections:       f.Direction == "",
		Direction:           string(f.Direction),
		AllStatuses:         len(f.Statuses) == 0,
		Statuses:            f.Statuses,
		Scoped:              scoped,
		AnyChannelResources: anyRes,
		ResourceChannels:    pairs,
		PageSize:            int32(f.Limit),
		PageOffset:          int32(f.Offset),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDataJobs(ctx, db.CountDataJobsParams{
		AllResources:        len(f.Resources) == 0,
		Resources:           f.Resources,
		AllDirections:       f.Direction == "",
		Direction:           string(f.Direction),
		AllStatuses:         len(f.Statuses) == 0,
		Statuses:            f.Statuses,
		Scoped:              scoped,
		AnyChannelResources: anyRes,
		ResourceChannels:    pairs,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Job, len(rows))
	for i, r := range rows {
		out[i] = toJob(r)
	}
	return out, int(total), nil
}

func (s *Service) Errors(ctx context.Context, jobID uuid.UUID, limit, offset int) ([]RowError, int, error) {
	if limit <= 0 {
		limit = defaultErrorsLimit
	}
	limit = min(limit, maxErrorsLimit)
	rows, err := s.q.ListDataJobErrors(ctx, db.ListDataJobErrorsParams{
		JobID:      jobID,
		PageSize:   int32(limit),
		PageOffset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDataJobErrors(ctx, jobID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RowError, len(rows))
	for i, r := range rows {
		out[i] = RowError{Line: int(r.Line), Ref: r.Ref, Message: r.Message}
	}
	return out, int(total), nil
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := s.q.CancelDataJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotCancelable
	}
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

// Streams the job's file back. Exports hand over the generated artifact, imports the original upload, so a
// failed run can be re-read against its error report
func (s *Service) OpenArtifact(ctx context.Context, id uuid.UUID) (io.ReadCloser, Job, error) {
	job, err := s.Get(ctx, id)
	if err != nil {
		return nil, Job{}, err
	}
	if !job.HasArtifact() {
		return nil, job, ErrNoArtifact
	}
	if s.blobs == nil {
		return nil, job, ErrNoStorage
	}
	rc, err := s.blobs.GetStream(ctx, job.ObjectKey)
	if err != nil {
		return nil, job, err
	}
	return rc, job, nil
}

func importKey(id uuid.UUID, resource, ext string) string {
	return fmt.Sprintf("data/imports/%s/%s.%s", id, resource, ext)
}

func exportKey(id uuid.UUID, resource, ext string) string {
	return fmt.Sprintf("data/exports/%s/%s.%s", id, resource, ext)
}

func withLocale(opts map[string]string, locale string) map[string]string {
	out := map[string]string{}
	maps.Copy(out, opts)
	if locale != "" {
		out[localeOption] = locale
	}
	return out
}

// Locale rides in the options blob so the job row stays schema-stable, but it is promoted onto Opts before a resource sees it
const localeOption = "__locale"

func toJob(r db.DataJob) Job {
	opts := map[string]string{}
	badOptions := false
	if len(r.Options) > 0 {
		if err := json.Unmarshal(r.Options, &opts); err != nil {
			badOptions = true // the runner fails such a job rather than running it with no options at all
		}
	}
	j := Job{
		ID:              r.ID,
		Direction:       dataio.Direction(r.Direction),
		Resource:        r.Resource,
		Format:          dataio.Format(r.Format),
		Status:          Status(r.Status),
		Options:         opts,
		DryRun:          r.DryRun,
		ChannelID:       r.ChannelID.UUID,
		AdminID:         r.AdminID.UUID,
		IntegrationID:   r.IntegrationID.UUID,
		Filename:        r.Filename,
		ObjectKey:       r.ObjectKey,
		ByteSize:        r.ByteSize,
		TotalRows:       int(r.TotalRows),
		ProcessedRows:   int(r.ProcessedRows),
		CreatedRows:     int(r.CreatedRows),
		UpdatedRows:     int(r.UpdatedRows),
		SkippedRows:     int(r.SkippedRows),
		FailedRows:      int(r.FailedRows),
		ErrorSummary:    r.ErrorSummary,
		Warnings:        r.Warnings,
		ErrorsTruncated: r.ErrorsTruncated,
		Attempts:        int(r.Attempts),
		CreatedAt:       r.CreatedAt,
		badOptions:      badOptions,
	}
	if r.StartedAt.Valid {
		j.StartedAt = &r.StartedAt.Time
	}
	if r.FinishedAt.Valid {
		j.FinishedAt = &r.FinishedAt.Time
	}
	if r.ExpiresAt.Valid {
		j.ExpiresAt = &r.ExpiresAt.Time
	}
	return j
}

// Trims to what a UI can show without becoming a wall of identical Postgres detail
func summarise(errs []dataio.RowError) string {
	if len(errs) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, e := range errs {
		if seen[e.Message] || len(parts) >= 3 {
			continue
		}
		seen[e.Message] = true
		parts = append(parts, e.Message)
	}
	out := strings.Join(parts, " | ")
	if len(errs) > len(parts) {
		out += fmt.Sprintf(" (and %d more)", len(errs)-len(parts))
	}
	return out
}
