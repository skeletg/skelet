package front

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

func TestAssistantLinksComeOutAsOneAddressEachTime(t *testing.T) {
	huge := math.MaxInt32 + 10
	links := assistantLinks([]shoptools.Link{
		{Label: "Navy jackets", Path: "/shop/jackets", Filters: map[string][]string{"size": {"m"}, "color": {"navy"}, "brand": {}}, Count: &huge},
		{Label: "Returns", Path: "/info/returns"},
	})
	if len(links) != 2 {
		t.Fatalf("got %d links", len(links))
	}
	got := links[0]
	if len(got.Filters) != 2 || got.Filters[0].Slug != "color" || got.Filters[1].Slug != "size" {
		t.Errorf("filters = %+v - slug-sorted, empty ones dropped", got.Filters)
	}
	if got.Count == nil || *got.Count != math.MaxInt32 {
		t.Errorf("count = %v, want it clamped to an Int", got.Count)
	}
	if page := links[1]; page.Count != nil || page.Filters == nil || len(page.Filters) != 0 {
		t.Errorf("a page link = %+v - no count, and an empty (not null) filter list", page)
	}
}

func TestNoStoreOrNoAssistantMeansNoButton(t *testing.T) {
	r := &queryResolver{&Resolver{}}
	if shown, err := r.AssistantAvailable(context.Background()); shown || err != nil {
		t.Errorf("without a store = %v, %v", shown, err)
	}
	if shown, err := r.AssistantAvailable(channel.WithChannel(context.Background(), &channel.Channel{ID: uuid.New()})); shown || err != nil {
		t.Errorf("without an assistant wired = %v, %v", shown, err)
	}
}
