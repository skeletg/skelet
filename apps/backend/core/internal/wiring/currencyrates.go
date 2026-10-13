package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const baseChangeRefreshTimeout = time.Minute

// Adapts a remoteprovider kind=currency_rate endpoint to currency.RemoteRateSource
type currencyRemoteRates struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewCurrencyRemoteRates(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) currency.RemoteRateSource {
	return &currencyRemoteRates{svc: svc, invoker: invoker}
}

type remoteRateRequest struct {
	Base       string   `json:"base"`
	Currencies []string `json:"currencies"`
}

type remoteRateResponse struct {
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}

func (r *currencyRemoteRates) Rates(ctx context.Context, base string, codes []string) (currency.RateQuote, bool, error) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindCurrencyRate, nil) // FX is global
	if err != nil {
		// Not the same as "no provider": falling through would silently reprice the whole catalog off the built-in public
		// feed instead of the merchant's authoritative rates, which is the opposite of what fail_closed asked for
		return currency.RateQuote{}, false, fmt.Errorf("currency provider lookup: %w", err)
	}
	if len(providers) == 0 {
		return currency.RateQuote{}, false, nil
	}
	p := providers[0]

	var resp remoteRateResponse
	if err := r.invoker.Invoke(ctx, p, remoteRateRequest{Base: base, Currencies: codes}, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return currency.RateQuote{}, false, err
		}
		slog.WarnContext(ctx, "currency: remote rate provider failed, falling back to the built-in feed", "provider", p.Slug, "err", err)
		return currency.RateQuote{}, false, nil
	}
	if len(resp.Rates) == 0 {
		return currency.RateQuote{}, false, nil
	}
	return currency.RateQuote{Base: resp.Base, Rates: resp.Rates}, true, nil
}

type baseCurrencyReader interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type rateRefresher interface {
	RefreshRates(ctx context.Context, base string) error
}

// Refreshes against the configured base - a failed settings read is an error, not a quiet refresh against the default
func refreshCurrencyRates(ctx context.Context, cfg baseCurrencyReader, rates rateRefresher) error {
	base, err := cfg.GetString(ctx, config.BaseCurrencyPath, config.DefaultScope)
	if err != nil {
		return fmt.Errorf("read base currency: %w", err)
	}
	if base == "" {
		base = config.DefaultBaseCurrency
	}
	return rates.RefreshRates(ctx, base)
}

type configChangeNotifier interface {
	OnChange(fn func(ctx context.Context, def config.Definition))
}

// Stored rates are quoted against the old base until the next tick, so a base change refreshes at once - off the save's goroutine, so a slow feed never holds up the admin
func registerBaseCurrencyRefresh(cfg configChangeNotifier, reader baseCurrencyReader, rates rateRefresher) {
	cfg.OnChange(func(ctx context.Context, def config.Definition) {
		if def.Path != config.BaseCurrencyPath {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), baseChangeRefreshTimeout)
			defer cancel()
			if err := refreshCurrencyRates(ctx, reader, rates); err != nil {
				slog.ErrorContext(ctx, "currency: rate refresh after a base change failed", "err", err)
			}
		}()
	})
}

func wireCurrencyRates(cfg *config.Service, cur *currency.Service) {
	registerBaseCurrencyRefresh(cfg, cfg, cur)
}
