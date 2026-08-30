package currency

// Exchange-rate refresh and conversion - rates come from frankfurter.dev (ECB, no key) and are stored per currency as units per ONE base-currency unit - The currency-rates job calls RefreshRates, and Convert is the shared cross-currency helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// Settable for tests
var ratesEndpoint = "https://api.frankfurter.dev/v1/latest"

// Fetches rates against base and stores one row per active currency - base is pinned at 1.0, and unknown currencies are skipped with a log line rather than failing
func (s *Service) RefreshRates(ctx context.Context, base string) error {
	base = strings.ToUpper(strings.TrimSpace(base))
	if base == "" {
		return fmt.Errorf("refresh rates: empty base currency")
	}
	list, err := s.repo.List(ctx, false)
	if err != nil {
		return fmt.Errorf("refresh rates: list currencies: %w", err)
	}
	if len(list) == 0 {
		return nil
	}

	// A configured remote provider replaces the built-in fetch, failing open to it unless the merchant set fail_closed
	if s.remoteRates != nil {
		codes := make([]string, 0, len(list))
		for _, c := range list {
			codes = append(codes, c.Code)
		}
		if rates, ok, err := s.remoteRates.Rates(ctx, base, codes); err != nil {
			return fmt.Errorf("refresh rates: remote provider: %w", err)
		} else if ok {
			return s.storeRates(ctx, list, base, rates)
		}
	}

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ratesEndpoint+"?base="+base, nil)
	if err != nil {
		return fmt.Errorf("refresh rates: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh rates: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh rates: provider status %d", resp.StatusCode)
	}
	var payload struct {
		Base  string             `json:"base"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("refresh rates: decode: %w", err)
	}

	return s.storeRates(ctx, list, base, payload.Rates)
}

// Writes one row per active currency from a base-relative rate map, shared by the built-in feed and the remote path - base pinned at 1.0, absent currencies skipped
func (s *Service) storeRates(ctx context.Context, list []Currency, base string, rates map[string]float64) error {
	for _, c := range list {
		switch c.Code {
		case base:
			if err := s.repo.SetRate(ctx, c.Code, 1); err != nil {
				return err
			}
		default:
			rate, ok := rates[c.Code]
			if !ok || rate <= 0 {
				slog.Info("currency-rates: no rate for currency; skipping", "code", c.Code, "base", base)
				continue
			}
			if err := s.repo.SetRate(ctx, c.Code, rate); err != nil {
				return err
			}
		}
	}
	return nil
}

// Translates FROM's minor units into TO's through the stored base-relative rates, honoring each currency's decimals - false when either rate is unset
func Convert(amountMinor int64, from, to Currency) (int64, bool) {
	if from.Code == to.Code {
		return amountMinor, true
	}
	if from.ExchangeRate == nil || to.ExchangeRate == nil || *from.ExchangeRate <= 0 || *to.ExchangeRate <= 0 {
		return 0, false
	}
	fromMajor := float64(amountMinor) / math.Pow10(int(from.Decimals))
	inBase := fromMajor / *from.ExchangeRate
	toMajor := inBase * *to.ExchangeRate
	return int64(math.Round(toMajor * math.Pow10(int(to.Decimals)))), true
}
