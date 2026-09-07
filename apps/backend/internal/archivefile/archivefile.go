// Package archivefile streams retention exports to object storage as gzip JSONL, shared by the spine and the send log so neither imports the other for it
package archivefile

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// Sink is the object-storage seam, satisfied by the BACKUP_* destination adapter in signalsboot - Put must return nil only once the object is durably stored and verified, because the sweep deletes rows on its word
type Sink interface {
	Put(ctx context.Context, key, contentType string, body io.Reader) error
}

// Open opens the archive file for one aged slice, lazily on its first row - that row's id rides into the key, so a crash-retry overwrites the same object and a late arrival gets its own
type Open func(day time.Time, first uuid.UUID) (*File, error)

// Fail-closed: archive mode with no destination must never fall back to plain deletion
var ErrNoSink = errors.New("retention_action is archive but no archive destination is configured - set the BACKUP_S3_* environment variables or switch back to prune")

var errAborted = errors.New("archive export aborted")

// PageSize is the keyset page for archive exports - bounds memory while the gzip stream runs
const PageSize = 5000

// File streams gzip JSONL into the sink through a pipe, so a slice never buffers in memory - Close commits the upload, Abort kills it without finalizing a partial object
type File struct {
	gz   *gzip.Writer
	pw   *io.PipeWriter
	done chan error
	enc  *json.Encoder
}

func New(ctx context.Context, sink Sink, key string) *File {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		var err error
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("archive upload panicked: %v", rec)
				}
			}()
			err = sink.Put(ctx, key, "application/gzip", pr)
		}()
		pr.CloseWithError(err) // a Put that returns early (failed upload) must unblock the writer side, not deadlock it
		done <- err
	}()
	gz := gzip.NewWriter(pw)
	return &File{gz: gz, pw: pw, done: done, enc: json.NewEncoder(gz)}
}

func (f *File) Write(v any) error { return f.enc.Encode(v) }

func (f *File) Close() error {
	gzErr := f.gz.Close()
	_ = f.pw.Close()
	if err := <-f.done; err != nil { // the upload's verdict outranks a pipe-side symptom of the same failure
		return err
	}
	return gzErr
}

func (f *File) Abort() {
	f.pw.CloseWithError(errAborted)
	<-f.done // reap the upload goroutine so an abort can't leak it
}

// Discard closes an archive file that is being abandoned - a nil file is the "never opened" case and is not an error
func Discard(f *File) {
	if f != nil {
		f.Abort()
	}
}
