package shoptools

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

type Meaning interface {
	Products(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]embedding.Hit, error)
	Passages(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]embedding.Passage, error)
	ProductTexts(ctx context.Context, ids []uuid.UUID, locales []string) (map[uuid.UUID]string, error)
	Similar(ctx context.Context, channelID uuid.UUID, locales []string, products []uuid.UUID, limit int) (embedding.Similar, error)
}

const passagesPerPage = 2

func searchLocales(ctx context.Context, ch channel.Channel) []string {
	return search.MeaningLocales(locale.FromContext(ctx), ch.DefaultLocale)
}

// The store's own Search by meaning setting governs the assistant too - off keeps it on the words, and the strictness sets the reach
func (c call) reach(ctx context.Context) (float64, bool) {
	if c.svc.deps.Meaning == nil {
		return 0, false
	}
	var s search.Settings
	if c.svc.deps.Settings != nil {
		s = c.svc.deps.Settings
	}
	return search.MeaningReach(ctx, s, c.ch.ID)
}

func fuse(words, meaning []string, limit int) []string {
	order := search.Fuse(words, meaning, 1, search.FusionK)
	return order[:min(len(order), limit)]
}

func (c call) nearProducts(ctx context.Context, query string, limit int) []string {
	share, on := c.reach(ctx)
	if !on {
		return nil
	}
	hits, err := c.svc.deps.Meaning.Products(ctx, c.ch.ID, searchLocales(ctx, c.ch), query, limit)
	if err != nil {
		quietWhenPaced(ctx, "shoptools: search by meaning failed, words only", c.ch.ID, err)
		return nil
	}
	var out []string
	for _, h := range hits {
		if search.Within(hits[0].Distance, h.Distance, share) {
			out = append(out, h.ProductID.String())
		}
	}
	return out
}

func (c call) nearPassages(ctx context.Context, query string, limit int) []embedding.Passage {
	share, on := c.reach(ctx)
	if !on {
		return nil
	}
	found, err := c.svc.deps.Meaning.Passages(ctx, c.ch.ID, searchLocales(ctx, c.ch), query, limit)
	if err != nil {
		quietWhenPaced(ctx, "shoptools: page search by meaning failed, words only", c.ch.ID, err)
		return nil
	}
	var out []embedding.Passage
	for _, p := range found {
		if search.Within(found[0].Distance, p.Distance, share) {
			out = append(out, p)
		}
	}
	return out
}

// A shopper past their pace isn't a fault worth a warning - the search goes on with its words
func quietWhenPaced(ctx context.Context, msg string, channelID uuid.UUID, err error) {
	if !errors.Is(err, ratelimit.ErrLimited) {
		slog.WarnContext(ctx, msg, "channel", channelID, "err", err)
	}
}
