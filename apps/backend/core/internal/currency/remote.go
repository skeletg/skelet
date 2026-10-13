package currency

import "context"

// An alternate FX source for the refresh job in place of the built-in frankfurter.dev fetch, so a merchant can point at their bank or paid feed without a Go plugin - the remoteprovider adapter lives in wiring
// fail_open, or no provider, falls back to frankfurter.dev; fail_closed aborts, keeping the last stored rates

type RemoteRateSource interface {
	// ok=false means no provider or fail_open, so use the built-in feed, while a non-nil error means a fail_closed provider failed
	Rates(ctx context.Context, base string, codes []string) (quote RateQuote, ok bool, err error)
}

// Units of each code per one unit of Base - Base may be blank when the source doesn't echo it
type RateQuote struct {
	Base  string
	Rates map[string]float64
}

func (s *Service) SetRemoteRateSource(src RemoteRateSource) { s.remoteRates = src }
