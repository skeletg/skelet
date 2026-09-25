package subscription

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

type channelLister interface {
	ListForChannels(ctx context.Context, status string, channelIDs []uuid.UUID, limit, offset int32) ([]Subscription, error)
}

func (s *Service) ListForChannels(ctx context.Context, status string, channelIDs []uuid.UUID, limit, offset int32) ([]Subscription, error) {
	if channelIDs == nil {
		return s.List(ctx, status, limit, offset)
	}
	if len(channelIDs) == 0 {
		return []Subscription{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	l, ok := s.repo.(channelLister)
	if !ok {
		return nil, fmt.Errorf("list subscriptions by channel: unsupported repository")
	}
	return l.ListForChannels(ctx, status, channelIDs, limit, offset)
}

func (r *postgresRepository) ListForChannels(ctx context.Context, status string, channelIDs []uuid.UUID, limit, offset int32) ([]Subscription, error) {
	var st sql.NullString
	if status != "" {
		st = sql.NullString{String: status, Valid: true}
	}
	rows, err := r.queries.ListSubscriptionsForChannels(ctx, db.ListSubscriptionsForChannelsParams{
		Status: st, ChannelIds: channelIDs, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list subscriptions by channel: %w", err)
	}
	return toSubscriptions(rows), nil
}
