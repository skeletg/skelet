package sales

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeRepo struct {
	Repository // anything the test doesn't set up should panic rather than answer quietly

	summaries []CurrencySummary
	costs     map[string]int64
	sumErr    error
	costErr   error

	sawFilter Filter
	sawBucket string
	sawLimit  int32
	sawSKU    string
	costCalls int
}

func (f *fakeRepo) SummaryByCurrency(_ context.Context, flt Filter) ([]CurrencySummary, error) {
	f.sawFilter = flt
	return f.summaries, f.sumErr
}

func (f *fakeRepo) CostByCurrency(_ context.Context, flt Filter) (map[string]int64, error) {
	f.sawFilter = flt
	f.costCalls++
	return f.costs, f.costErr
}

func (f *fakeRepo) Timeseries(_ context.Context, flt Filter, bucket string) ([]TimePoint, error) {
	f.sawFilter, f.sawBucket = flt, bucket
	return nil, nil
}

func (f *fakeRepo) TopProducts(_ context.Context, flt Filter, limit int32) ([]ProductSales, error) {
	f.sawFilter, f.sawLimit = flt, limit
	return nil, nil
}

func (f *fakeRepo) ProductSalesBySKU(_ context.Context, flt Filter, sku string) (ProductSales, error) {
	f.sawFilter, f.sawSKU = flt, sku
	return ProductSales{}, nil
}

func someFilter() Filter {
	return Filter{
		Start:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:        time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		ChannelIDs: []uuid.UUID{uuid.New()},
	}
}

func TestSummaryComposesMarginFromSubtotalLessDiscountAndCost(t *testing.T) { // margin is the number an operator plans on, and every term dropped or double-counted here misstates it
	repo := &fakeRepo{
		summaries: []CurrencySummary{{
			CurrencyCode: "USD", OrderCount: 3, GrossRevenue: 15_000,
			SubtotalTotal: 12_000, DiscountTotal: 2_000, TaxTotal: 1_200, ShippingTotal: 1_800,
		}},
		costs: map[string]int64{"USD": 5_000},
	}

	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("%d summaries, want 1", len(got))
	}
	if got[0].CostTotal != 5_000 {
		t.Errorf("CostTotal = %d, want 5000", got[0].CostTotal)
	}
	if want := int64(5_000); got[0].MarginTotal != want { // 12000 - 2000 - 5000
		t.Errorf("MarginTotal = %d, want %d", got[0].MarginTotal, want)
	}
}

func TestSummaryMarginIgnoresTaxAndShipping(t *testing.T) { // tax is the state's money and shipping is the carrier's, so folding either into margin would inflate it
	base := CurrencySummary{CurrencyCode: "USD", SubtotalTotal: 10_000, DiscountTotal: 0}
	withExtras := base
	withExtras.TaxTotal, withExtras.ShippingTotal, withExtras.GrossRevenue = 9_999, 4_444, 24_443

	svc := NewService(&fakeRepo{summaries: []CurrencySummary{base}, costs: map[string]int64{"USD": 4_000}})
	plain, err := svc.Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	svc = NewService(&fakeRepo{summaries: []CurrencySummary{withExtras}, costs: map[string]int64{"USD": 4_000}})
	loaded, err := svc.Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if plain[0].MarginTotal != loaded[0].MarginTotal {
		t.Errorf("margin moved from %d to %d once tax and shipping were added", plain[0].MarginTotal, loaded[0].MarginTotal)
	}
}

func TestSummaryMatchesCostByCurrencyNotByPosition(t *testing.T) { // the two queries order their rows independently, so pairing them positionally would charge EUR sales against USD costs
	repo := &fakeRepo{
		summaries: []CurrencySummary{
			{CurrencyCode: "USD", SubtotalTotal: 10_000},
			{CurrencyCode: "EUR", SubtotalTotal: 20_000},
			{CurrencyCode: "GBP", SubtotalTotal: 30_000},
		},
		costs: map[string]int64{"GBP": 3_000, "EUR": 2_000, "USD": 1_000},
	}

	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := map[string]int64{"USD": 1_000, "EUR": 2_000, "GBP": 3_000}
	for _, s := range got {
		if s.CostTotal != want[s.CurrencyCode] {
			t.Errorf("%s CostTotal = %d, want %d", s.CurrencyCode, s.CostTotal, want[s.CurrencyCode])
		}
		if s.MarginTotal != s.SubtotalTotal-s.CostTotal {
			t.Errorf("%s MarginTotal = %d, want %d", s.CurrencyCode, s.MarginTotal, s.SubtotalTotal-s.CostTotal)
		}
	}
}

func TestSummaryTreatsAMissingCostRowAsZero(t *testing.T) { // most catalogs have no cost snapshots at all, and a missing row must read as "unknown cost" rather than dropping the currency from the report
	repo := &fakeRepo{
		summaries: []CurrencySummary{
			{CurrencyCode: "USD", SubtotalTotal: 10_000, DiscountTotal: 1_000},
			{CurrencyCode: "EUR", SubtotalTotal: 5_000},
		},
		costs: map[string]int64{"USD": 4_000},
	}

	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("%d summaries, want both currencies", len(got))
	}
	for _, s := range got {
		switch s.CurrencyCode {
		case "USD":
			if s.MarginTotal != 5_000 {
				t.Errorf("USD margin = %d, want 5000", s.MarginTotal)
			}
		case "EUR":
			if s.CostTotal != 0 || s.MarginTotal != 5_000 {
				t.Errorf("EUR cost = %d margin = %d, want 0 and the full 5000", s.CostTotal, s.MarginTotal)
			}
		}
	}
}

func TestSummaryWithNoCostsAtAll(t *testing.T) { // a nil map is what the repository returns when no line carries a cost snapshot
	repo := &fakeRepo{summaries: []CurrencySummary{{CurrencyCode: "USD", SubtotalTotal: 7_000, DiscountTotal: 500}}, costs: nil}
	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got[0].CostTotal != 0 || got[0].MarginTotal != 6_500 {
		t.Errorf("cost = %d margin = %d, want 0 and 6500", got[0].CostTotal, got[0].MarginTotal)
	}
}

func TestSummaryReportsANegativeMarginAsIs(t *testing.T) { // selling below cost is a real thing an operator needs to SEE, so clamping it at zero would hide the loss
	repo := &fakeRepo{
		summaries: []CurrencySummary{{CurrencyCode: "USD", SubtotalTotal: 1_000, DiscountTotal: 300}},
		costs:     map[string]int64{"USD": 2_000},
	}
	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got[0].MarginTotal != -1_300 {
		t.Errorf("MarginTotal = %d, want -1300", got[0].MarginTotal)
	}
}

func TestSummaryIsStableWhenRunTwice(t *testing.T) { // it writes margin back into the same rows, so a re-run over cached slices must not subtract cost a second time
	repo := &fakeRepo{
		summaries: []CurrencySummary{{CurrencyCode: "USD", SubtotalTotal: 10_000, DiscountTotal: 1_000}},
		costs:     map[string]int64{"USD": 4_000},
	}
	svc := NewService(repo)
	first, err := svc.Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	second, err := svc.Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("second Summary: %v", err)
	}
	if first[0].MarginTotal != second[0].MarginTotal {
		t.Errorf("margin drifted from %d to %d across identical calls", first[0].MarginTotal, second[0].MarginTotal)
	}
}

func TestSummaryPropagatesRepositoryErrors(t *testing.T) { // a half-answered report is worse than none - the dashboard would show a margin computed against no costs at all
	boom := errors.New("query timed out")

	repo := &fakeRepo{sumErr: boom}
	if _, err := NewService(repo).Summary(context.Background(), someFilter()); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the summary query's error", err)
	}
	if repo.costCalls != 0 {
		t.Error("the cost query ran after the summary query had already failed")
	}

	repo = &fakeRepo{summaries: []CurrencySummary{{CurrencyCode: "USD", SubtotalTotal: 1}}, costErr: boom}
	got, err := NewService(repo).Summary(context.Background(), someFilter())
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the cost query's error", err)
	}
	if got != nil {
		t.Errorf("summaries = %v alongside the error, want nil", got)
	}
}

func TestSummaryOnAnEmptyRange(t *testing.T) { // a range with no orders is the common case on a fresh channel, and it must read as an empty report rather than an error
	got, err := NewService(&fakeRepo{}).Summary(context.Background(), someFilter())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("summaries = %v, want none", got)
	}
}

func TestFilterReachesTheRepositoryUnchanged(t *testing.T) { // ChannelIDs IS the authorization scope here, so a service that widened, defaulted or dropped it would leak another channel's revenue
	scoped := Filter{
		Start:      time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		End:        time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC),
		ChannelIDs: []uuid.UUID{uuid.New(), uuid.New()},
	}
	ctx := context.Background()

	for name, call := range map[string]func(*Service, *fakeRepo){
		"Summary":     func(s *Service, _ *fakeRepo) { _, _ = s.Summary(ctx, scoped) },
		"Timeseries":  func(s *Service, _ *fakeRepo) { _, _ = s.Timeseries(ctx, scoped, "week") },
		"TopProducts": func(s *Service, _ *fakeRepo) { _, _ = s.TopProducts(ctx, scoped, 25) },
		"BySKU":       func(s *Service, _ *fakeRepo) { _, _ = s.ProductSalesBySKU(ctx, scoped, "SKU-1") },
	} {
		repo := &fakeRepo{}
		call(NewService(repo), repo)

		if repo.sawFilter.AllChannels {
			t.Errorf("%s: AllChannels = true, want the scoped filter passed through", name)
		}
		if len(repo.sawFilter.ChannelIDs) != len(scoped.ChannelIDs) {
			t.Errorf("%s: ChannelIDs = %v, want %v", name, repo.sawFilter.ChannelIDs, scoped.ChannelIDs)
		}
		if !repo.sawFilter.Start.Equal(scoped.Start) || !repo.sawFilter.End.Equal(scoped.End) {
			t.Errorf("%s: range = [%s, %s], want [%s, %s]", name, repo.sawFilter.Start, repo.sawFilter.End, scoped.Start, scoped.End)
		}
	}
}

func TestAnEmptyChannelSetIsNotWidenedToAll(t *testing.T) { // a scoped caller permitted no channels must see nothing - reading "no channels" as "every channel" is the whole-tenant leak
	repo := &fakeRepo{}
	_, _ = NewService(repo).Summary(context.Background(), Filter{ChannelIDs: nil})
	if repo.sawFilter.AllChannels {
		t.Error("AllChannels = true for a caller with no permitted channels")
	}
	if len(repo.sawFilter.ChannelIDs) != 0 {
		t.Errorf("ChannelIDs = %v, want none", repo.sawFilter.ChannelIDs)
	}
}

func TestPassthroughArgumentsSurviveTheCall(t *testing.T) { // the bucket is whitelisted upstream and interpolated into date_trunc downstream, so a service that rewrote it would silently change the grouping
	ctx := context.Background()

	repo := &fakeRepo{}
	_, _ = NewService(repo).Timeseries(ctx, someFilter(), "month")
	if repo.sawBucket != "month" {
		t.Errorf("bucket = %q, want month", repo.sawBucket)
	}

	repo = &fakeRepo{}
	_, _ = NewService(repo).TopProducts(ctx, someFilter(), 5)
	if repo.sawLimit != 5 {
		t.Errorf("limit = %d, want 5", repo.sawLimit)
	}

	repo = &fakeRepo{}
	_, _ = NewService(repo).ProductSalesBySKU(ctx, someFilter(), "TIRE-205-55-16")
	if repo.sawSKU != "TIRE-205-55-16" {
		t.Errorf("sku = %q, want TIRE-205-55-16", repo.sawSKU)
	}
}
