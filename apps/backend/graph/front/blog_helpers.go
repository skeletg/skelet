package front

// Helpers live out of blog.resolvers.go because gqlgen rewrites *.resolvers.go on regeneration and buries any non-resolver function in a comment block at the bottom

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
)

func channelIDFromContext(ctx context.Context) *uuid.UUID { // nil when no store was resolved - admin GraphQL traffic, introspection and the like
	if sv, ok := channel.FromContext(ctx); ok && sv != nil {
		id := sv.ID
		return &id
	}
	return nil
}

func parseUUIDOpt(s *string) (*uuid.UUID, error) { // surfaces the error, unlike graph.parseOptionalUUID which silently returns nil on a bad parse
	if s == nil || *s == "" {
		return nil, nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil, invalidID("")
	}
	return &u, nil
}

func intDeref(n *int32, def int32) int32 {
	if n == nil {
		return def
	}
	return *n
}

func unmarshalJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
