package currency

import (
	"context"
	"fmt"
	"log"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// An alternate FX source for the refresh job (remoteprovider kind=currency_rate) in place of the built-in frankfurter.dev fetch, so a merchant can point at their bank or paid feed without a Go plugin
// fail_open, or no provider, falls back to frankfurter.dev; fail_closed aborts, keeping the last stored rates

type RemoteRateSource interface {
	// Returns code to rate against base - ok=false means no provider or fail_open, so use the built-in feed, while a non-nil error means a fail_closed provider failed
	Rates(ctx context.Context, base string, codes []string) (map[string]float64, bool, error)
}

func (s *Service) SetRemoteRateSource(src RemoteRateSource) { s.remoteRates = src }

type remoteRateRequest struct {
	Base       string   `json:"base"`
	Currencies []string `json:"currencies"`
}

type remoteRateResponse struct {
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}

type remoteRateSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteRateSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteRateSource {
	return &remoteRateSource{svc: svc, invoker: invoker}
}

func (r *remoteRateSource) Rates(ctx context.Context, base string, codes []string) (map[string]float64, bool, error) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindCurrencyRate, nil) // FX is global
	if err != nil {
		// Not the same as "no provider": falling through would silently reprice the whole catalog off the built-in public
		// feed instead of the merchant's authoritative rates, which is the opposite of what fail_closed asked for
		return nil, false, fmt.Errorf("currency provider lookup: %w", err)
	}
	if len(providers) == 0 {
		return nil, false, nil // no provider - built-in feed
	}
	p := providers[0]

	var resp remoteRateResponse
	if err := r.invoker.Invoke(ctx, p, remoteRateRequest{Base: base, Currencies: codes}, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return nil, false, err
		}
		log.Printf("[currency] remote rate provider %q failed (falling back to built-in feed): %v", p.Slug, err)
		return nil, false, nil
	}
	if len(resp.Rates) == 0 {
		return nil, false, nil
	}
	return resp.Rates, true, nil
}
