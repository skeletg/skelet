package signalsboot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A settings reader that answers the registered defaults alone - which is the value under test
type defaultSettings struct{ retentionAction string }

func (d defaultSettings) Int(_ context.Context, path string, _ *uuid.UUID) (int64, error) {
	if path == "signals/analytics/send_log_retention_months" {
		return 24, nil
	}
	return 0, nil
}

func (d defaultSettings) String(_ context.Context, path string, _ *uuid.UUID) (string, error) {
	if path == "signals/analytics/retention_action" {
		return d.retentionAction, nil
	}
	return "", nil
}

type fakeSendLog struct {
	partitions int
	pruned     []time.Time
	archived   []bool
	ensureErr  error
	pruneErr   error
}

func (f *fakeSendLog) EnsurePartitions(context.Context) error {
	f.partitions++
	return f.ensureErr
}

func (f *fakeSendLog) Prune(_ context.Context, before time.Time, archive bool) (int64, error) {
	f.pruned = append(f.pruned, before)
	f.archived = append(f.archived, archive)
	return 1, f.pruneErr
}

// The send log fills on every marketing mail, so shipping "keep forever" as the default is what let it
// grow unbounded - this pins the shipped window
func TestSendLogRetentionDefaultsToTwentyFourMonths(t *testing.T) {
	m := &fakeSendLog{}
	if err := SendLogRetentionJob(defaultSettings{retentionAction: "prune"}, m)(context.Background()); err != nil {
		t.Fatalf("job: %v", err)
	}
	if len(m.pruned) != 1 {
		t.Fatalf("pruned %d times, want 1 - the default has to age the log out", len(m.pruned))
	}
	now := time.Now().UTC()
	want := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -24, 0)
	if !m.pruned[0].Equal(want) {
		t.Errorf("cutoff = %s, want %s (24 months back, anchored to the first of the month)", m.pruned[0], want)
	}
	if m.partitions != 1 {
		t.Errorf("ensured partitions %d times, want 1 - pre-creation must not hang off the retention decision", m.partitions)
	}
}

// A failure to pre-create partitions is the one that silently degrades the table, so it fails the job
func TestSendLogPartitionFailureStopsTheJob(t *testing.T) {
	m := &fakeSendLog{ensureErr: errors.New("no permission")}
	if err := SendLogRetentionJob(defaultSettings{retentionAction: "prune"}, m)(context.Background()); err == nil {
		t.Fatal("partition failure was swallowed, want the job to report it")
	}
	if len(m.pruned) != 0 {
		t.Errorf("pruned %d times after a partition failure, want 0", len(m.pruned))
	}
}

// retention_action is one operator setting, and every sweep honours it
func TestSendLogHonoursTheArchiveAction(t *testing.T) {
	m := &fakeSendLog{}
	if err := SendLogRetentionJob(defaultSettings{retentionAction: "archive"}, m)(context.Background()); err != nil {
		t.Fatalf("job: %v", err)
	}
	if len(m.archived) != 1 || !m.archived[0] {
		t.Errorf("archived = %v, want the months exported before they are dropped", m.archived)
	}
}
