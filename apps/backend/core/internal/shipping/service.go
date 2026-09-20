package shipping

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
	// remoteRates adds remote providers to QuoteAll/PriceSelected, nil = registry only (remote.go)
	remoteRates RemoteRateSource
	// remoteTracking resolves tracking via a universal tracker before the carrier's own (remote_tracking.go)
	remoteTracking RemoteTrackingSource
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Repo() Repository { return s.repo }

// Every active method whose zone covers the country, priced against the subtotal, in sort order - empty means no zone covers it, and the storefront shows "delivery not available"
func (s *Service) Quote(ctx context.Context, in QuoteInput) ([]Quote, error) {
	if in.ChannelID == uuid.Nil || in.CountryCode == "" {
		return nil, nil
	}
	zone, err := s.repo.FindZoneForCountry(ctx, in.ChannelID, in.CountryCode)
	if err != nil {
		if errors.Is(err, ErrZoneNotFound) {
			return nil, nil
		}
		return nil, err
	}
	methods, err := s.repo.ListActiveMethodsByZone(ctx, zone.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Quote, 0, len(methods))
	for _, m := range methods {
		ok, err := s.methodAllowsCountry(ctx, m, in.CountryCode)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, Quote{
			MethodID:   m.ID,
			Code:       m.Code,
			Name:       m.Name,
			PriceCents: ComputePrice(m, in.SubtotalCents),
		})
	}
	return out, nil
}

// The lowest free_above_cents among active, country-allowed 'free_above' methods, powering the "add $X for free shipping" nudge - 0 means none applies, and an empty country scans every zone for a channel-wide estimate
func (s *Service) FreeShippingThreshold(ctx context.Context, channelID uuid.UUID, country string) (int64, error) {
	if channelID == uuid.Nil {
		return 0, nil
	}
	country = strings.ToUpper(strings.TrimSpace(country))

	// Zones to scan - the one covering the country, or every zone when none is known yet
	var zones []Zone
	if country != "" {
		zone, err := s.repo.FindZoneForCountry(ctx, channelID, country)
		if err != nil {
			if errors.Is(err, ErrZoneNotFound) {
				return 0, nil
			}
			return 0, err
		}
		zones = []Zone{zone}
	} else {
		all, err := s.repo.ListZonesByChannel(ctx, channelID)
		if err != nil {
			return 0, err
		}
		zones = all
	}

	var threshold int64
	for _, zone := range zones {
		methods, err := s.repo.ListActiveMethodsByZone(ctx, zone.ID)
		if err != nil {
			return 0, err
		}
		for _, m := range methods {
			if m.CalcType != CalcFreeAbove || m.FreeAboveCents <= 0 {
				continue
			}
			// With a known country, respect per-method allow-lists; the channel-wide estimate takes any
			if country != "" {
				ok, err := s.methodAllowsCountry(ctx, m, country)
				if err != nil {
					return 0, err
				}
				if !ok {
					continue
				}
			}
			if threshold == 0 || m.FreeAboveCents < threshold {
				threshold = m.FreeAboveCents
			}
		}
	}
	return threshold, nil
}

// Always true for mode "all", else only when the country is in the per-method allow-list, case-insensitively
func (s *Service) methodAllowsCountry(ctx context.Context, m Method, country string) (bool, error) {
	if m.CountryMode != "specific" {
		return true, nil
	}
	countries, err := s.repo.ListCountriesForMethod(ctx, m.ID)
	if err != nil {
		return false, err
	}
	for _, c := range countries {
		if strings.EqualFold(c, country) {
			return true, nil
		}
	}
	return false, nil
}

// The authoritative price at placement - verifies methodCode still exists and is active for the zone, then prices it with the live formula rather than the client's cents - ErrMethodNotFound means "option no longer available, retry"
func (s *Service) ComputeForOrder(ctx context.Context, methodCode string, in QuoteInput) (int64, Method, error) {
	if in.ChannelID == uuid.Nil || in.CountryCode == "" {
		return 0, Method{}, fmt.Errorf("shipping: channel and country required")
	}
	zone, err := s.repo.FindZoneForCountry(ctx, in.ChannelID, in.CountryCode)
	if err != nil {
		return 0, Method{}, err
	}
	method, err := s.repo.GetMethodByCode(ctx, zone.ID, methodCode)
	if err != nil {
		return 0, Method{}, err
	}
	if !method.IsActive {
		return 0, Method{}, ErrMethodNotFound
	}
	if ok, err := s.methodAllowsCountry(ctx, method, in.CountryCode); err != nil {
		return 0, Method{}, err
	} else if !ok {
		return 0, Method{}, ErrMethodNotFound
	}
	return ComputePrice(method, in.SubtotalCents), method, nil
}

// The ID-based sibling of ComputeForOrder - verifies the method belongs to the country's zone, so one from another zone can't be smuggled in, and is still active
func (s *Service) ComputeForOrderByID(ctx context.Context, methodID uuid.UUID, in QuoteInput) (int64, Method, error) {
	if in.ChannelID == uuid.Nil || in.CountryCode == "" {
		return 0, Method{}, fmt.Errorf("shipping: channel and country required")
	}
	zone, err := s.repo.FindZoneForCountry(ctx, in.ChannelID, in.CountryCode)
	if err != nil {
		return 0, Method{}, err
	}
	method, err := s.repo.GetMethod(ctx, methodID)
	if err != nil {
		return 0, Method{}, err
	}
	if method.ZoneID != zone.ID || !method.IsActive {
		return 0, Method{}, ErrMethodNotFound
	}
	if ok, err := s.methodAllowsCountry(ctx, method, in.CountryCode); err != nil {
		return 0, Method{}, err
	} else if !ok {
		return 0, Method{}, ErrMethodNotFound
	}
	return ComputePrice(method, in.SubtotalCents), method, nil
}

// Fills Zone.Countries per zone - the bare ListZones omits them, being a separate table, so the admin surface uses this to return both together
func (s *Service) HydrateZoneCountries(ctx context.Context, zones []Zone) ([]Zone, error) {
	for i := range zones {
		countries, err := s.repo.ListCountriesForZone(ctx, zones[i].ID)
		if err != nil {
			return nil, err
		}
		zones[i].Countries = countries
	}
	return zones, nil
}

// Rewrites a zone's country membership - the caller wraps it in repo.WithTx for atomicity
func (s *Service) ReplaceZoneCountries(ctx context.Context, zoneID uuid.UUID, countries []string) error {
	if err := s.repo.ClearZoneCountries(ctx, zoneID); err != nil {
		return err
	}
	for _, c := range countries {
		if c == "" {
			continue
		}
		if err := s.repo.AddCountryToZone(ctx, zoneID, c); err != nil {
			return err
		}
	}
	return nil
}

// Fills Method.AllowedCountries for "specific"-mode methods, leaving "all" empty
func (s *Service) HydrateMethodCountries(ctx context.Context, methods []Method) ([]Method, error) {
	for i := range methods {
		if methods[i].CountryMode != "specific" {
			continue
		}
		countries, err := s.repo.ListCountriesForMethod(ctx, methods[i].ID)
		if err != nil {
			return nil, err
		}
		methods[i].AllowedCountries = countries
	}
	return methods, nil
}

func (s *Service) ReplaceMethodCountries(ctx context.Context, methodID uuid.UUID, countries []string) error {
	if err := s.repo.ClearMethodCountries(ctx, methodID); err != nil {
		return err
	}
	for _, c := range countries {
		if c == "" {
			continue
		}
		if err := s.repo.AddCountryToMethod(ctx, methodID, c); err != nil {
			return err
		}
	}
	return nil
}
