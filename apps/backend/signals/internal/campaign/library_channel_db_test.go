package campaign

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type sendingRepo struct {
	*fakeCampaignRepo
}

func (sendingRepo) ListPending(context.Context, uuid.UUID, int32) ([]Recipient, error) {
	return nil, nil
}
func (sendingRepo) RefreshCounts(context.Context, uuid.UUID) error            { return nil }
func (sendingRepo) HasPending(context.Context, uuid.UUID) (bool, error)       { return true, nil }
func (sendingRepo) AddCounts(context.Context, uuid.UUID, int, int, int) error { return nil }

type anyStore struct{}

func (anyStore) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test", Active: true}, nil
}

type capturingSender struct{ sent []email.Message }

func (c *capturingSender) Send(_ context.Context, m email.Message) error {
	c.sent = append(c.sent, m)
	return nil
}

func TestACampaignOnlyUsesALibraryEmailOfItsOwnChannel(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	tmpl := emailtemplate.NewService(emailtemplate.NewRepository(db.New(conn)), nil)
	mine, theirs := uuid.New(), uuid.New()
	own, err := tmpl.CreateLibraryEmail(ctx, emailtemplate.LibraryEmail{ChannelID: mine, Name: "Own", Subject: "own"})
	if err != nil {
		t.Fatalf("seed own email: %v", err)
	}
	foreign, err := tmpl.CreateLibraryEmail(ctx, emailtemplate.LibraryEmail{ChannelID: theirs, Name: "Theirs", Subject: "theirs"})
	if err != nil {
		t.Fatalf("seed foreign email: %v", err)
	}
	svc := func(repo Repository, sender email.Sender) *Service {
		return &Service{repo: repo, tmpl: tmpl, stores: anyStore{}, sender: sender, track: emailtrack.NewSigner([]byte("campaign-test-key"))}
	}
	input := func(emailID uuid.UUID) Input {
		return Input{ChannelID: mine, Name: "Spring sale", AudienceKind: AudienceAll, EmailID: &emailID}
	}

	t.Run("create", func(t *testing.T) {
		repo := &fakeCampaignRepo{}
		if _, err := svc(repo, nil).Create(ctx, input(foreign.ID)); err == nil || len(repo.created) != 0 {
			t.Errorf("created with another channel's email: err=%v, stored %d", err, len(repo.created))
		}
		if _, err := svc(repo, nil).Create(ctx, input(own.ID)); err != nil {
			t.Errorf("own channel's email refused: %v", err)
		}
	})
	t.Run("update", func(t *testing.T) {
		repo := &fakeCampaignRepo{updateOK: true}
		if _, err := svc(repo, nil).Update(ctx, uuid.New(), input(foreign.ID)); err == nil || len(repo.updated) != 0 {
			t.Errorf("updated to another channel's email: err=%v, stored %d", err, len(repo.updated))
		}
	})
	stored := Campaign{ID: uuid.New(), ChannelID: mine, Status: StatusDraft, EmailID: &foreign.ID}
	t.Run("test send", func(t *testing.T) {
		sender := &capturingSender{}
		repo := &fakeCampaignRepo{campaign: stored, found: true}
		if err := svc(repo, sender).SendTest(ctx, stored.ID, "qa@example.test"); err == nil || len(sender.sent) != 0 {
			t.Errorf("test-sent another channel's email: err=%v, mailed %d", err, len(sender.sent))
		}
	})
	t.Run("send", func(t *testing.T) {
		repo := sendingRepo{&fakeCampaignRepo{}}
		if _, err := svc(repo, &capturingSender{}).send(ctx, stored); err == nil {
			t.Error("a send pass ran with another channel's email")
		}
	})
}
