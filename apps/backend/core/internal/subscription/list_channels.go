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
	CountForChannels(ctx context.Context, status string, channelIDs []uuid.UUID) (int32, error)
}

// CountForChannels is the total behind ListForChannels: nil channelIDs is every channel, an empty slice none
func (s *Service) CountForChannels(ctx context.Context, status string, channelIDs []uuid.UUID) (int32, error) {
	if channelIDs != nil && len(channelIDs) == 0 {
		return 0, nil
	}
	l, ok := s.repo.(channelLister)
	if !ok {
		return 0, fmt.Errorf("count subscriptions by channel: unsupported repository")
	}
	return l.CountForChannels(ctx, status, channelIDs)
}

func (r *postgresRepository) CountForChannels(ctx context.Context, status string, channelIDs []uuid.UUID) (int32, error) {
	var st sql.NullString
	if status != "" {
		st = sql.NullString{String: status, Valid: true}
	}
	n, err := r.queries.CountSubscriptions(ctx, db.CountSubscriptionsParams{Status: st, ChannelIds: channelIDs})
	if err != nil {
		return 0, fmt.Errorf("count subscriptions: %w", err)
	}
	return n, nil
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
