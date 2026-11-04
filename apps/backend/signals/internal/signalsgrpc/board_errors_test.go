package signalsgrpc

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/signals/internal/board"
)

func TestBoardErrorsTravelAsTheirCodes(t *testing.T) {
	for err, want := range map[error]codes.Code{
		board.ErrNotFound:                codes.NotFound,
		board.ErrStale:                   codes.FailedPrecondition,
		errors.Join(board.ErrInvalid):    codes.InvalidArgument,
		errors.New("connection refused"): codes.Unknown,
	} {
		if got := status.Code(boardErr(err)); got != want {
			t.Errorf("%v -> %v, want %v", err, got, want)
		}
	}
}
