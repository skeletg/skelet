package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Only the two methods attribution touches are answered - anything else panics on the nil embed,
// which is what proves the anchor is resolved from the spine alone
type attributionSpine struct {
	SpineStore
	candidates []EngagementCandidate
	appended   []SpineEvent
}

func (s *attributionSpine) RecentEngagementEvents(context.Context, uuid.UUID, time.Time) ([]EngagementCandidate, error) {
	return s.candidates, nil
}

func (s *attributionSpine) AppendEvent(_ context.Context, e SpineEvent) (bool, error) {
	s.appended = append(s.appended, e)
	return true, nil
}

func attributionRecorder(cands ...EngagementCandidate) (*Recorder, *attributionSpine) {
	sp := &attributionSpine{candidates: cands}
	return &Recorder{spine: sp}, sp
}

// The send's origin now rides on the event, so a campaign click anchors with its campaign id attached
func TestAClickAnchorsTheConversionWithItsCampaign(t *testing.T) {
	now := time.Now()
	send, campaign := uuid.New(), uuid.New()
	r, sp := attributionRecorder(EngagementCandidate{
		EventName: EventEmailClicked, OccurredAt: now.Add(-2 * time.Hour), SendID: send,
		Source: SourceCampaign, CampaignID: campaign.String(),
	})

	ok, err := r.RecordConversion(context.Background(), ConversionSpec{
		CustomerID: uuid.New(), ChannelID: uuid.New(), OrderID: uuid.New(),
		RevenueCents: 4200, Currency: "EUR", At: now,
	})
	if err != nil || !ok {
		t.Fatalf("RecordConversion = %v, %v, want true, nil", ok, err)
	}
	if len(sp.appended) != 1 {
		t.Fatalf("wrote %d events, want the one conversion", len(sp.appended))
	}
	got := sp.appended[0]
	if got.EventName != EventConversion {
		t.Errorf("wrote %q, want %q", got.EventName, EventConversion)
	}
	if got.Props["campaign_id"] != campaign.String() {
		t.Errorf("campaign_id = %v, want %s - attribution lost the origin the event carried", got.Props["campaign_id"], campaign)
	}
	if got.Props["via"] != EventEmailClicked || got.Props["attributed_source"] != SourceCampaign {
		t.Errorf("anchor recorded as via=%v source=%v", got.Props["via"], got.Props["attributed_source"])
	}
}

func TestAFlowSendAnchorsWithItsFlow(t *testing.T) {
	now := time.Now()
	flowID := uuid.New()
	r, sp := attributionRecorder(EngagementCandidate{
		EventName: EventEmailClicked, OccurredAt: now.Add(-time.Hour), SendID: uuid.New(),
		Source: SourceFlow, FlowID: flowID.String(),
	})

	if ok, err := r.RecordConversion(context.Background(), ConversionSpec{
		CustomerID: uuid.New(), OrderID: uuid.New(), At: now,
	}); err != nil || !ok {
		t.Fatalf("RecordConversion = %v, %v", ok, err)
	}
	if sp.appended[0].Props["flow_id"] != flowID.String() {
		t.Errorf("flow_id = %v, want %s", sp.appended[0].Props["flow_id"], flowID)
	}
}

// A transactional send is not marketing, so it must never anchor a conversion
func TestATransactionalSendAnchorsNothing(t *testing.T) {
	now := time.Now()
	r, sp := attributionRecorder(EngagementCandidate{
		EventName: EventEmailClicked, OccurredAt: now.Add(-time.Hour), SendID: uuid.New(), Source: "transactional",
	})

	ok, err := r.RecordConversion(context.Background(), ConversionSpec{
		CustomerID: uuid.New(), OrderID: uuid.New(), At: now,
	})
	if err != nil {
		t.Fatalf("RecordConversion err = %v", err)
	}
	if ok || len(sp.appended) != 0 {
		t.Errorf("a transactional send anchored a conversion (ok=%v, wrote %d)", ok, len(sp.appended))
	}
}

// Candidates arrive best-first, so the first marketing one wins even with a transactional send ahead of it
func TestTheFirstMarketingCandidateWins(t *testing.T) {
	now := time.Now()
	wanted := uuid.New()
	r, sp := attributionRecorder(
		EngagementCandidate{EventName: EventEmailClicked, OccurredAt: now.Add(-time.Minute), SendID: uuid.New(), Source: "transactional"},
		EngagementCandidate{EventName: EventEmailClicked, OccurredAt: now.Add(-time.Hour), SendID: wanted, Source: SourceCampaign},
		EngagementCandidate{EventName: EventEmailOpened, OccurredAt: now.Add(-2 * time.Hour), SendID: uuid.New(), Source: SourceCampaign},
	)

	if ok, err := r.RecordConversion(context.Background(), ConversionSpec{
		CustomerID: uuid.New(), OrderID: uuid.New(), At: now,
	}); err != nil || !ok {
		t.Fatalf("RecordConversion = %v, %v", ok, err)
	}
	if got := sp.appended[0].Props["attributed_send_id"]; got != wanted.String() {
		t.Errorf("anchored %v, want the first marketing candidate %s", got, wanted)
	}
}

// A bare open credits for a day, not the click window's seven - an older one anchors nothing
func TestAStaleOpenDoesNotAnchor(t *testing.T) {
	now := time.Now()
	r, sp := attributionRecorder(EngagementCandidate{
		EventName: EventEmailOpened, OccurredAt: now.Add(-3 * 24 * time.Hour), SendID: uuid.New(), Source: SourceCampaign,
	})

	ok, err := r.RecordConversion(context.Background(), ConversionSpec{
		CustomerID: uuid.New(), OrderID: uuid.New(), At: now,
	})
	if err != nil {
		t.Fatalf("RecordConversion err = %v", err)
	}
	if ok || len(sp.appended) != 0 {
		t.Errorf("an open %v old anchored a conversion", 3*24*time.Hour)
	}
}
