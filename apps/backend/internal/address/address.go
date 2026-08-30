// Package address is the address-validation seam - a registry of pluggable validators, with the builtin format validator shipping by default and SmartyStreets or Loqate-style integrations registering alongside, picked per channel via config
// The storefront calls validateAddress before checkout submit for a soft correctness check and normalization suggestions, and strict mode lets a store block placement on invalid
package address

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/config"
)

func init() {
	config.Register(config.Definition{Path: "sales/checkout/address_validator", Type: config.TypeSelect, Default: "basic", Label: "Address validator", Description: "Which validator checks checkout addresses. Options come from what is registered, so an uninstalled provider stops being offered. An unrecognised value is treated as no checking rather than failing checkout.", Public: true, Source: "address_validators"})
	config.Register(config.Definition{Path: "sales/checkout/address_validation_strict", Type: config.TypeBool, Default: "false", Label: "Reject checkout addresses the validator marks invalid", Public: true})
}

type Input struct {
	Street1     string
	Street2     string
	City        string
	State       string
	PostalCode  string
	CountryCode string
}

type Result struct {
	Valid      bool
	Messages   []string
	Normalized *Input // nil when nothing changed
}

type Validator interface { // channelID scopes the provider config
	Validate(ctx context.Context, channelID uuid.UUID, in Input) (Result, error)
}

var (
	mu         sync.RWMutex
	validators = map[string]Validator{}
)

func Register(slug string, v Validator) { // a later registration with the same slug overrides, so plugin reload works
	mu.Lock()
	defer mu.Unlock()
	validators[slug] = v
}

type Service struct {
	cfg *config.Service
}

func NewService(cfg *config.Service) *Service { return &Service{cfg: cfg} }

// Runs the channel's configured validator - "none", or an unknown slug, reports valid, since validation must never break checkout because a provider was unplugged
func (s *Service) Validate(ctx context.Context, channelID uuid.UUID, in Input) (Result, error) {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	slug, err := s.cfg.GetString(ctx, "sales/checkout/address_validator", scope)
	if err != nil {
		return Result{}, fmt.Errorf("address validator config: %w", err)
	}
	slug = strings.TrimSpace(slug)
	if slug == "" || slug == "none" {
		return Result{Valid: true}, nil
	}
	mu.RLock()
	v, ok := validators[slug]
	mu.RUnlock()
	if !ok {
		return Result{Valid: true}, nil
	}
	return v.Validate(ctx, channelID, in)
}

func (s *Service) Strict(ctx context.Context, channelID uuid.UUID) bool { // whether invalid addresses block placement on this channel
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	strict, err := s.cfg.GetBool(ctx, "sales/checkout/address_validation_strict", scope)
	return err == nil && strict
}

// Covers the formats worth catching typos in - countries absent from the map only get a non-empty check
var postalPatterns = map[string]*regexp.Regexp{
	"US": regexp.MustCompile(`^\d{5}(-\d{4})?$`),
	"CA": regexp.MustCompile(`^[A-Za-z]\d[A-Za-z] ?\d[A-Za-z]\d$`),
	"GB": regexp.MustCompile(`^[A-Za-z]{1,2}\d[A-Za-z\d]? ?\d[A-Za-z]{2}$`),
	"DE": regexp.MustCompile(`^\d{5}$`),
	"FR": regexp.MustCompile(`^\d{5}$`),
	"NL": regexp.MustCompile(`^\d{4} ?[A-Za-z]{2}$`),
	"AU": regexp.MustCompile(`^\d{4}$`),
	"RS": regexp.MustCompile(`^\d{5}$`),
}

var zip9 = regexp.MustCompile(`^\d{9}$`)

type Basic struct{}

func (Basic) Validate(_ context.Context, _ uuid.UUID, in Input) (Result, error) {
	res := Result{Valid: true}
	norm := Input{
		Street1:     strings.TrimSpace(in.Street1),
		Street2:     strings.TrimSpace(in.Street2),
		City:        strings.TrimSpace(in.City),
		State:       strings.TrimSpace(in.State),
		PostalCode:  strings.ToUpper(strings.TrimSpace(in.PostalCode)),
		CountryCode: strings.ToUpper(strings.TrimSpace(in.CountryCode)),
	}
	if norm.Street1 == "" {
		res.Valid = false
		res.Messages = append(res.Messages, "street address is required")
	}
	if norm.City == "" {
		res.Valid = false
		res.Messages = append(res.Messages, "city is required")
	}
	if len(norm.CountryCode) != 2 {
		res.Valid = false
		res.Messages = append(res.Messages, "country must be a 2-letter code")
	}
	// A US ZIP+4 with a missing dash is the classic normalization case - fix it before the format check so it validates clean
	if norm.CountryCode == "US" && zip9.MatchString(norm.PostalCode) {
		norm.PostalCode = norm.PostalCode[:5] + "-" + norm.PostalCode[5:]
	}
	if norm.PostalCode == "" {
		res.Valid = false
		res.Messages = append(res.Messages, "postal code is required")
	} else if pat, ok := postalPatterns[norm.CountryCode]; ok && !pat.MatchString(norm.PostalCode) {
		res.Valid = false
		res.Messages = append(res.Messages, fmt.Sprintf("postal code %q doesn't match the %s format", norm.PostalCode, norm.CountryCode))
	}
	if norm != in {
		res.Normalized = &norm
	}
	return res, nil
}

func init() { Register("basic", Basic{}) }
