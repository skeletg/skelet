package campaign

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type deactivatedStore struct{}

func (deactivatedStore) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test"}, nil
}

func TestADeactivatedChannelPreviewsAndTestsButNeverSends(t *testing.T) {
	ctx := context.Background()
	stored := Campaign{ID: uuid.New(), ChannelID: uuid.New(), Status: StatusDraft, Subject: "Spring"}
	sender := &capturingSender{}
	svc := func(repo Repository) *Service {
		return &Service{repo: repo, tmpl: emailtemplate.NewService(nil, nil), stores: deactivatedStore{}, sender: sender, track: emailtrack.NewSigner([]byte("k"))}
	}

	if _, _, err := svc(nil).Preview(ctx, stored.ChannelID, "Spring", nil, emailtemplate.EmailStyles{}); err != nil {
		t.Errorf("preview on a deactivated channel: %v", err)
	}
	if err := svc(&fakeCampaignRepo{campaign: stored, found: true}).SendTest(ctx, stored.ID, "qa@example.test"); err != nil || len(sender.sent) != 1 {
		t.Errorf("test send on a deactivated channel: err=%v, mailed %d", err, len(sender.sent))
	}
	before := len(sender.sent)
	if _, err := svc(sendingRepo{&fakeCampaignRepo{}}).send(ctx, stored); err != nil || len(sender.sent) != before {
		t.Errorf("a send pass under a deactivated channel: err=%v, mailed %d", err, len(sender.sent)-before)
	}
}

type pausedChannel struct{ paused uuid.UUID }

func (p pausedChannel) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test", Active: id != p.paused}, nil
}

type touchRepo struct {
	passRepo
	touched []uuid.UUID
}

func (r *touchRepo) RefreshCounts(_ context.Context, id uuid.UUID) error {
	r.touched = append(r.touched, id)
	return nil
}

func TestADeactivatedChannelsCampaignIsHeldWithoutStallingTheDrain(t *testing.T) {
	paused, live := Campaign{ID: uuid.New(), ChannelID: uuid.New()}, Campaign{ID: uuid.New(), ChannelID: uuid.New()}
	repo := &touchRepo{sending: []Campaign{paused, live}}
	svc := &Service{repo: repo, stores: pausedChannel{paused: paused.ChannelID}, track: emailtrack.NewSigner([]byte("k"))}
	if _, err := svc.Drain(context.Background(), time.Second); err != nil {
		t.Fatalf("drain = %v, want a held campaign to be no failure", err)
	}
	if !slices.Contains(repo.passed, live.ID) || slices.Contains(repo.passed, paused.ID) {
		t.Errorf("passes went to %v, want the live campaign and never the paused one", repo.passed)
	}
	if !slices.Contains(repo.touched, paused.ID) {
		t.Error("the held campaign was not moved back, so it leads every pass")
	}
}

type refusingSender struct{}

func (refusingSender) Send(context.Context, email.Message) error {
	return errors.New("550 relay denied")
}

// A failed test send is the relay talking, not a bad campaign - it travels as its own sentinel so the composer can say so
func TestAFailedTestSendNamesTheRelay(t *testing.T) {
	stored := Campaign{ID: uuid.New(), ChannelID: uuid.New(), Status: StatusDraft, Subject: "Spring"}
	svc := &Service{repo: &fakeCampaignRepo{campaign: stored, found: true}, tmpl: emailtemplate.NewService(nil, nil), stores: anyStore{}, sender: refusingSender{}, track: emailtrack.NewSigner([]byte("k"))}
	err := svc.SendTest(context.Background(), stored.ID, "qa@example.test")
	if !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "relay denied") {
		t.Fatalf("err = %v, want ErrSendFailed carrying the relay's answer", err)
	}
}
