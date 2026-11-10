package enroll

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
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
	c, err := signalsclient.New(ln.Addr().String(), signalsclient.Transport{}, auth.MustServiceKeys("test-secret"), time.Minute, nil)
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

type enrollCapture struct {
	signalsv1.UnimplementedFlowServiceServer
	got chan *signalsv1.EnrollRequest
}

func (f enrollCapture) Enroll(_ context.Context, req *signalsv1.EnrollRequest) (*signalsv1.EnrollResponse, error) {
	f.got <- req
	return &signalsv1.EnrollResponse{Enrolled: 1}, nil
}

// The language a moment happened in rides along, so its email goes out in it - account mail from the storefront the
// shopper is browsing, order mail from the order
func TestMomentsCarryTheShoppersLanguage(t *testing.T) {
	fake := enrollCapture{got: make(chan *signalsv1.EnrollRequest, 3)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	signalsv1.RegisterFlowServiceServer(srv, fake)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	c, err := signalsclient.New(ln.Addr().String(), signalsclient.Transport{}, auth.MustServiceKeys("test-secret"), time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	e := New(c)

	browsing := locale.WithLocale(context.Background(), "de")
	e.OnPasswordReset(browsing, uuid.New(), uuid.New(), "https://shop.test/reset/x", "30 minutes", "Shop", "Ana")
	e.OnOrderPlaced(context.Background(), uuid.New(), uuid.New(), "a@b.test", uuid.New(), email.OrderSnapshot{Number: "1", Locale: "fr"})
	for _, want := range []string{`"locale":"de"`, `"locale":"fr"`} {
		if got := (<-fake.got).GetPayloadJson(); !strings.Contains(got, want) {
			t.Errorf("payload %s, want it to carry %s", got, want)
		}
	}
	// off the storefront nothing says which language, so the payload doesn't pretend one
	e.OnPasswordReset(context.Background(), uuid.New(), uuid.New(), "https://shop.test/reset/y", "30 minutes", "Shop", "Ana")
	if got := (<-fake.got).GetPayloadJson(); strings.Contains(got, `"locale"`) {
		t.Errorf("payload %s names a locale nobody gave", got)
	}
}
