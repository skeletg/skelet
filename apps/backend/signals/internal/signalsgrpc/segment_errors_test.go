package signalsgrpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

func TestSegmentErrorsTravelAsTheirCodes(t *testing.T) {
	for err, want := range map[error]codes.Code{
		segment.ErrSegmentNotFound:                               codes.NotFound,
		fmt.Errorf("%w: a name is required", segment.ErrInvalid): codes.InvalidArgument,
		fmt.Errorf("create segment: %w", segment.ErrSlugTaken):   codes.AlreadyExists,
		segment.ErrSlugImmutable:                                 codes.FailedPrecondition,
		errors.New("connection refused"):                         codes.Unknown,
	} {
		if got := status.Code(segmentErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
}
