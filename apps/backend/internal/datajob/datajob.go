// Package datajob persists and runs the transfers internal/dataio describes - every import and export is a
// job so a 200k-row file is never a request/response payload, and a half-hour run survives the admin closing the tab
package datajob

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/dataio"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

func (s Status) Terminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCancelled
}

type Job struct {
	ID        uuid.UUID
	Direction dataio.Direction
	Resource  string
	Format    dataio.Format
	Status    Status
	Options   map[string]string
	DryRun    bool
	ChannelID uuid.UUID
	AdminID   uuid.UUID

	Filename  string
	ObjectKey string
	ByteSize  int64

	TotalRows       int
	ProcessedRows   int
	CreatedRows     int
	UpdatedRows     int
	SkippedRows     int
	FailedRows      int
	ErrorSummary    string
	Warnings        []string
	ErrorsTruncated bool

	Attempts   int
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	ExpiresAt  *time.Time
}

// Whether the artifact is still downloadable - the row outlives its file, so history stays readable after the sweep
func (j Job) HasArtifact() bool { return j.ObjectKey != "" }

type RowError struct {
	Line    int
	Ref     string
	Message string
}

// The object-store operations a job needs - narrower than media.Storage so a test can fake it
type Blobs interface {
	UploadStream(ctx context.Context, key, contentType string, body io.Reader) error
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

var (
	ErrJobNotFound   = errors.New("data job not found")
	ErrNoArtifact    = errors.New("this job has no downloadable file")
	ErrNotCancelable = errors.New("only a job that has not started can be cancelled")
	ErrNoStorage     = errors.New("object storage is not configured, so import and export jobs cannot run")
)
