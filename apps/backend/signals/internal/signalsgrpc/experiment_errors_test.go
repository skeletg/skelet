package signalsgrpc

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
)

// A validation message is the admin's to read - before this every one surfaced as an internal error
func TestExperimentErrorsTravelAsTheirCodes(t *testing.T) {
	for err, want := range map[error]codes.Code{
		experiment.ErrNotFound:           codes.NotFound,
		experiment.ErrKeyTaken:           codes.AlreadyExists,
		experiment.ErrLocked:             codes.FailedPrecondition,
		errors.New("connection refused"): codes.Unknown,
	} {
		if got := status.Code(experimentErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
	if got := status.Code(experimentErr(errors.Join(experiment.ErrInvalid))); got != codes.InvalidArgument {
		t.Errorf("invalid -> %v", got)
	}
}
