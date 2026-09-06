package example

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Quote-only, so the host's capability type-assertion has a carrier that implements neither
// Tracker nor Labeler to detect - the shape most real carriers start as
type carrier struct{}

func (c *carrier) Quote(_ context.Context, req sdk.RateRequest) ([]sdk.RatedMethod, error) {
	return []sdk.RatedMethod{{
		Code:          "standard",
		Name:          "Example Standard",
		PriceCents:    500 + int64(len(req.Items))*100,
		EstimatedDays: 3,
	}}, nil
}
