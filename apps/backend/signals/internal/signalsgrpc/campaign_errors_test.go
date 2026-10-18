package signalsgrpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
)

// Core renders each code differently - a database failure must not read as the admin's mistake, nor a typo as an outage
func TestCampaignErrorsTravelAsTheirCodes(t *testing.T) {
	for err, want := range map[error]codes.Code{
		campaign.ErrNotFound: codes.NotFound,
		fmt.Errorf("%w: name is required", campaign.ErrInvalid):          codes.InvalidArgument,
		fmt.Errorf("%w: only drafts", campaign.ErrWrongState):            codes.FailedPrecondition,
		fmt.Errorf("%w: %w", campaign.ErrSendFailed, errors.New("smtp")): codes.Unavailable,
		errors.New("pq: connection refused not found"):                   codes.Unknown,
	} {
		if got := status.Code(campaignErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
}
