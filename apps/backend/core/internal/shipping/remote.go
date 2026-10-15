package shipping

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Rate sources implemented as merchant HTTP endpoints (remoteprovider kind=shipping_carrier) rather than registry entries, so a merchant can plug in a 3PL or negotiated-rate service without a Go plugin QuoteAll consults them alongside registered carriers and PriceSelected re-quotes at placement Wire contract: request {channelId, currency, subtotalCents, destination, items, parcels}, response {rates: [{code, name, amountCents, currency, etaDays}]} A provider that errors, times out or trips its circuit is skipped, so a flaky rate API never blanks checkout

// The seam QuoteAll and PriceSelected use for remote providers, wired through SetRemoteRateSource so shipping's tests can skip the remoteprovider stack
type RemoteRateSource interface {
	// Every remote carrier's rated methods, and the slugs of those that failed - never errors, since failures are skipped
	Quotes(ctx context.Context, req RateRequest) ([]RatedQuote, []string)
	// Re-quotes one remote carrier for the chosen code, or ErrMethodNotFound
	Price(ctx context.Context, slug, code string, req RateRequest) (int64, error)
}

// Optional - nil leaves rate collection to the registry carriers only
func (s *Service) SetRemoteRateSource(src RemoteRateSource) { s.remoteRates = src }

type remoteRateRequest struct {
	ChannelID     string           `json:"channelId"`
	Currency      string           `json:"currency"`
	SubtotalCents int64            `json:"subtotalCents"`
	Destination   remoteAddress    `json:"destination"`
	Items         []remoteRateItem `json:"items"`
	Parcels       []remoteRateBox  `json:"parcels,omitempty"`
}

type remoteAddress struct {
	Street1     string `json:"street1"`
	Street2     string `json:"street2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postalCode"`
	CountryCode string `json:"countryCode"`
}

type remoteRateItem struct {
	SKU            string  `json:"sku"`
	Quantity       float64 `json:"quantity"`
	UnitPriceCents int64   `json:"unitPriceCents"`
	WeightGrams    int32   `json:"weightGrams,omitempty"`
	LengthMm       int32   `json:"lengthMm,omitempty"`
	WidthMm        int32   `json:"widthMm,omitempty"`
	HeightMm       int32   `json:"heightMm,omitempty"`
}

type remoteRateBox struct {
	WeightGrams int64 `json:"weightGrams"`
	LengthMm    int32 `json:"lengthMm"`
	WidthMm     int32 `json:"widthMm"`
	HeightMm    int32 `json:"heightMm"`
}

type remoteRate struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
	EtaDays     int    `json:"etaDays"`
}

type remoteRateResponse struct {
	Rates []remoteRate `json:"rates"`
}

func buildRemoteRateRequest(req RateRequest) remoteRateRequest {
	items := make([]remoteRateItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = remoteRateItem(it) // same fields, differ only in JSON tags
	}
	var parcels []remoteRateBox
	for _, p := range req.Parcels {
		parcels = append(parcels, remoteRateBox{
			WeightGrams: p.WeightGrams, LengthMm: p.LengthMm, WidthMm: p.WidthMm, HeightMm: p.HeightMm,
		})
	}
	return remoteRateRequest{
		ChannelID:     req.ChannelID.String(),
		Currency:      req.CurrencyCode,
		SubtotalCents: req.SubtotalCents,
		Destination: remoteAddress{
			Street1: req.Destination.Street1, Street2: req.Destination.Street2,
			City: req.Destination.City, Region: req.Destination.Region,
			PostalCode: req.Destination.PostalCode, CountryCode: req.Destination.CountryCode,
		},
		Items:   items,
		Parcels: parcels,
	}
}

// The default RemoteRateSource, backed by the remoteprovider registry and invoker
type remoteRateSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteRateSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteRateSource {
	return &remoteRateSource{svc: svc, invoker: invoker}
}

func (r *remoteRateSource) providers(ctx context.Context, channelID uuid.UUID) []remoteprovider.Provider {
	ch := channelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindShippingCarrier, &ch)
	if err != nil {
		log.Printf("[shipping] list remote carriers: %v", err)
		return nil
	}
	return providers
}

func (r *remoteRateSource) Quotes(ctx context.Context, req RateRequest) ([]RatedQuote, []string) {
	providers := r.providers(ctx, req.ChannelID)
	if len(providers) == 0 {
		return nil, nil
	}
	payload := buildRemoteRateRequest(req)

	var (
		mu     sync.Mutex
		out    []RatedQuote
		failed []string
		wg     sync.WaitGroup
	)
	fail := func(slug string) {
		mu.Lock()
		failed = append(failed, slug)
		mu.Unlock()
	}
	for _, p := range providers {
		wg.Add(1)
		go func(p remoteprovider.Provider) {
			defer wg.Done()
			// same posture as the plugin-carrier fan-out in carriers.go - a misbehaving provider must
			// not crash checkout
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[shipping] remote carrier %q quote panicked (skipped): %v", p.Slug, rec)
					fail(p.Slug)
				}
			}()
			var resp remoteRateResponse
			if err := r.invoker.Invoke(ctx, p, payload, &resp); err != nil {
				// Fail-open - a broken rate endpoint never blanks checkout
				log.Printf("[shipping] remote carrier %q quote failed (skipped): %v", p.Slug, err)
				fail(p.Slug)
				return
			}
			mu.Lock()
			for _, rate := range resp.Rates {
				out = append(out, RatedQuote{
					CarrierSlug: p.Slug,
					CarrierName: p.Name,
					Method: RatedMethod{
						Code: rate.Code, Name: rate.Name,
						PriceCents: rate.AmountCents, EstimatedDays: rate.EtaDays,
					},
				})
			}
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return out, failed
}

func (r *remoteRateSource) Price(ctx context.Context, slug, code string, req RateRequest) (int64, error) {
	ch := req.ChannelID
	p, err := r.svc.GetBySlug(ctx, remoteprovider.KindShippingCarrier, slug, &ch)
	if err != nil || !p.IsActive {
		return 0, ErrMethodNotFound
	}
	// A store-scoped provider only prices its own channel
	if p.ChannelID != nil && *p.ChannelID != ch {
		return 0, ErrMethodNotFound
	}
	var resp remoteRateResponse
	if err := r.invoker.Invoke(ctx, p, buildRemoteRateRequest(req), &resp); err != nil {
		// Placement is fail-CLOSED on the selected method (customer re-picks if
		// their carrier is down), even though quoting fails open
		return 0, ErrMethodNotFound
	}
	for _, rate := range resp.Rates {
		if rate.Code == code {
			return rate.AmountCents, nil
		}
	}
	return 0, ErrMethodNotFound
}
