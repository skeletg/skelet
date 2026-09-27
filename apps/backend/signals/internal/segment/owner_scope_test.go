package segment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type ownedStore struct {
	Repository
	seg    Segment
	stored json.RawMessage
}

func (r *ownedStore) Get(context.Context, uuid.UUID) (Segment, error) { return r.seg, nil }
func (r *ownedStore) SetRules(_ context.Context, id uuid.UUID, raw json.RawMessage) (Segment, error) {
	r.stored = raw
	return Segment{ID: id, Rules: raw}, nil
}

func ownedBy(ch uuid.UUID) string { return fmt.Sprintf(`{"channelId":%q}`, ch) }

func TestAnOwnedSegmentRefusesARuleOnAnotherChannel(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	store := &ownedStore{seg: Segment{ID: uuid.New(), Definition: ownedBy(owner)}}
	svc := &Service{repo: store}
	foreign := fmt.Sprintf(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1,"channelId":%q}]}`, other)
	if _, err := svc.SetRules(context.Background(), store.seg.ID, json.RawMessage(foreign)); err == nil || store.stored != nil {
		t.Fatalf("a rule on another channel was saved (err=%v) - it reads that channel's spend", err)
	}
	own := fmt.Sprintf(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1,"channelId":%q},{"kind":"group","slug":"vip"}]}`, owner)
	if _, err := svc.SetRules(context.Background(), store.seg.ID, json.RawMessage(own)); err != nil {
		t.Fatalf("a rule on the owner's channel was refused: %v", err)
	}
}

func TestCompileInHoldsEveryBranchToTheOwner(t *testing.T) {
	owner := uuid.New()
	r := mustRules(t, `{"logic":"or","conditions":[{"kind":"group","slug":"a"},{"kind":"group","slug":"b"}]}`)
	q, args := r.CompileIn(nil, owner)
	if !strings.Contains(q, "c.channel_id = $3 AND (") || args[2] != owner {
		t.Fatalf("query %q args %v, want the owner predicate around the whole OR", q, args)
	}
}

func TestRefreshingAnOwnedSegmentOnlyTakesItsOwnCustomers(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	owner, other := uuid.New(), uuid.New()
	slug := "owned-" + uuid.NewString()[:8]
	mine := seedRoster(t, conn, rosterRow{channelID: owner, groupSlug: slug})
	theirs := seedRoster(t, conn, rosterRow{channelID: other, groupSlug: slug})
	repo := NewRepository(db.New(conn))
	seg, err := repo.Create(ctx, Segment{Slug: slug, Name: slug, IsActive: true, Definition: ownedBy(owner)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), seg.ID) })
	if _, err := repo.SetRules(ctx, seg.ID, json.RawMessage(fmt.Sprintf(`{"conditions":[{"kind":"group","slug":%q}]}`, slug))); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(repo, txrunner.New(conn), conn, nil).RefreshDynamic(ctx, seg.ID); err != nil {
		t.Fatal(err)
	}
	ids, err := repo.MemberIDs(ctx, seg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !selects(ids, mine) || selects(ids, theirs) {
		t.Fatalf("members %v, want the owner's customer %s and never the other channel's %s", ids, mine, theirs)
	}
}
