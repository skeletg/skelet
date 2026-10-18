package shipping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func TestAChannelsOwnTrackerAnswersForItsOrders(t *testing.T) {
	answer := func(status string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"` + status + `"}`))
		}))
	}
	global, own := answer("global"), answer("own")
	defer global.Close()
	defer own.Close()
	ch := uuid.New()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	for _, in := range []remoteprovider.CreateInput{
		{Kind: remoteprovider.KindShippingTracking, Slug: "a-global", Name: "Global", TargetURL: global.URL},
		{Kind: remoteprovider.KindShippingTracking, Slug: "z-own", Name: "Own", TargetURL: own.URL, ChannelID: &ch},
	} {
		if _, err := svc.Create(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{}
	s.SetRemoteTracking(NewRemoteTrackingSource(svc, remoteprovider.NewInvoker(svc, nil)))

	res, ok := s.trackRemoteFirst(context.WithValue(context.Background(), trackingChannelKey{}, ch), "ups", "1Z")
	if !ok || res.Status != "own" {
		t.Fatalf("tracked as the channel = %+v, %v, want its own provider's answer", res, ok)
	}
	if res, ok := s.trackRemoteFirst(context.Background(), "ups", "1Z"); !ok || res.Status != "global" {
		t.Fatalf("tracked with no channel = %+v, %v, want the global provider", res, ok)
	}
}
