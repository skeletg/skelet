package campaign

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// Email open and click tracking - at send time each recipient's HTML gets a 1x1 open pixel and its links rewritten through a click-redirect, using the shared emailtrack signer so campaign and flow tokens are interchangeable, and the /e/o and /e/c routes decode the token, hand the hit to the tracker, which keeps the send log and the spine, and keep the campaign counters in sync through the recipient

func TransparentGIF() []byte { return emailtrack.GIF() }

func (s *Service) injectTracking(html string, ch Store, sendID uuid.UUID) string {
	return s.track.Inject(html, storefrontURL(ch), sendID)
}

// Keeps the campaign's unique-open counter in sync on the first open per recipient, and returns the engagement a human open by a known customer amounts to, for the caller to enrol a flow on - best-effort, an invalid or expired send is ignored and the pixel still serves
func (s *Service) RecordOpen(ctx context.Context, token string, h analytics.Hit) *Engaged {
	p, err := s.track.Verify(token)
	if err != nil || s.recorder == nil {
		return nil
	}
	eng, err := s.recorder.Open(ctx, p.S, h)
	if err != nil || !eng.Found || eng.Machine {
		return nil
	}
	send := eng.Send
	if send.RecipientID != nil {
		if first, err := s.repo.RecordOpen(ctx, *send.RecipientID); err == nil && first && send.CampaignID != nil {
			_ = s.repo.BumpOpened(ctx, *send.CampaignID)
		}
	}
	if send.CustomerID != nil {
		return &Engaged{Kind: KindOpened, ChannelID: send.ChannelID, CustomerID: *send.CustomerID, CampaignID: send.CampaignID, FlowID: send.FlowID}
	}
	return nil
}

// ClickTarget is the signed destination alone, recording nothing - an over-limit click still sends the reader on
func (s *Service) ClickTarget(token string) string {
	p, err := s.track.Verify(token)
	if err != nil {
		return ""
	}
	return p.U
}

// Returns the signed destination URL ("" when invalid) plus the engagement a human click by a known customer amounts to - the redirect happens even when the send is gone from the log, so links never break
func (s *Service) RecordClick(ctx context.Context, token string, h analytics.Hit) (string, *Engaged) {
	p, err := s.track.Verify(token)
	if err != nil || p.U == "" {
		return "", nil
	}
	if s.recorder == nil {
		return p.U, nil
	}
	eng, err := s.recorder.Click(ctx, p.S, p.U, p.L, h)
	if err != nil || !eng.Found || eng.Machine {
		return p.U, nil
	}
	send := eng.Send
	if send.RecipientID != nil {
		if first, err := s.repo.RecordClick(ctx, *send.RecipientID); err == nil && first && send.CampaignID != nil {
			_ = s.repo.BumpClicked(ctx, *send.CampaignID)
		}
	}
	if send.CustomerID != nil {
		return p.U, &Engaged{Kind: KindClicked, ChannelID: send.ChannelID, CustomerID: *send.CustomerID, CampaignID: send.CampaignID, FlowID: send.FlowID}
	}
	return p.U, nil
}
