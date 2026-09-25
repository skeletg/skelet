package wiring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Per channel, so one busy shop cannot starve the others' recovery mail
const abandonmentPerChannel = 500

type intSettings interface {
	GetInt(ctx context.Context, path string, scope config.Scope) (int64, error)
}

type abandonmentWindow struct{ idleSince, maxAge time.Time }

func channelWindow(ctx context.Context, cfg intSettings, ch uuid.UUID, minsPath, agePath string, defMins, defAgeH int64, now time.Time) abandonmentWindow {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch}
	mins, _ := cfg.GetInt(ctx, minsPath, scope)
	ageH, _ := cfg.GetInt(ctx, agePath, scope)
	if mins <= 0 {
		mins = defMins
	}
	if ageH <= 0 {
		ageH = defAgeH
	}
	return abandonmentWindow{idleSince: now.Add(-time.Duration(mins) * time.Minute), maxAge: now.Add(-time.Duration(ageH) * time.Hour)}
}

type abandonedCarts interface {
	ListAbandoned(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]cart.AbandonedCart, error)
	MarkAbandonedNotified(ctx context.Context, ids []uuid.UUID) error
}

type cartAbandonEnroller interface {
	OnCartAbandoned(ctx context.Context, channelID, customerID uuid.UUID, to string, cartID uuid.UUID, lastActivity time.Time) error
}

func runCartAbandonment(ctx context.Context, channels []uuid.UUID, cfg intSettings, carts abandonedCarts, flows cartAbandonEnroller, now time.Time) error {
	return eachChannel(channels, func(ch uuid.UUID) error { return cartAbandonmentFor(ctx, ch, cfg, carts, flows, now) })
}

func eachChannel(channels []uuid.UUID, run func(uuid.UUID) error) error {
	var errs []error
	for _, ch := range channels {
		if err := run(ch); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", ch, err))
		}
	}
	return errors.Join(errs...)
}

func cartAbandonmentFor(ctx context.Context, ch uuid.UUID, cfg intSettings, carts abandonedCarts, flows cartAbandonEnroller, now time.Time) error {
	w := channelWindow(ctx, cfg, ch, "outreach/triggers/abandoned_cart_minutes", "outreach/triggers/abandoned_cart_max_age_hours", 60, 72, now)
	list, err := carts.ListAbandoned(ctx, ch, w.idleSince, w.maxAge, abandonmentPerChannel)
	if err != nil {
		return err
	}
	done := make([]uuid.UUID, 0, len(list))
	var unreached error
	for _, c := range list {
		if err := flows.OnCartAbandoned(ctx, c.ChannelID, c.CustomerID, c.Email, c.ID, c.LastActivity); err != nil {
			unreached = err
			break
		}
		done = append(done, c.ID)
	}
	if err := carts.MarkAbandonedNotified(ctx, done); err != nil {
		return err
	}
	return unreached
}

type browseAbandonSource interface {
	BrowseAbandoners(ctx context.Context, channelID uuid.UUID, idleSince, maxAge time.Time, limit int32) ([]signalsclient.BrowseAbandoner, error)
	MarkBrowseNotified(ctx context.Context, channelID, customerID uuid.UUID, lastBrowseAt time.Time) error
}

type browseAbandonEnroller interface {
	OnBrowseAbandoned(ctx context.Context, channelID, customerID uuid.UUID, pageKey, sku, name, image string, lastBrowseAt time.Time) error
}

type browseExclusions interface {
	BrowseAbandonersExcluded(ctx context.Context, arg db.BrowseAbandonersExcludedParams) ([]uuid.UUID, error)
}

func runBrowseAbandonment(ctx context.Context, channels []uuid.UUID, cfg intSettings, src browseAbandonSource, excl browseExclusions, flows browseAbandonEnroller, now time.Time) error {
	return eachChannel(channels, func(ch uuid.UUID) error { return browseAbandonmentFor(ctx, ch, cfg, src, excl, flows, now) })
}

func browseAbandonmentFor(ctx context.Context, ch uuid.UUID, cfg intSettings, src browseAbandonSource, excl browseExclusions, flows browseAbandonEnroller, now time.Time) error {
	w := channelWindow(ctx, cfg, ch, "outreach/triggers/browse_abandoned_minutes", "outreach/triggers/browse_abandoned_max_age_hours", 120, 48, now)
	cands, err := src.BrowseAbandoners(ctx, ch, w.idleSince, w.maxAge, abandonmentPerChannel)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(cands))
	ats := make([]time.Time, len(cands))
	for i, c := range cands {
		ids[i], ats[i] = c.CustomerID, c.LastBrowseAt
	}
	excludedRows, err := excl.BrowseAbandonersExcluded(ctx, db.BrowseAbandonersExcludedParams{CustomerIds: ids, BrowseAts: ats})
	if err != nil {
		return err
	}
	excluded := make(map[uuid.UUID]bool, len(excludedRows))
	for _, id := range excludedRows {
		excluded[id] = true
	}
	for _, b := range cands {
		if b.ChannelID != ch || !b.LastBrowseAt.Before(w.idleSince) {
			continue
		}
		if b.LastBrowseAt.After(w.maxAge) && !excluded[b.CustomerID] {
			if err := flows.OnBrowseAbandoned(ctx, b.ChannelID, b.CustomerID, b.PageKey, b.Sku, b.ProductName, b.ProductImage, b.LastBrowseAt); err != nil {
				return err
			}
		}
		if err := src.MarkBrowseNotified(ctx, b.ChannelID, b.CustomerID, b.LastBrowseAt); err != nil {
			return err
		}
	}
	return nil
}

func activeChannelIDs(ctx context.Context, svc interface {
	List(ctx context.Context) ([]channel.Channel, error)
}) ([]uuid.UUID, error) {
	chs, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(chs))
	for _, c := range chs {
		if c.IsActive {
			out = append(out, c.ID)
		}
	}
	return out, nil
}
