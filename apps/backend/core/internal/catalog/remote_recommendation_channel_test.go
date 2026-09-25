package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func TestAChannelsOwnRecommenderAnswersItsStorefront(t *testing.T) {
	want := uuid.New()
	own := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"productIds":["` + want.String() + `"]}`))
	}))
	defer own.Close()
	ch := uuid.New()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindRecommendation, Slug: "own", Name: "Own", TargetURL: own.URL, ChannelID: &ch,
	}); err != nil {
		t.Fatal(err)
	}
	src := NewRemoteRecommendationSource(svc, remoteprovider.NewInvoker(svc, nil))

	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: ch})
	if ids, ok := src.Recommend(ctx, uuid.New(), "also_bought", 4); !ok || len(ids) != 1 || ids[0] != want {
		t.Fatalf("the channel's storefront got %v, %v, want its own recommender's answer", ids, ok)
	}
	other := channel.WithChannel(context.Background(), &channel.Channel{ID: uuid.New()})
	if _, ok := src.Recommend(other, uuid.New(), "also_bought", 4); ok {
		t.Fatal("another channel's storefront was answered by a recommender scoped elsewhere")
	}
}
