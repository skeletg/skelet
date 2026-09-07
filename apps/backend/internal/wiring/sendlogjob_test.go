package wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// An empty config store, so every read falls through to the registered default - which is the value under test
type emptyConfigRepo struct{}

func (emptyConfigRepo) Upsert(context.Context, config.Scope, string, string) (config.StoredValue, error) {
	return config.StoredValue{}, errors.New("not used")
}
func (emptyConfigRepo) Delete(context.Context, config.Scope, string) error { return nil }
func (emptyConfigRepo) GetCandidates(context.Context, config.Scope, string) ([]config.StoredValue, error) {
	return nil, nil
}
func (emptyConfigRepo) ListInScopes(context.Context, config.Scope) ([]config.StoredValue, error) {
	return nil, nil
}
func (emptyConfigRepo) ListAll(context.Context) ([]config.StoredValue, error) { return nil, nil }

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

func noArchive(context.Context, *config.Service) (bool, error) { return false, nil }

func runSendLogJob(t *testing.T, m *fakeSendLog, archiveAction func(context.Context, *config.Service) (bool, error)) error {
	t.Helper()
	return sendLogRetentionJob(config.NewService(emptyConfigRepo{}), m, archiveAction)(context.Background())
}

// The send log is a commerce table that fills on every transactional mail, so shipping "keep forever" as the
// default is what let it grow unbounded - this pins the shipped window
func TestSendLogRetentionDefaultsToTwentyFourMonths(t *testing.T) {
	m := &fakeSendLog{}
	if err := runSendLogJob(t, m, noArchive); err != nil {
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
}

// Partition pre-creation must not hang off the retention decision - without it sends land in the DEFAULT
// partition once the pre-created months run out, whatever the operator set retention to
func TestSendLogPartitionsArePreCreatedBeforeRetentionIsRead(t *testing.T) {
	m := &fakeSendLog{}
	if err := runSendLogJob(t, m, noArchive); err != nil {
		t.Fatalf("job: %v", err)
	}
	if m.partitions != 1 {
		t.Errorf("ensured partitions %d times, want 1", m.partitions)
	}
}

// A failure to pre-create partitions is the one that silently degrades the table, so it fails the job
func TestSendLogPartitionFailureStopsTheJob(t *testing.T) {
	m := &fakeSendLog{ensureErr: errors.New("no permission")}
	if err := runSendLogJob(t, m, noArchive); err == nil {
		t.Fatal("partition failure was swallowed, want the job to report it")
	}
	if len(m.pruned) != 0 {
		t.Errorf("pruned %d times after a partition failure, want 0", len(m.pruned))
	}
}

// retention_action is an operator setting the core job has to honour, not just the Signals one
func TestSendLogHonoursTheArchiveAction(t *testing.T) {
	m := &fakeSendLog{}
	archive := func(context.Context, *config.Service) (bool, error) { return true, nil }
	if err := runSendLogJob(t, m, archive); err != nil {
		t.Fatalf("job: %v", err)
	}
	if len(m.archived) != 1 || !m.archived[0] {
		t.Errorf("archived = %v, want the months exported before they are dropped", m.archived)
	}
}
