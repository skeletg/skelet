// Package sales is a read-only reporting model over order data - it backs the admin sales dashboard with aggregate queries and owns no writes, since order state is mutated only by the order subsystem
package sales

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// The common scope for every report - an inclusive [Start, End] range on order or transaction time plus a channel restriction
// AllChannels means no channel filter, for a superuser or global-scoped caller; otherwise rows are limited to ChannelIDs, so an empty set matches nothing, which is correct for a scoped caller with no permitted channels
type Filter struct {
	Start       time.Time
	End         time.Time
	AllChannels bool
	ChannelIDs  []uuid.UUID
}

// The KPI rollup for one currency over the range - money is int64 cents, and callers derive AOV as GrossRevenue / OrderCount
type CurrencySummary struct {
	CurrencyCode    string
	OrderCount      int32
	GrossRevenue    int64
	SubtotalTotal   int64
	DiscountTotal   int64
	TaxTotal        int64
	ShippingTotal   int64
	UniqueCustomers int32
	// Sums the order lines' unit-cost snapshots times quantity - lines without a snapshot contribute nothing, so treat margin as a floor
	CostTotal   int64
	MarginTotal int64
}

type TimePoint struct {
	Period       time.Time
	CurrencyCode string
	OrderCount   int32
	Revenue      int64
}

type ProductSales struct {
	SKU        string
	Name       string
	UnitsSold  int64
	Revenue    int64
	Cost       int64
	Margin     int64
	OrderCount int32
}

// The order count and revenue for one status, cancelled included - this is the distribution view, not a revenue figure
type StatusCount struct {
	Status     string
	OrderCount int32
	Revenue    int64
}

type RefundSummary struct {
	CurrencyCode   string
	RefundedAmount int64
	RefundCount    int32
}

// The DB surface the reporting model needs - read-only, backed by the generated sqlc aggregates in production and swappable in tests
type Repository interface {
	SummaryByCurrency(ctx context.Context, f Filter) ([]CurrencySummary, error)
	CostByCurrency(ctx context.Context, f Filter) (map[string]int64, error)
	Timeseries(ctx context.Context, f Filter, bucket string) ([]TimePoint, error)
	TopProducts(ctx context.Context, f Filter, limit int32) ([]ProductSales, error)
	ProductSalesBySKU(ctx context.Context, f Filter, sku string) (ProductSales, error)
	ByStatus(ctx context.Context, f Filter) ([]StatusCount, error)
	RefundTotals(ctx context.Context, f Filter) ([]RefundSummary, error)
}

// The application-facing reporting API - it delegates to the repository, since aggregation lives in SQL and there's no business logic here beyond passing validated inputs through
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Summary(ctx context.Context, f Filter) ([]CurrencySummary, error) {
	summaries, err := s.repo.SummaryByCurrency(ctx, f)
	if err != nil {
		return nil, err
	}
	costs, err := s.repo.CostByCurrency(ctx, f)
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		summaries[i].CostTotal = costs[summaries[i].CurrencyCode]
		summaries[i].MarginTotal = summaries[i].SubtotalTotal - summaries[i].DiscountTotal - summaries[i].CostTotal
	}
	return summaries, nil
}

// bucket must be a value Postgres date_trunc accepts - the resolver whitelists day, week and month
func (s *Service) Timeseries(ctx context.Context, f Filter, bucket string) ([]TimePoint, error) {
	return s.repo.Timeseries(ctx, f, bucket)
}

func (s *Service) TopProducts(ctx context.Context, f Filter, limit int32) ([]ProductSales, error) {
	return s.repo.TopProducts(ctx, f, limit)
}

// One product's rollup over the range - a product with no sales returns zeroes, not an error
func (s *Service) ProductSalesBySKU(ctx context.Context, f Filter, sku string) (ProductSales, error) {
	return s.repo.ProductSalesBySKU(ctx, f, sku)
}

func (s *Service) ByStatus(ctx context.Context, f Filter) ([]StatusCount, error) {
	return s.repo.ByStatus(ctx, f)
}

func (s *Service) RefundTotals(ctx context.Context, f Filter) ([]RefundSummary, error) {
	return s.repo.RefundTotals(ctx, f)
}
