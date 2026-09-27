package activity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

type fakeConversions struct {
	err         error
	hadDeadline bool
}

func (f *fakeConversions) RecordConversion(ctx context.Context, _ signalsclient.ConversionSpec) error {
	_, f.hadDeadline = ctx.Deadline()
	return f.err
}

func TestConversionFailureReachesTheOutbox(t *testing.T) {
	cid := uuid.New()
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"recorded":    {nil, false},
		"unreachable": {status.Error(codes.Unavailable, "down"), true},
		"timed out":   {status.Error(codes.DeadlineExceeded, "slow"), true},
		"refused":     {status.Error(codes.InvalidArgument, "bad"), false},
		"no suite":    {signalsclient.ErrUnavailable, true},
	} {
		t.Run(name, func(t *testing.T) {
			bus := events.NewBus()
			rec := &fakeConversions{err: tc.err}
			RegisterConversionAttribution(bus, rec)
			err := bus.DispatchAsyncNow(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid})
			if (err != nil) != tc.wantErr {
				t.Fatalf("dispatch err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, tc.err) && status.Code(err) != status.Code(tc.err) {
				t.Errorf("err = %v does not carry the cause", err)
			}
			if !rec.hadDeadline {
				t.Error("conversion call ran without a deadline")
			}
		})
	}
}
