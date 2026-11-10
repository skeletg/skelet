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
	got         signalsclient.ConversionSpec
}

func (f *fakeConversions) RecordConversion(ctx context.Context, cs signalsclient.ConversionSpec) error {
	_, f.hadDeadline = ctx.Deadline()
	f.got = cs
	return f.err
}

// A board adds attributed revenue up across currencies, so the conversion carries it in the base currency too - and
// without a rate it goes without, rather than with a wrong figure
func TestAConversionCarriesItsRevenueInTheBaseCurrency(t *testing.T) {
	cid := uuid.New()
	for name, tc := range map[string]struct {
		base rate
		want *int64
	}{
		"a known rate": {rate(2), ptr(int64(10000))},
		"no rate":      {rate(0), nil},
	} {
		bus := events.NewBus()
		rec := &fakeConversions{}
		RegisterConversionAttribution(bus, rec, tc.base)
		if err := bus.DispatchAsyncNow(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, TotalCents: 5000, Currency: "EUR"}); err != nil {
			t.Fatal(err)
		}
		if got := rec.got.RevenueBaseCents; (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: base revenue = %v, want %v", name, got, tc.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

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
			RegisterConversionAttribution(bus, rec, rate(0))
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
