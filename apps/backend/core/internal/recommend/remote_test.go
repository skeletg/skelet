package recommend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func recSource(t *testing.T, url string) RemoteRecommendationSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindRecommendation, Slug: "recs", Name: "Recs", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteRecommendationSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteRecommend(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"productIds": ["%s", "%s"]}`, a, b)
	}))
	defer srv.Close()

	ids, ok := recSource(t, srv.URL).Recommend(context.Background(), uuid.New(), StrategyAlsoBought, 8)
	if !ok || len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Fatalf("recommend: ok=%v ids=%v", ok, ids)
	}
}

func TestRemoteRecommendFailOpen(t *testing.T) {
	if _, ok := recSource(t, "http://127.0.0.1:9/down").Recommend(context.Background(), uuid.New(), StrategyAlsoBought, 8); ok {
		t.Fatal("down provider should fall back (ok=false)")
	}
}

func TestRemoteRecommendEmptyIsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"productIds": []}`))
	}))
	defer srv.Close()
	if _, ok := recSource(t, srv.URL).Recommend(context.Background(), uuid.New(), StrategyAlsoBought, 8); ok {
		t.Fatal("empty result should fall back to native (ok=false)")
	}
}

func TestRemoteRecommendDropsSelfRepeatsAndOverflow(t *testing.T) {
	self, a, b, c := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"productIds": ["%s", "%s", "bogus", "%s", "%s", "%s"]}`, self, a, a, b, c)
	}))
	defer srv.Close()

	ids, ok := recSource(t, srv.URL).Recommend(context.Background(), self, StrategyAlsoBought, 2)
	if !ok || len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Fatalf("recommend = %v (ok=%v), want [a b] - no self, no repeat, capped at the limit", ids, ok)
	}
}
