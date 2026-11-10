package catalog

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

func ptr[T any](v T) *T { return &v }

func TestTheSaleRanksByTheShareTakenOff(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	quarter, half, tenth, expired, noise, upcoming := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	cands := []SaleCandidate{
		{quarter, pricing.Inputs{BasePrice: 7500, CompareAt: ptr[int64](10000)}},
		{tenth, pricing.Inputs{BasePrice: 90000, CompareAt: ptr[int64](100000)}},
		{half, pricing.Inputs{BasePrice: 2000, Special: ptr[int64](1000)}},
		{expired, pricing.Inputs{BasePrice: 2000, Special: ptr[int64](500), SpecialTo: ptr(now.Add(-time.Hour))}},
		{upcoming, pricing.Inputs{BasePrice: 2000, Special: ptr[int64](500), SpecialFrom: ptr(now.Add(time.Hour))}},
		{noise, pricing.Inputs{BasePrice: 5000, CompareAt: ptr[int64](4000)}},
	}
	got := rankSales(cands, now, 10)
	want := []uuid.UUID{half, quarter, tenth}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v - an over-priced compare-at or a special outside its window isn't a sale", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d is %v, want %v", i, got[i], want[i])
		}
	}
	if top := rankSales(cands, now, 1); len(top) != 1 || top[0] != half {
		t.Errorf("limit 1 = %v", top)
	}
}
