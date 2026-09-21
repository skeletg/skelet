// Package engagement is the one path a send and its opens and clicks take into the suite's records: the send log, the spine event beside it, and the human-or-machine call
// The contract's handlers and the suite's own engines all go through it, so a send reported by core and one the suite made leave the same trail
package engagement

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/uaclass"
)

type Tracker struct {
	rec   *analytics.Recorder
	sends *sendlog.Store // nil in a test stack without a log - sends then reach the spine alone
}

func New(rec *analytics.Recorder, sends *sendlog.Store) *Tracker {
	return &Tracker{rec: rec, sends: sends}
}

// NewSendID mints the id a send is recorded under - time-ordered, like the log's own default, so the
// primary key it lands in walks in send order
func NewSendID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

// Sent records a send under the caller's id: in the log when its source keeps one, then on the spine
func (t *Tracker) Sent(ctx context.Context, spec sendlog.Spec) error {
	if t.sends != nil && sendlog.Logged(spec.Source) {
		if _, err := t.sends.New(ctx, spec); err != nil {
			return err
		}
	}
	return t.rec.RecordSent(ctx, refOf(spec))
}

// Resolved is what an open or click came back as: the send it belongs to, whether the log still holds
// it, and whether the hit was a reader or a proxy
type Resolved struct {
	Send    sendlog.Send
	Found   bool
	Machine bool
}

// Open stamps the log on the first human open and writes the spine. A send the log no longer holds -
// too old, or never logged - resolves to nothing, and the caller carries on
func (t *Tracker) Open(ctx context.Context, sendID uuid.UUID, hit analytics.Hit) (Resolved, error) {
	send, ok, err := t.lookup(ctx, sendID)
	if err != nil || !ok {
		return Resolved{}, err
	}
	hit, err = t.OpenOf(ctx, send, hit)
	return Resolved{Send: send, Found: true, Machine: hit.Machine}, err
}

// Click is Open's twin for a link, with the destination and its index riding to the spine
func (t *Tracker) Click(ctx context.Context, sendID uuid.UUID, url string, linkIndex int, hit analytics.Hit) (Resolved, error) {
	send, ok, err := t.lookup(ctx, sendID)
	if err != nil || !ok {
		return Resolved{}, err
	}
	hit, err = t.ClickOf(ctx, send, url, linkIndex, hit)
	return Resolved{Send: send, Found: true, Machine: hit.Machine}, err
}

// OpenOf records an open against a send already in hand, classified against when it went out - the
// classified hit comes back so the caller can tell a reader from a proxy
func (t *Tracker) OpenOf(ctx context.Context, send sendlog.Send, hit analytics.Hit) (analytics.Hit, error) {
	hit = Classify(hit, send)
	if _, err := t.rec.RecordOpen(ctx, refOfSend(send), hit); err != nil {
		return hit, err
	}
	if !hit.Machine && send.FirstOpenAt == nil && t.sends != nil {
		if err := t.sends.MarkOpened(ctx, send.ID); err != nil {
			return hit, err
		}
	}
	return hit, nil
}

func (t *Tracker) ClickOf(ctx context.Context, send sendlog.Send, url string, linkIndex int, hit analytics.Hit) (analytics.Hit, error) {
	hit = Classify(hit, send)
	if _, err := t.rec.RecordClick(ctx, refOfSend(send), url, linkIndex, hit); err != nil {
		return hit, err
	}
	if !hit.Machine && send.FirstClickAt == nil && t.sends != nil {
		if err := t.sends.MarkClicked(ctx, send.ID); err != nil {
			return hit, err
		}
	}
	return hit, nil
}

// VariantStats is the per-arm tally off the log, human hits only - what an A/B winner is picked on
func (t *Tracker) VariantStats(ctx context.Context, campaignID uuid.UUID) ([]sendlog.VariantStat, error) {
	if t.sends == nil {
		return nil, nil
	}
	return t.sends.VariantStats(ctx, campaignID)
}

// CustomerEmailEngagement is a customer's lifetime tallies off the spine, machine opens excluded
func (t *Tracker) CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (analytics.EmailEngagement, error) {
	return t.rec.CustomerEmailEngagement(ctx, customerID)
}

func (t *Tracker) lookup(ctx context.Context, sendID uuid.UUID) (sendlog.Send, bool, error) {
	if t.sends == nil {
		return sendlog.Send{}, false, nil
	}
	return t.sends.Get(ctx, sendID)
}

// Classify marks a hit as a machine's when the user agent says so or it landed within seconds of the send - a proxy or scanner rather than a reader
// Decided here because the log knows when the send went out; a send with no time keeps whatever the caller said
func Classify(h analytics.Hit, send sendlog.Send) analytics.Hit {
	if send.SentAt.IsZero() {
		return h
	}
	at := h.At
	if at.IsZero() {
		at = time.Now()
	}
	enr := uaclass.Classify(h.UA, at.Sub(send.SentAt))
	h.Machine, h.Device, h.Client = enr.Machine, enr.Device, enr.Client
	return h
}

func refOf(sp sendlog.Spec) analytics.SendRef {
	return analytics.SendRef{
		ID: sp.ID, ChannelID: sp.ChannelID, CustomerID: sp.CustomerID, Source: sp.Source,
		CampaignID: sp.CampaignID, FlowID: sp.FlowID,
	}
}

func refOfSend(send sendlog.Send) analytics.SendRef {
	return analytics.SendRef{
		ID: send.ID, ChannelID: send.ChannelID, CustomerID: send.CustomerID, Source: send.Source,
		CampaignID: send.CampaignID, FlowID: send.FlowID,
	}
}
