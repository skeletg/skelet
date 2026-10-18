package lens

import (
	"context"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// A customer who browsed a product and didn't cart or buy - the scanner's output, carrying the last browsed product for the recovery email's merge tags
type BrowseAbandoner struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	PageKey      string
	Sku          string
	ProductName  string
	ProductImage string
	LastBrowseAt time.Time
}

// The most abandoners one scan returns - core asks for far fewer per channel per tick
const maxBrowseAbandoners int32 = 5000

// Resolves idle product browses to the customers who made them, minus anyone already notified for that browse
// Bounded pages, newest-first: the browse rows are anon-keyed and the spine engine resolves each page to customers
func (s *Service) ListBrowseAbandoners(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]BrowseAbandoner, error) {
	const pageSize = 10000
	if limit <= 0 { // a caller that forgets a limit still gets a bounded scan
		limit = maxBrowseAbandoners
	}
	var cands []BrowseAbandoner
	seen := make(map[uuid.UUID]bool)
	// keyset cursor starts above any real row - browse times are event times, never a day in the future
	afterAt, afterAnon := time.Now().UTC().Add(24*time.Hour), ""
	for {
		rows, err := s.read().BrowseSeenSincePage(ctx, signalsdb.BrowseSeenSincePageParams{
			MaxAge: maxAge, AfterAt: afterAt, AfterAnon: afterAnon, Lim: pageSize,
			ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		anonIDs := make([]string, len(rows))
		for i, r := range rows {
			anonIDs[i] = r.AnonID
		}
		resolved, err := s.rec.CustomersForAnonIDs(ctx, anonIDs)
		if err != nil {
			return nil, err
		}
		// newest-first across pages, so a customer's first resolved row is their latest browse - only an IDLE latest (before the threshold) is an abandonment, a fresher one means they're still browsing
		for _, r := range rows {
			cust, ok := resolved[r.AnonID]
			if !ok || seen[cust] {
				continue
			}
			seen[cust] = true
			if !r.LastBrowseAt.Before(threshold) {
				continue
			}
			cands = append(cands, BrowseAbandoner{
				CustomerID: cust, ChannelID: r.ChannelID, PageKey: r.PageKey,
				Sku: r.Sku, ProductName: r.ProductName, ProductImage: r.ProductImage, LastBrowseAt: r.LastBrowseAt,
			})
		}
		last := rows[len(rows)-1]
		afterAt, afterAnon = last.LastBrowseAt, last.AnonID
		if len(rows) < pageSize {
			break
		}
		// the ones left behind are the STALEST idlers - they surface on a later scan as notified customers
		// drop out of candidacy, while the seen-map and slice stay bounded on a busy window
		if limit > 0 && len(cands) >= int(limit) {
			break
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(cands))
	ats := make([]time.Time, len(cands))
	for i, c := range cands {
		ids[i], ats[i] = c.CustomerID, c.LastBrowseAt
	}
	notifiedRows, err := s.read().BrowseNotifiedForCustomers(ctx, ids)
	if err != nil {
		return nil, err
	}
	type notifyKey struct{ channel, customer uuid.UUID }
	notified := make(map[notifyKey]time.Time, len(notifiedRows))
	for _, n := range notifiedRows {
		notified[notifyKey{n.ChannelID, n.CustomerID}] = n.LastBrowseAt
	}
	out := make([]BrowseAbandoner, 0, limit)
	for _, c := range cands {
		if at, ok := notified[notifyKey{c.ChannelID, c.CustomerID}]; ok && !at.Before(c.LastBrowseAt) {
			continue
		}
		out = append(out, c)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

// Records that a customer was enrolled for a browse at that time, so the episode fires at most once - a newer browse re-fires it
func (s *Service) MarkBrowseNotified(ctx context.Context, channelID, customerID uuid.UUID, lastBrowseAt time.Time) error {
	return s.q.MarkBrowseNotified(ctx, signalsdb.MarkBrowseNotifiedParams{ChannelID: channelID, CustomerID: customerID, LastBrowseAt: lastBrowseAt})
}
