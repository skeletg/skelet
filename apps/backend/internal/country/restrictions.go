package country

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/config"
)

// Enforces the channel's country and region options on one address - the country must exist, be active and, when an allow-list is configured, be in it, while the region must be present when required and valid when the country has a canonical subdivision list
// Region matching is case-insensitive, and the defaults are permissive: an empty allow-list means all active, no required-region setting means region optional, and a country with no seeded regions leaves region free text
func (s *Service) ValidateAddress(ctx context.Context, scope config.Scope, countryCode string, region *string) error {
	code := normalizeCode(countryCode)
	if err := validateCode(code); err != nil {
		return err
	}
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return err // ErrCountryNotFound, etc.
	}
	if !c.IsActive {
		return fmt.Errorf("%w: %s", ErrCountryNotAllowed, code)
	}
	if allowed := s.AllowedCountries(ctx, scope); allowed != nil && !allowed[code] {
		return fmt.Errorf("%w: %s", ErrCountryNotAllowed, code)
	}

	regionVal := ""
	if region != nil {
		regionVal = strings.TrimSpace(*region)
	}
	if s.RequiredRegionCountries(ctx, scope)[code] && regionVal == "" {
		return fmt.Errorf("%w: %s", ErrRegionRequired, code)
	}
	if regionVal != "" {
		regions, err := s.repo.ListRegions(ctx, code)
		if err != nil {
			return err
		}
		if len(regions) > 0 && !regionMatches(regions, regionVal) {
			return fmt.Errorf("%w: %q for %s", ErrInvalidRegion, regionVal, code)
		}
	}
	return nil
}

// The channel-scoped convenience for callers holding a channel ID, like order placement and customer address save, rather than a config.Scope
func (s *Service) ValidateAddressForChannel(ctx context.Context, channelID uuid.UUID, countryCode string, region *string) error {
	scope := config.DefaultScope
	if channelID != uuid.Nil {
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	}
	return s.ValidateAddress(ctx, scope, countryCode, region)
}

func regionMatches(regions []Region, val string) bool {
	for _, r := range regions {
		if strings.EqualFold(r.Code, val) {
			return true
		}
	}
	return false
}
