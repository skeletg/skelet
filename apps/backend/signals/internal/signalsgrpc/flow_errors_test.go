package signalsgrpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

func TestFlowErrorsTravelAsTheirCodes(t *testing.T) {
	for err, want := range map[error]codes.Code{
		flow.ErrNotFound: codes.NotFound,
		fmt.Errorf("%w: name is required", flow.ErrInvalid): codes.InvalidArgument,
		errors.New("node x not found in the database"):      codes.Unknown,
	} {
		if got := status.Code(flowErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
}
