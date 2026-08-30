package order

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakePageRepo struct {
	Repository
	saw ListPageFilter
}

func (f *fakePageRepo) ListOrdersPage(_ context.Context, flt ListPageFilter) ([]Order, int64, error) {
	f.saw = flt
	return []Order{}, 7, nil
}

func TestListOrdersPageClampsItsInputs(t *testing.T) { // an unclamped limit is the full-table read this pager exists to prevent
	for name, tc := range map[string]struct {
		limit, offset         int32
		wantLimit, wantOffset int32
	}{
		"unset":        {0, 0, 50, 0},
		"negative":     {-3, -9, 50, 0},
		"normal":       {20, 40, 20, 40},
		"over the cap": {10000, 0, 200, 0},
	} {
		repo := &fakePageRepo{}
		s := &Service{repo: repo}
		if _, _, err := s.ListOrdersPage(context.Background(), ListPageFilter{Limit: tc.limit, Offset: tc.offset}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if repo.saw.Limit != tc.wantLimit || repo.saw.Offset != tc.wantOffset {
			t.Errorf("%s: repo saw limit=%d offset=%d, want %d/%d", name, repo.saw.Limit, repo.saw.Offset, tc.wantLimit, tc.wantOffset)
		}
	}
}

func TestListOrdersPagePassesTheScopeThrough(t *testing.T) { // ChannelIDs is the authorization boundary - widening or dropping it leaks another store's orders
	repo := &fakePageRepo{}
	s := &Service{repo: repo}
	ch := uuid.New()
	f := ListPageFilter{
		ChannelIDs:      []uuid.UUID{ch},
		Search:          "  ORD-000042  ",
		Statuses:        []string{"pending", "confirmed"},
		PaymentStatuses: []string{"unpaid"},
		Limit:           25,
	}
	_, total, err := s.ListOrdersPage(context.Background(), f)
	if err != nil {
		t.Fatalf("ListOrdersPage: %v", err)
	}
	if total != 7 {
		t.Errorf("total = %d, want the repo's 7", total)
	}
	if len(repo.saw.ChannelIDs) != 1 || repo.saw.ChannelIDs[0] != ch {
		t.Errorf("channels = %v, want the caller's scope untouched", repo.saw.ChannelIDs)
	}
	if repo.saw.Search != "ORD-000042" {
		t.Errorf("search = %q, want it trimmed", repo.saw.Search)
	}
	if len(repo.saw.Statuses) != 2 || len(repo.saw.PaymentStatuses) != 1 {
		t.Errorf("filters = %v / %v, want them passed through", repo.saw.Statuses, repo.saw.PaymentStatuses)
	}
}
