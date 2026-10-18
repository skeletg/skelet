package enroll

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type enrollAnswer struct {
	signalsv1.UnimplementedFlowServiceServer
	res *signalsv1.EnrollResponse
	err error
}

func (f enrollAnswer) Enroll(context.Context, *signalsv1.EnrollRequest) (*signalsv1.EnrollResponse, error) {
	return f.res, f.err
}

func enrollerAgainst(t *testing.T, fake enrollAnswer) *Enroller {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	signalsv1.RegisterFlowServiceServer(srv, fake)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	c, err := signalsclient.New(ln.Addr().String(), auth.MustServiceKeys("test-secret"), time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return New(c)
}

func TestAMomentSomeFlowTookIsNotSentAgainByCore(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		fake enrollAnswer
		want int
	}{
		"partial":      {enrollAnswer{res: &signalsv1.EnrollResponse{Enrolled: 1, Incomplete: true}}, 1},
		"complete":     {enrollAnswer{res: &signalsv1.EnrollResponse{Enrolled: 2}}, 2},
		"none took it": {enrollAnswer{err: status.Error(codes.Internal, "db down")}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := enrollerAgainst(t, c.fake)
			got := e.OnOrderShipped(ctx, uuid.New(), uuid.New(), "a@b.test", uuid.New(), uuid.New(), email.OrderSnapshot{Number: "1"})
			if got != c.want {
				t.Fatalf("taken by %d flows, want %d", got, c.want)
			}
		})
	}
}

func TestAPartialEnrollIsStillAnErrorForAMomentThatCanBeRaisedAgain(t *testing.T) {
	e := enrollerAgainst(t, enrollAnswer{res: &signalsv1.EnrollResponse{Enrolled: 1, Incomplete: true}})
	if err := e.OnCustomerRegistered(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("a partial answer came back clean, so the flows that failed are never asked again")
	}
}
