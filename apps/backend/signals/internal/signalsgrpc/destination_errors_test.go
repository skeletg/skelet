package signalsgrpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
)

// Core renders each code differently, so an admin's mistake has to arrive as theirs rather than as an outage
func TestDestinationErrorsTravelAsTheirCodes(t *testing.T) {
	plain := errors.New("db down")
	for err, want := range map[error]codes.Code{
		fmt.Errorf("%w: name is required", destination.ErrInvalid): codes.InvalidArgument,
		destination.ErrNotFound:             codes.NotFound,
		destination.ErrAudienceNeedsWebhook: codes.FailedPrecondition,
	} {
		if got := status.Code(destinationErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
	if got := destinationErr(plain); !errors.Is(got, plain) || status.Code(got) != codes.Unknown {
		t.Error("an unrecognised error should pass through untouched")
	}
}
