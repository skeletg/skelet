package segment

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type oneChannelRoster struct {
	Repository
	channel uuid.UUID
}

func (r oneChannelRoster) RosterChannels(context.Context) ([]uuid.UUID, error) {
	return []uuid.UUID{r.channel}, nil
}

func TestTheRefreshCompletesAnOldSpendRuleAndRuns(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	only := uuid.New()
	repo := oneChannelRoster{Repository: NewRepository(db.New(conn)), channel: only}
	slug := "old-spend-" + uuid.NewString()[:8]
	seg, err := repo.Create(ctx, Segment{Slug: slug, Name: slug, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })
	if _, err := repo.SetRules(ctx, seg.ID, json.RawMessage(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":10000}]}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := NewService(repo, txrunner.New(conn), conn, nil).RefreshDynamic(ctx, seg.ID); err != nil {
		t.Fatalf("refresh = %v, want the rule completed with the install's only channel and run", err)
	}
	stored, err := repo.Get(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p := RulesProblem(stored.Rules); p != "" {
		t.Fatalf("the stored rule still can't run: %s", p)
	}
}

func TestTheSoleChannelFillNeverOverwritesAConcurrentEdit(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	repo := oneChannelRoster{Repository: NewRepository(db.New(conn)), channel: uuid.New()}
	slug := "fill-race-" + uuid.NewString()[:8]
	seg, err := repo.Create(ctx, Segment{Slug: slug, Name: slug, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })
	stale := json.RawMessage(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":10000}]}`)
	if _, err := repo.SetRules(ctx, seg.ID, stale); err != nil {
		t.Fatal(err)
	}
	read, err := repo.Get(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	edited := json.RawMessage(`{"conditions":[{"kind":"group","slug":"vip"}]}`)
	if _, err := repo.SetRules(ctx, seg.ID, edited); err != nil {
		t.Fatal(err)
	}

	got, err := NewService(repo, txrunner.New(conn), conn, nil).fillSoleChannel(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r := mustRules(t, string(stored.Rules)); r.Conditions[0].Kind != "group" {
		t.Fatalf("stored rules %s, want the admin's edit rather than the filled stale read", stored.Rules)
	}
	if r := mustRules(t, string(got.Rules)); r.Conditions[0].Kind != "group" {
		t.Fatalf("fill answered %s, want the rules as they now stand", got.Rules)
	}
}
