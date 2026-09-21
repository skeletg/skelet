package config

import (
	"context"

	"github.com/google/uuid"
)

// ChannelScoped reads settings for a caller that spells scope as a channel id, which the Signals packages do
// The one place a nil channel becomes the store-wide value and an id becomes that channel's
type ChannelScoped struct{ svc *Service }

func NewChannelScoped(svc *Service) *ChannelScoped { return &ChannelScoped{svc: svc} }

func (s *ChannelScoped) Int(ctx context.Context, path string, channelID *uuid.UUID) (int64, error) {
	return s.svc.GetInt(ctx, path, scopeOfChannel(channelID))
}

func (s *ChannelScoped) Bool(ctx context.Context, path string, channelID *uuid.UUID) (bool, error) {
	return s.svc.GetBool(ctx, path, scopeOfChannel(channelID))
}

func (s *ChannelScoped) String(ctx context.Context, path string, channelID *uuid.UUID) (string, error) {
	return s.svc.GetString(ctx, path, scopeOfChannel(channelID))
}

func scopeOfChannel(channelID *uuid.UUID) Scope {
	if channelID == nil {
		return DefaultScope
	}
	return Scope{Kind: ScopeChannel, ChannelID: channelID}
}
