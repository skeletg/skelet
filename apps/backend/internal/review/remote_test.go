package review

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

func aggSource(t *testing.T, url string) RemoteAggregateSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindReviews, Slug: "yotpo-ish", Name: "Reviews", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteAggregateSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteAggregates(t *testing.T) {
	pid := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"aggregates": {"%s": {"count": 12, "average": 4.5, "distribution": [1,0,1,3,7]}}}`, pid)
	}))
	defer srv.Close()

	m, ok := aggSource(t, srv.URL).Aggregates(context.Background(), []uuid.UUID{pid})
	if !ok {
		t.Fatal("expected ok")
	}
	agg := m[pid]
	if agg.Count != 12 || agg.OverallAverage == nil || *agg.OverallAverage != 4.5 {
		t.Fatalf("aggregate wrong: %+v", agg)
	}
	if agg.Distribution != [5]int{1, 0, 1, 3, 7} {
		t.Fatalf("distribution wrong: %v", agg.Distribution)
	}
}

func TestRemoteAggregatesFailOpen(t *testing.T) {
	if _, ok := aggSource(t, "http://127.0.0.1:9/down").Aggregates(context.Background(), []uuid.UUID{uuid.New()}); ok {
		t.Fatal("down provider should fall back (ok=false)")
	}
}
