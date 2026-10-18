package shipping

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type Service struct {
	repo Repository
	// remoteRates adds remote providers to QuoteAll/PriceSelected, nil = registry only (remote.go)
	remoteRates RemoteRateSource
	// remoteTracking resolves tracking via a universal tracker before the carrier's own (remote_tracking.go)
	remoteTracking RemoteTrackingSource
	dims           DimensionLookup
	tx             *txrunner.Manager
}

func (s *Service) SetDimensionLookup(l DimensionLookup) { s.dims = l }

// SetTx lets SaveMethod write a method, its rates and its countries as one - without it each write commits on its own
func (s *Service) SetTx(tx *txrunner.Manager) { s.tx = tx }

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Repo() Repository { return s.repo }

// Every active method whose zone covers the country, priced against the subtotal, in sort order - empty means no zone covers it, and the storefront shows "delivery not available"
func (s *Service) Quote(ctx context.Context, in QuoteInput) ([]Quote, error) {
	in.CountryCode = normCountry(in.CountryCode)
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
		if !methodAllowsCountry(m, in.CountryCode) {
			continue
		}
		out = append(out, Quote{
			MethodID:           m.ID,
			Code:               m.Code,
			Name:               m.Name,
			PriceCents:         ComputePrice(m, in.SubtotalCents, in.Items),
			DeliveryKind:       normalizeDeliveryKind(m.DeliveryKind),
			EstimatedDays:      m.EstimatedDays,
			DeliversOnWeekends: m.DeliversOnWeekends,
		})
	}
	return out, nil
}

// The lowest free_above_cents among active, country-allowed 'free_above' methods, powering the "add $X for free shipping" nudge - 0 means none applies, and an empty country scans every zone for a channel-wide estimate
func (s *Service) FreeShippingThreshold(ctx context.Context, channelID uuid.UUID, country string) (int64, error) {
	if channelID == uuid.Nil {
		return 0, nil
	}
	country = normCountry(country)

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
			if country != "" && !methodAllowsCountry(m, country) {
				continue
			}
			if threshold == 0 || m.FreeAboveCents < threshold {
				threshold = m.FreeAboveCents
			}
		}
	}
	return threshold, nil
}

// Always true for mode "all", else only when the country is in the method's hydrated allow-list
func methodAllowsCountry(m Method, country string) bool {
	return m.CountryMode != "specific" || containsFold(m.AllowedCountries, country)
}

var errCountryInvalid = apperr.New("SHIPPING_COUNTRY_INVALID", apperr.ClassValidation, "countries are two-letter ISO codes")

// normCountry is the stored and looked-up form of a country code - trimmed and upper-case
func normCountry(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// normCountries cleans an admin's country list, dropping blanks and duplicates and refusing anything that isn't two letters
func normCountries(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = normCountry(c)
		if c == "" || slices.Contains(out, c) {
			continue
		}
		if len(c) != 2 || c[0] < 'A' || c[0] > 'Z' || c[1] < 'A' || c[1] > 'Z' {
			return nil, errCountryInvalid
		}
		out = append(out, c)
	}
	return out, nil
}

// The authoritative price at placement - verifies methodCode still exists and is active for the zone, then prices it with the live formula rather than the client's cents - ErrMethodNotFound means "option no longer available, retry"
func (s *Service) ComputeForOrder(ctx context.Context, methodCode string, in QuoteInput) (int64, Method, error) {
	in.CountryCode = normCountry(in.CountryCode)
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
	if !methodAllowsCountry(method, in.CountryCode) {
		return 0, Method{}, ErrMethodNotFound
	}
	return ComputePrice(method, in.SubtotalCents, in.Items), method, nil
}

// The ID-based sibling of ComputeForOrder - verifies the method belongs to the country's zone, so one from another zone can't be smuggled in, and is still active
func (s *Service) ComputeForOrderByID(ctx context.Context, methodID uuid.UUID, in QuoteInput) (int64, Method, error) {
	in.CountryCode = normCountry(in.CountryCode)
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
	if !methodAllowsCountry(method, in.CountryCode) {
		return 0, Method{}, ErrMethodNotFound
	}
	return ComputePrice(method, in.SubtotalCents, in.Items), method, nil
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

// SaveZone creates a zone (no ID) or updates one, with its countries, in one transaction - a failed country write never leaves a zone live with none. Nil countries keep what the zone has
func (s *Service) SaveZone(ctx context.Context, z Zone, countries []string) (Zone, error) {
	if countries != nil {
		var err error
		if countries, err = normCountries(countries); err != nil {
			return Zone{}, err
		}
	}
	var out Zone
	err := s.inTx(ctx, func(r Repository) error {
		var err error
		if z.ID == uuid.Nil {
			out, err = r.CreateZone(ctx, z)
		} else {
			out, err = r.UpdateZone(ctx, z)
		}
		if err != nil || countries == nil {
			return err
		}
		if err := r.ClearZoneCountries(ctx, out.ID); err != nil {
			return err
		}
		for _, c := range countries {
			if err := r.AddCountryToZone(ctx, out.ID, c); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// inTx runs fn on a transactional repository, or on the plain one when no tx manager is wired
func (s *Service) inTx(ctx context.Context, fn func(Repository) error) error {
	if s.tx == nil {
		return fn(s.repo)
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error { return fn(s.repo.WithTx(t)) })
}

// The kinds a built-in method can price - store pickup is the pickup carrier's, never a zone method
var methodKinds = []string{DefaultKind, "parcel_locker", "carrier_point"}

var (
	errMethodKind      = apperr.New("SHIPPING_METHOD_KIND_INVALID", apperr.ClassValidation, "a shipping method prices door delivery, a pickup point, or delivery to the nearest pickup point")
	errMethodUnits     = apperr.New("SHIPPING_METHOD_UNITS_INVALID", apperr.ClassValidation, "units per charge can't be negative")
	errBuiltinWeekends = apperr.New("SHIPPING_CARRIER_WEEKENDS_PER_METHOD", apperr.ClassValidation, "the built-in carrier's methods each set their own weekend delivery")
	errMethodDays      = apperr.New("SHIPPING_METHOD_DAYS_INVALID", apperr.ClassValidation, "delivery days can't be negative")
	errMethodPrice     = apperr.New("SHIPPING_METHOD_PRICE_INVALID", apperr.ClassValidation, "a method's price and free-shipping threshold can't be negative")
	errTypeRate        = apperr.New("SHIPPING_TYPE_RATE_INVALID", apperr.ClassValidation, "each product type is priced once, at zero or more")
	errTypeRateUnknown = apperr.New("SHIPPING_TYPE_RATE_UNKNOWN_TYPE", apperr.ClassValidation, "a type rate names a product type that doesn't exist")
)

// CheckMethodPricing refuses a kind or a bundle size the engine can't price
func CheckMethodPricing(m Method) error {
	if m.DeliveryKind != "" && !slices.Contains(methodKinds, m.DeliveryKind) {
		return errMethodKind
	}
	if m.UnitsPerCharge < 0 {
		return errMethodUnits
	}
	if m.EstimatedDays < 0 {
		return errMethodDays
	}
	if m.FlatCents < 0 || m.FreeAboveCents < 0 {
		return errMethodPrice
	}
	return nil
}

// ReplaceMethodTypeRates sets a method's per-product-type prices - each type at most once
func (s *Service) ReplaceMethodTypeRates(ctx context.Context, methodID uuid.UUID, rates []TypeRate) error {
	if err := checkTypeRates(rates); err != nil {
		return err
	}
	return s.repo.ReplaceTypeRates(ctx, methodID, rates)
}

func checkTypeRates(rates []TypeRate) error {
	seen := map[uuid.UUID]bool{}
	for _, r := range rates {
		if r.ProductTypeID == uuid.Nil || seen[r.ProductTypeID] || r.PriceCents < 0 || r.UnitsPerCharge < 0 {
			return errTypeRate
		}
		seen[r.ProductTypeID] = true
	}
	return nil
}

// SaveMethod creates a method (no ID) or updates one with its per-type rates and its countries in one transaction, so a bad rate never leaves a half-saved method live at checkout. Nil rates or countries keep what the method has
func (s *Service) SaveMethod(ctx context.Context, m Method, rates []TypeRate, countries []string) (Method, error) {
	if err := CheckMethodPricing(m); err != nil {
		return Method{}, err
	}
	if err := checkTypeRates(rates); err != nil {
		return Method{}, err
	}
	if countries != nil {
		var err error
		if countries, err = normCountries(countries); err != nil {
			return Method{}, err
		}
	}
	var out Method
	save := func(r Repository) error {
		var err error
		if m.ID == uuid.Nil {
			out, err = r.CreateMethod(ctx, m)
		} else {
			out, err = r.UpdateMethod(ctx, m)
		}
		if err != nil {
			return err
		}
		if rates != nil {
			if err := r.ReplaceTypeRates(ctx, out.ID, rates); err != nil {
				if pgdb.IsForeignKeyViolation(err) {
					return errTypeRateUnknown
				}
				return err
			}
		}
		if countries != nil {
			if err := r.ClearMethodCountries(ctx, out.ID); err != nil {
				return err
			}
			for _, c := range countries {
				if err := r.AddCountryToMethod(ctx, out.ID, c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := s.inTx(ctx, save); err != nil {
		return Method{}, err
	}
	return s.repo.GetMethod(ctx, out.ID)
}
