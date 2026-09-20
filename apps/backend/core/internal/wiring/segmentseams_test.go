package wiring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

func segSource(t *testing.T, url string) RemoteSegments {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindCustomerSegments, Slug: "cdp", Name: "CDP", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteSegmentSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteSegments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"segments": ["vip", "lapsed"]}`))
	}))
	defer srv.Close()

	slugs, ok := segSource(t, srv.URL).Slugs(context.Background(), uuid.New())
	if !ok || len(slugs) != 2 || slugs[0] != "vip" {
		t.Fatalf("segments: ok=%v slugs=%v", ok, slugs)
	}
}

func TestRemoteSegmentsFailOpen(t *testing.T) {
	if _, ok := segSource(t, "http://127.0.0.1:9/down").Slugs(context.Background(), uuid.New()); ok {
		t.Fatal("down provider should fall back to native (ok=false)")
	}
}

// nativeSegments stands in for the contract client - what the suite answers about one shopper
type nativeSegments struct {
	refs []signalsclient.SegmentRef
	err  error
}

func (n nativeSegments) CustomerSegments(context.Context, uuid.UUID) ([]signalsclient.SegmentRef, error) {
	return n.refs, n.err
}
func (nativeSegments) SegmentMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (nativeSegments) AssignSegmentMembers(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}

type fixedRemote struct {
	slugs []string
	ok    bool
}

func (f fixedRemote) Slugs(context.Context, uuid.UUID) ([]string, bool) { return f.slugs, f.ok }

// The union is core's to make, since the suite never sees a merchant's provider rows
// A slug both sides answer counts once, and a CDP-only slug still gates a promotion
func TestSegmentLookupUnionsNativeAndRemoteAudiences(t *testing.T) {
	native := nativeSegments{refs: []signalsclient.SegmentRef{{ID: uuid.New(), Slug: "vip"}}}
	l := NewSegmentLookup(native, fixedRemote{slugs: []string{"vip", "lapsed", ""}, ok: true})

	slugs, err := l.SlugsForCustomer(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("SlugsForCustomer: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "vip" || slugs[1] != "lapsed" {
		t.Errorf("slugs = %v, want [vip lapsed] - native first, remote unioned, blanks and duplicates dropped", slugs)
	}
}

// Either side failing leaves the other's answer standing - an unreachable suite must not silence a
// CDP audience, and a down CDP must not silence the native ones
func TestSegmentLookupFailsOpenOnEitherSide(t *testing.T) {
	down := NewSegmentLookup(nativeSegments{err: errors.New("suite unreachable")}, fixedRemote{slugs: []string{"lapsed"}, ok: true})
	slugs, _ := down.SlugsForCustomer(context.Background(), uuid.New())
	if len(slugs) != 1 || slugs[0] != "lapsed" {
		t.Errorf("suite down: slugs = %v, want the CDP's [lapsed]", slugs)
	}

	cdpDown := NewSegmentLookup(nativeSegments{refs: []signalsclient.SegmentRef{{Slug: "vip"}}}, fixedRemote{ok: false})
	slugs, _ = cdpDown.SlugsForCustomer(context.Background(), uuid.New())
	if len(slugs) != 1 || slugs[0] != "vip" {
		t.Errorf("CDP down: slugs = %v, want the native [vip]", slugs)
	}

	none := NewSegmentLookup(nativeSegments{refs: []signalsclient.SegmentRef{{Slug: "vip"}}}, nil)
	slugs, _ = none.SlugsForCustomer(context.Background(), uuid.New())
	if len(slugs) != 1 || slugs[0] != "vip" {
		t.Errorf("no CDP wired: slugs = %v, want the native [vip]", slugs)
	}
}
