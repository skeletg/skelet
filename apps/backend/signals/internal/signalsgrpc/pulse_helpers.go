package signalsgrpc

// Shared plumbing for the Pulse surfaces - flows, campaigns and the email builder, which core relays with the caller's own grants
// Core's own automation reaches a few of them on the system token, so every gate here admits either

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// The permission, then the same permission on the channel asked for - or core itself
func channelScopeOrSystem(ctx context.Context, perm, channelID string) (uuid.UUID, error) {
	if auth.IsSystemCaller(ctx) {
		return parseID("channel_id", channelID)
	}
	return channelScope(ctx, perm, channelID)
}

// For an operation on an entity already loaded - the channel it belongs to is what the grant has to cover
func requireChannelOrSystem(ctx context.Context, perm string, channelID uuid.UUID) error {
	if auth.IsSystemCaller(ctx) {
		return nil
	}
	return requireChannel(ctx, perm, channelID)
}

// The admin behind a relayed call, for the authorship a revision records - none on the system token
func callingAdmin(ctx context.Context) *uuid.UUID {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims.AdminID == uuid.Nil {
		return nil
	}
	id := claims.AdminID
	return &id
}

// A domain "not found" travels as NOT_FOUND, and the composer's own validation as INVALID_ARGUMENT -
// the resolvers on the other side render each differently
func domainErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not found") {
		return status.Error(codes.NotFound, err.Error())
	}
	return status.Error(codes.InvalidArgument, err.Error())
}

func blocksFromJSON(raw string) ([]emailtemplate.Block, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var blocks []emailtemplate.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return nil, invalidArgument("blocks must be a JSON array of {type,...}: %v", err)
	}
	return blocks, nil
}

func stylesFromJSON(raw string) (emailtemplate.EmailStyles, error) {
	var st emailtemplate.EmailStyles
	if strings.TrimSpace(raw) == "" {
		return st, nil
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, invalidArgument("styles must be a JSON object: %v", err)
	}
	return st, nil
}

// Nil normalizes to an empty array so the dashboard's JSON scalar reads `[]` rather than `null`
func blocksJSON(blocks []emailtemplate.Block) string {
	if blocks == nil {
		blocks = []emailtemplate.Block{}
	}
	raw, _ := json.Marshal(blocks)
	return string(raw)
}

func stylesJSON(st emailtemplate.EmailStyles) string {
	raw, _ := json.Marshal(st)
	return string(raw)
}

func optionalIDString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func optionalIDFrom(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	id, err := parseID(field, *raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func tsPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func timeFromTS(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.AsTime()
	return &v
}

func notFound(what string) error { return status.Errorf(codes.NotFound, "%s not found", what) }
