package admin

// Shared plumbing for the Pulse resolvers - core relays the suite's flows, campaigns and email builder with the caller's own grants
// Block documents, themes and field schemas travel as JSON text, so core never decodes them

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// The dashboard's JSON scalar onto the wire's string - nil stays empty, which the suite reads as absent
func jsonText(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode json: %w", err)
	}
	return string(raw), nil
}

// The wire's string back onto the JSON scalar - empty is the given fallback, so a document never reads as null
func jsonValue(raw, fallback string) any {
	if raw == "" {
		raw = fallback
	}
	return json.RawMessage(raw)
}

func protoTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

func protoTimePtr(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.AsTime()
	return &v
}

func protoTimeFrom(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func optionalStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func optionalInt(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
