package country

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// Whether a store takes a phone from any country or only from its own - "allowed" on a single-country store fixes
// every phone to that country's format
const CfgPhoneCountries = "general/country/phone_countries"

const (
	PhoneCountriesAny     = "any"
	PhoneCountriesAllowed = "allowed"
)

func init() {
	config.Register(config.Definition{
		Path: CfgPhoneCountries, Type: config.TypeSelect, Default: PhoneCountriesAny, Public: true,
		Label:       "Phone numbers from",
		Description: "Any country takes every valid number, typed with + and its country code. Allowed countries only takes numbers from the countries above - every active country when none are picked - and a store with one country takes only that country's numbers.",
		Options: []config.Option{
			{Value: PhoneCountriesAny, Label: "Any country"},
			{Value: PhoneCountriesAllowed, Label: "Allowed countries only"},
		},
	})
}

// PhoneSettings is one store's phone policy with the formats its forms show
type PhoneSettings struct {
	Rules   phone.Rules
	Locked  string                // the one country every phone must be from, "" when the shopper picks
	Formats []phone.CountryFormat // one per country the store offers, in its countries order
}

// PhoneRules is the policy every phone write on a store goes through
func (s *Service) PhoneRules(ctx context.Context, scope config.Scope) phone.Rules {
	r := phone.Rules{Default: s.DefaultCountry(ctx, scope)}
	if s.cfg == nil {
		return r
	}
	mode, err := s.cfg.GetString(ctx, CfgPhoneCountries, scope)
	if err != nil || mode != PhoneCountriesAllowed {
		return r
	}
	// No allow-list means the store sells to every active country, so those are the ones a phone may be from
	allowed := s.codeList(ctx, scope, CfgAllowedCountries)
	if len(allowed) == 0 {
		allowed = s.activeCodes(ctx)
	}
	if len(allowed) > 0 {
		r.Allowed = allowed
		if len(allowed) == 1 {
			r.Default = allowed[0]
		}
	}
	return r
}

// A failed read gives none, which leaves phones unrestricted rather than refusing every one
func (s *Service) activeCodes(ctx context.Context) []string {
	active, err := s.List(ctx, true)
	if err != nil {
		return nil
	}
	codes := make([]string, 0, len(active))
	for _, c := range active {
		codes = append(codes, c.Code)
	}
	return codes
}

// PhoneRulesForChannel is PhoneRules for callers holding a channel ID, as ValidateAddressForChannel is
func (s *Service) PhoneRulesForChannel(ctx context.Context, channelID uuid.UUID) phone.Rules {
	scope := config.DefaultScope
	if channelID != uuid.Nil {
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	}
	return s.PhoneRules(ctx, scope)
}

// PhoneSettings adds the formats of the countries a form can pick from - the allowed ones, else every active one
func (s *Service) PhoneSettings(ctx context.Context, scope config.Scope) (PhoneSettings, error) {
	out := PhoneSettings{Rules: s.PhoneRules(ctx, scope)}
	if len(out.Rules.Allowed) == 1 {
		out.Locked = out.Rules.Allowed[0]
	}
	codes := s.codeList(ctx, scope, CfgAllowedCountries)
	if len(codes) == 0 {
		codes = s.activeCodes(ctx)
	}
	for _, code := range codes {
		if f, ok := phone.FormatFor(code); ok {
			out.Formats = append(out.Formats, f)
		}
	}
	return out, nil
}
