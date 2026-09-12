package campaign

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/emailtrack"
	"github.com/skeletg/skelet/apps/backend/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/internal/uaclass"
)

// Email open and click tracking - at send time each recipient's HTML gets a 1x1 open pixel and its links rewritten through a click-redirect, using the shared emailtrack signer so campaign, flow and transactional tokens are interchangeable, and the /e/o and /e/c routes decode the token, persist to the analytics spine, and keep the campaign counters in sync through the recipient

func TransparentGIF() []byte { return emailtrack.GIF() }

func (s *Service) injectTracking(html string, ch channel.Channel, sendID uuid.UUID) string {
	return s.track.Inject(html, storefrontURL(ch), sendID)
}

// Keeps the campaign's unique-open counter in sync on the first open per recipient, and returns a bus activation event for a human open by a known customer for the caller to publish for flow enrollment - best-effort, an invalid or expired send is ignored and the pixel still serves
func (s *Service) RecordOpen(ctx context.Context, token string, h signalsclient.Hit) *sendlog.Engaged {
	p, err := s.track.Verify(token)
	if err != nil {
		return nil
	}
	send, ok, err := s.sends.Get(ctx, p.S)
	if err != nil || !ok {
		return nil
	}
	enr := uaclass.Classify(h.UA, h.At.Sub(send.SentAt))
	h.Machine, h.Device, h.Client = enr.Machine, enr.Device, enr.Client
	_ = s.recorder.RecordEmailOpen(ctx, sendRef(send), h)
	if h.Machine {
		return nil
	}
	// Core's own record of the send, stamped whether or not Signals is running - the spine event above is the analytics half
	if send.FirstOpenAt == nil {
		_ = s.sends.MarkOpened(ctx, send.ID)
	}
	if send.RecipientID != nil {
		if first, err := s.repo.RecordOpen(ctx, *send.RecipientID); err == nil && first && send.CampaignID != nil {
			_ = s.repo.BumpOpened(ctx, *send.CampaignID)
		}
	}
	if send.CustomerID != nil {
		return &sendlog.Engaged{Kind: sendlog.KindOpened, ChannelID: send.ChannelID, CustomerID: *send.CustomerID, CampaignID: send.CampaignID}
	}
	return nil
}

// Returns the signed destination URL ("" when invalid) plus a bus activation event for a human click by a known customer - the redirect happens even when the send row is gone, so links never break
func (s *Service) RecordClick(ctx context.Context, token string, h signalsclient.Hit) (string, *sendlog.Engaged) {
	p, err := s.track.Verify(token)
	if err != nil || p.U == "" {
		return "", nil
	}
	var ev *sendlog.Engaged
	if send, ok, err := s.sends.Get(ctx, p.S); err == nil && ok {
		enr := uaclass.Classify(h.UA, h.At.Sub(send.SentAt))
		h.Machine, h.Device, h.Client = enr.Machine, enr.Device, enr.Client
		_ = s.recorder.RecordEmailClick(ctx, sendRef(send), p.U, p.L, h)
		if !h.Machine {
			if send.FirstClickAt == nil {
				_ = s.sends.MarkClicked(ctx, send.ID)
			}
			if send.RecipientID != nil {
				if first, err := s.repo.RecordClick(ctx, *send.RecipientID); err == nil && first && send.CampaignID != nil {
					_ = s.repo.BumpClicked(ctx, *send.CampaignID)
				}
			}
			if send.CustomerID != nil {
				ev = &sendlog.Engaged{Kind: sendlog.KindClicked, ChannelID: send.ChannelID, CustomerID: *send.CustomerID, CampaignID: send.CampaignID}
			}
		}
	}
	return p.U, ev
}

// The send reduced to what the spine stores - the send log itself stays core's
func sendRef(s sendlog.Send) signalsclient.SendRef {
	return signalsclient.SendRef{
		ID: s.ID, ChannelID: s.ChannelID, CustomerID: s.CustomerID, Source: s.Source,
		CampaignID: s.CampaignID, FlowID: s.FlowID,
	}
}
