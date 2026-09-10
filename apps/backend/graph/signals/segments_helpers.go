package signals

import (
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/graph/signals/model"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
)

// Kept out of the resolver file so gqlgen doesn't evict it on regeneration
func segmentToModel(s segment.Segment, memberCount int32) *model.CustomerSegment { // the caller fetches memberCount so a list doesn't COUNT(*) per row
	m := &model.CustomerSegment{
		ID:          s.ID.String(),
		Slug:        s.Slug,
		Name:        s.Name,
		Description: s.Description,
		IsActive:    s.IsActive,
		MemberCount: memberCount,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	if len(s.Rules) > 0 {
		var out any
		if err := json.Unmarshal(s.Rules, &out); err == nil {
			m.Rules = out
		}
	}
	return m
}
