package experiment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type oneExperiment struct {
	Repository
	stored  Experiment
	found   bool
	updates []Experiment
}

func (r *oneExperiment) Get(context.Context, uuid.UUID) (Experiment, bool, error) {
	return r.stored, r.found, nil
}

func (r *oneExperiment) Update(_ context.Context, e Experiment) (bool, error) {
	r.updates = append(r.updates, e)
	return true, nil
}

func started(status string) *oneExperiment {
	in := validInput()
	return &oneExperiment{found: true, stored: Experiment{
		ID: uuid.New(), ChannelID: in.ChannelID, Key: in.Key, Name: in.Name, Status: status,
		Coverage: in.Coverage, GoalEvent: in.GoalEvent, Variations: in.Variations,
	}}
}

// A visitor's bucket comes from what's locked here, so changing it mid-run would re-bucket them under results that no longer apply
func TestAStartedExperimentLocksWhatBucketsVisitors(t *testing.T) {
	for name, mutate := range map[string]func(*Input){
		"a weight": func(in *Input) {
			in.Variations = []Variation{{Key: "control", Weight: 90, Value: "orange"}, {Key: "blue", Weight: 10, Value: "blue"}}
		},
		"a value": func(in *Input) {
			in.Variations = []Variation{{Key: "control", Weight: 50, Value: "red"}, {Key: "blue", Weight: 50, Value: "blue"}}
		},
		"coverage":  func(in *Input) { in.Coverage = 40 },
		"the goal":  func(in *Input) { in.GoalEvent = "purchase" },
		"targeting": func(in *Input) { in.SegmentSlug = "vip" },
	} {
		for _, status := range []string{StatusRunning, StatusStopped} {
			repo := started(status)
			in := validInput()
			mutate(&in)
			if _, err := NewService(repo).Update(context.Background(), repo.stored.ID, in); !errors.Is(err, ErrLocked) {
				t.Errorf("%s experiment, changing %s: err = %v, want ErrLocked", status, name, err)
			}
		}
	}
	repo := started(StatusRunning)
	in := validInput()
	in.Name, in.Status = "Renamed", StatusStopped
	in.Variations = []Variation{{Key: "control", Name: "Orange", Weight: 50, Value: "orange"}, {Key: "blue", Name: "Blue", Weight: 50, Value: "blue"}}
	if _, err := NewService(repo).Update(context.Background(), repo.stored.ID, in); err != nil || len(repo.updates) != 1 {
		t.Fatalf("renaming and stopping a running experiment: %v", err)
	}
	draft := started(StatusDraft)
	in = validInput()
	in.Coverage = 10
	if _, err := NewService(draft).Update(context.Background(), draft.stored.ID, in); err != nil {
		t.Errorf("a draft is freely editable: %v", err)
	}
}

func TestTheKeyNeverChangesOnUpdate(t *testing.T) {
	repo := started(StatusDraft)
	in := validInput()
	in.Key = "Not A Valid Key" // the stored key stands, so whatever the form sent can't fail or change it
	if _, err := NewService(repo).Update(context.Background(), repo.stored.ID, in); err != nil {
		t.Fatal(err)
	}
	if got := repo.updates[0].Key; got != "" && got != repo.stored.Key {
		t.Errorf("update wrote key %q", got)
	}
}

func TestMissingExperimentsAreNotFound(t *testing.T) {
	svc := NewService(&oneExperiment{})
	if _, err := svc.Update(context.Background(), uuid.New(), validInput()); !errors.Is(err, ErrNotFound) {
		t.Errorf("update: err = %v, want ErrNotFound", err)
	}
	if _, _, err := svc.Results(context.Background(), uuid.New(), time.Time{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("results: err = %v, want ErrNotFound", err)
	}
}

func TestThePublicPayloadIsBounded(t *testing.T) {
	in := validInput()
	for i := range maxVariations {
		in.Variations = append(in.Variations, Variation{Key: "v" + string(rune('a'+i)), Weight: 1})
	}
	if err := validate(in); !errors.Is(err, ErrInvalid) {
		t.Errorf("%d variations: err = %v", len(in.Variations), err)
	}
	in = validInput()
	in.Variations[1].Value = strings.Repeat("x", maxVariationValue+1)
	if err := validate(in); !errors.Is(err, ErrInvalid) {
		t.Errorf("an oversized value: err = %v", err)
	}
	for _, bad := range []string{"Has Space", "UPPER", "-lead", strings.Repeat("k", 70)} {
		in = validInput()
		in.Key = bad
		if err := validate(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("key %q: err = %v", bad, err)
		}
	}
}

func TestADuplicateKeyIsReportedAsTaken(t *testing.T) {
	conn := testdb.RequireSignals(t)
	svc := NewService(NewRepository(db.New(conn), nil))
	in := validInput()
	in.Key = "dup-" + uuid.NewString()[:8]
	first, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Delete(context.Background(), first.ChannelID, first.ID) })
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrKeyTaken) {
		t.Errorf("duplicate key: err = %v, want ErrKeyTaken", err)
	}
}
