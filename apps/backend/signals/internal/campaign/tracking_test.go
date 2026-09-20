package campaign

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

// The tracker keeps the send log and classifies the hit; what stays here is what the campaign does
// with the answer - the recipient's first-open stamp, the campaign counter and the engagement a flow enrols on

func trackingService(rec sendTracker) (*Service, *fakeCampaignRepo) {
	repo := &fakeCampaignRepo{}
	return &Service{repo: repo, recorder: rec, track: emailtrack.NewSigner([]byte("tracking-test-key"))}, repo
}

func resolved(customer, recipient, campaign *uuid.UUID, machine bool) engagement.Resolved {
	return engagement.Resolved{
		Found: true, Machine: machine,
		Send: sendlog.Send{ID: uuid.New(), ChannelID: uuid.New(), Source: sendlog.SourceCampaign, CustomerID: customer, RecipientID: recipient, CampaignID: campaign},
	}
}

func ptr(id uuid.UUID) *uuid.UUID { return &id }

func TestAHumanOpenBumpsTheCampaignOnceAndEnrolsTheCustomer(t *testing.T) {
	cust, rcpt, camp := uuid.New(), uuid.New(), uuid.New()
	svc, repo := trackingService(&fakeRecorder{engage: resolved(ptr(cust), ptr(rcpt), ptr(camp), false)})
	token := svc.track.Sign(emailtrack.Payload{S: uuid.New()})

	ev := svc.RecordOpen(context.Background(), token, analytics.Hit{})
	if ev == nil || ev.Kind != KindOpened || ev.CustomerID != cust || ev.CampaignID == nil || *ev.CampaignID != camp {
		t.Fatalf("event = %+v, want an opened event for the customer and campaign the suite resolved", ev)
	}
	if len(repo.opened) != 1 || repo.opened[0] != rcpt {
		t.Errorf("recipient opens recorded = %v, want the resolved recipient once", repo.opened)
	}
	if len(repo.bumpedOpen) != 1 || repo.bumpedOpen[0] != camp {
		t.Errorf("campaign bumps = %v, want the campaign once, on the recipient's first open", repo.bumpedOpen)
	}

	// The same recipient again - counted on the row, but the campaign's unique-open counter stays put
	svc.RecordOpen(context.Background(), token, analytics.Hit{})
	if len(repo.bumpedOpen) != 1 {
		t.Errorf("campaign bumped %d times over two opens by one recipient, want 1", len(repo.bumpedOpen))
	}
}

// A proxy prefetch is not the customer reading anything - nothing moves and no flow starts
func TestAMachineOpenMovesNothing(t *testing.T) {
	svc, repo := trackingService(&fakeRecorder{engage: resolved(ptr(uuid.New()), ptr(uuid.New()), ptr(uuid.New()), true)})
	token := svc.track.Sign(emailtrack.Payload{S: uuid.New()})

	if ev := svc.RecordOpen(context.Background(), token, analytics.Hit{}); ev != nil {
		t.Errorf("a machine open produced %+v, want nothing", ev)
	}
	if len(repo.opened) != 0 || len(repo.bumpedOpen) != 0 {
		t.Error("a machine open touched the recipient or the campaign")
	}
}

// A send the suite no longer holds - too old, or never logged - is ignored, and so is a guest's open,
// since there is no customer to enrol
func TestAnUnknownOrGuestSendEnrolsNobody(t *testing.T) {
	unknown, _ := trackingService(&fakeRecorder{engage: engagement.Resolved{Found: false}})
	if ev := unknown.RecordOpen(context.Background(), unknown.track.Sign(emailtrack.Payload{S: uuid.New()}), analytics.Hit{}); ev != nil {
		t.Errorf("an unknown send produced %+v", ev)
	}
	guest, repo := trackingService(&fakeRecorder{engage: resolved(nil, ptr(uuid.New()), ptr(uuid.New()), false)})
	if ev := guest.RecordOpen(context.Background(), guest.track.Sign(emailtrack.Payload{S: uuid.New()}), analytics.Hit{}); ev != nil {
		t.Errorf("a guest open produced %+v, want the counter bumped but nobody enrolled", ev)
	}
	if len(repo.opened) != 1 {
		t.Error("a guest's open was not counted on the recipient")
	}
}

// The redirect must never break - a click on a forgotten send, or with the suite unreachable, still lands on the destination baked into the token
func TestAClickRedirectsWhateverTheSuiteAnswers(t *testing.T) {
	for name, rec := range map[string]*fakeRecorder{
		"unknown send":      {engage: engagement.Resolved{Found: false}},
		"suite unreachable": {err: context.DeadlineExceeded},
		"machine click":     {engage: resolved(ptr(uuid.New()), nil, nil, true)},
	} {
		svc, _ := trackingService(rec)
		token := svc.track.Sign(emailtrack.Payload{S: uuid.New(), U: "https://shop.example/p/tire", L: 2})
		dest, ev := svc.RecordClick(context.Background(), token, analytics.Hit{})
		if dest != "https://shop.example/p/tire" {
			t.Errorf("%s: destination = %q, want the token's", name, dest)
		}
		if ev != nil {
			t.Errorf("%s: produced %+v, want no enrolment", name, ev)
		}
	}
	svc, _ := trackingService(nil)
	if dest, _ := svc.RecordClick(context.Background(), svc.track.Sign(emailtrack.Payload{S: uuid.New(), U: "https://shop.example/x"}), analytics.Hit{}); dest != "https://shop.example/x" {
		t.Errorf("with no recorder at all the destination = %q, want the token's", dest)
	}
}

// A forged or truncated token is nobody's open
func TestAnInvalidTokenIsIgnored(t *testing.T) {
	svc, repo := trackingService(&fakeRecorder{engage: resolved(ptr(uuid.New()), ptr(uuid.New()), ptr(uuid.New()), false)})
	if ev := svc.RecordOpen(context.Background(), "not-a-token", analytics.Hit{}); ev != nil || len(repo.opened) != 0 {
		t.Error("an invalid token was treated as an open")
	}
	if dest, _ := svc.RecordClick(context.Background(), "not-a-token", analytics.Hit{}); dest != "" {
		t.Errorf("an invalid token redirected to %q", dest)
	}
}
