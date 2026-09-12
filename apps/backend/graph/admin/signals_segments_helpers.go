package admin

// Converters between the segment contract and the admin models

import (
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func customerSegmentModel(s *signalsv1.CustomerSegment) *model.CustomerSegment {
	m := &model.CustomerSegment{
		ID:          s.GetId(),
		Slug:        s.GetSlug(),
		Name:        s.GetName(),
		Description: s.GetDescription(),
		IsActive:    s.GetIsActive(),
		MemberCount: s.GetMemberCount(),
		CreatedAt:   s.GetCreatedAt().AsTime(),
		UpdatedAt:   s.GetUpdatedAt().AsTime(),
	}
	// The rule tree is opaque on the wire - core stores and forwards it without knowing the grammar
	if raw := s.GetRulesJson(); raw != "" {
		var out any
		if err := json.Unmarshal([]byte(raw), &out); err == nil {
			m.Rules = out
		}
	}
	return m
}
