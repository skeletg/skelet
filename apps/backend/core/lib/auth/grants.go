package auth

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The permission model itself, kept out of the admin service so the same predicates answer from a database read or from a grant set carried in a token

// A single role assignment: the permission, and the scope it was granted at - nil channel or locale means any
type Grant struct {
	Slug      string
	ChannelID *uuid.UUID
	Locale    *string
}

// An admin's grants in memory - the predicates answer without a query per check
type GrantSet struct {
	bySlug map[string][]Grant
}

func NewGrantSet(grants []Grant) *GrantSet {
	bySlug := make(map[string][]Grant, len(grants))
	for _, g := range grants {
		bySlug[g.Slug] = append(bySlug[g.Slug], g)
	}
	return &GrantSet{bySlug: bySlug}
}

// Grants returns the flat list the set was built from, for callers that have to serialize it
func (g *GrantSet) Grants() []Grant {
	out := make([]Grant, 0, len(g.bySlug))
	for _, gs := range g.bySlug {
		out = append(out, gs...)
	}
	return out
}

// EffectiveSlugs expands a slug to everything that satisfies it, so managing something implies viewing it
func EffectiveSlugs(slug string) []string {
	if before, ok := strings.CutSuffix(slug, ".view"); ok {
		return []string{slug, before + ".manage"}
	}
	return []string{slug}
}

func (g *GrantSet) HasAny(slug string) bool { // holds slug at ANY scope
	return len(g.bySlug[slug]) > 0
}

func (g *GrantSet) HasGlobal(slug string) bool { // a scope row for slug whose channel AND locale are both nil
	for _, s := range g.bySlug[slug] {
		if s.ChannelID == nil && s.Locale == nil {
			return true
		}
	}
	return false
}

// A scope row matches when its channel matches or is nil, AND its locale matches or is nil or the requested locale is nil
func (g *GrantSet) HasStore(slug string, channelID uuid.UUID, locale *string) bool {
	for _, s := range g.bySlug[slug] {
		channelOK := s.ChannelID == nil || *s.ChannelID == channelID
		localeOK := s.Locale == nil || locale == nil || *s.Locale == *locale
		if channelOK && localeOK {
			return true
		}
	}
	return false
}

// Covers reports whether the set holds want at least as widely
func (g *GrantSet) Covers(want Grant) bool {
	for _, slug := range EffectiveSlugs(want.Slug) {
		for _, s := range g.bySlug[slug] {
			channelOK := s.ChannelID == nil || (want.ChannelID != nil && *s.ChannelID == *want.ChannelID)
			localeOK := s.Locale == nil || (want.Locale != nil && *s.Locale == *want.Locale)
			if channelOK && localeOK {
				return true
			}
		}
	}
	return false
}

func (g *GrantSet) PermittedChannelIDs(slug string) []uuid.UUID { // distinct non-nil channel ids only - a nil row means any and can't be enumerated
	var out []uuid.UUID
	seen := make(map[uuid.UUID]struct{})
	for _, s := range g.bySlug[slug] {
		if s.ChannelID == nil {
			continue
		}
		if _, ok := seen[*s.ChannelID]; ok {
			continue
		}
		seen[*s.ChannelID] = struct{}{}
		out = append(out, *s.ChannelID)
	}
	return out
}

// A set belongs to one admin, so answering for another would authorize them against the wrong grants
var ErrGrantSubjectMismatch = errors.New("grant set belongs to a different admin")

func init() {
	apperr.RegisterSentinel(ErrGrantSubjectMismatch, "GRANT_SUBJECT_MISMATCH", apperr.ClassForbidden)
}

// GrantChecker answers the permission interfaces from a carried grant set, for a service holding the caller's grants but not the commerce database
type GrantChecker struct {
	adminID uuid.UUID
	set     *GrantSet
}

func NewGrantChecker(adminID uuid.UUID, set *GrantSet) *GrantChecker {
	return &GrantChecker{adminID: adminID, set: set}
}

func (c *GrantChecker) HasPermission(_ context.Context, adminID uuid.UUID, slug string) (bool, error) {
	if err := c.subject(adminID); err != nil {
		return false, err
	}
	return slices.ContainsFunc(EffectiveSlugs(slug), c.set.HasAny), nil
}

func (c *GrantChecker) HasGlobalPermission(_ context.Context, adminID uuid.UUID, slug string) (bool, error) {
	if err := c.subject(adminID); err != nil {
		return false, err
	}
	return slices.ContainsFunc(EffectiveSlugs(slug), c.set.HasGlobal), nil
}

func (c *GrantChecker) HasChannelPermission(_ context.Context, adminID uuid.UUID, slug string, channelID uuid.UUID, locale *string) (bool, error) {
	if err := c.subject(adminID); err != nil {
		return false, err
	}
	for _, candidate := range EffectiveSlugs(slug) {
		if c.set.HasStore(candidate, channelID, locale) {
			return true, nil
		}
	}
	return false, nil
}

func (c *GrantChecker) ListPermittedChannelIDs(_ context.Context, adminID uuid.UUID, slug string) ([]uuid.UUID, error) {
	if err := c.subject(adminID); err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for _, candidate := range EffectiveSlugs(slug) {
		out = append(out, c.set.PermittedChannelIDs(candidate)...)
	}
	return out, nil
}

const systemCallerKey contextKey = "system_caller"

// WithSystemCaller marks the request as core's own process - it authorizes nothing on its own, and a
// handler that accepts it says so explicitly
func WithSystemCaller(ctx context.Context) context.Context {
	return context.WithValue(ctx, systemCallerKey, true)
}

func IsSystemCaller(ctx context.Context) bool {
	v, _ := ctx.Value(systemCallerKey).(bool)
	return v
}

const customerCallerKey contextKey = "customer_caller"

// WithCustomerCaller marks the request as core asking about one shopper, which is the authority a
// storefront path has when there is no admin behind it
func WithCustomerCaller(ctx context.Context, customerID uuid.UUID) context.Context {
	return context.WithValue(ctx, customerCallerKey, customerID)
}

// CustomerCallerFrom is the shopper the caller may read about, uuid.Nil when it is not that kind of caller
func CustomerCallerFrom(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(customerCallerKey).(uuid.UUID)
	return v
}

const grantCheckerKey contextKey = "grant_checker"

// WithGrantChecker carries the caller's checker on the request, for a service that authorizes from a token rather than a database
func WithGrantChecker(ctx context.Context, c *GrantChecker) context.Context {
	return context.WithValue(ctx, grantCheckerKey, c)
}

// GrantCheckerFrom returns the request's checker, or one that denies everything when the request carried no grants
// The return type also covers ChannelScopeProvider, so a token-authorized resolver can narrow a listing to the caller's channels
func GrantCheckerFrom(ctx context.Context) interface {
	ScopeAwareChecker
	ChannelScopeProvider
} {
	if c, ok := ctx.Value(grantCheckerKey).(*GrantChecker); ok && c != nil {
		return c
	}
	return denyAll{}
}

// Never reached through the guards, which refuse an unauthenticated request first - it exists so a wiring mistake denies instead of panicking
type denyAll struct{}

func (denyAll) HasPermission(context.Context, uuid.UUID, string) (bool, error) { return false, nil }
func (denyAll) HasGlobalPermission(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (denyAll) HasChannelPermission(context.Context, uuid.UUID, string, uuid.UUID, *string) (bool, error) {
	return false, nil
}
func (denyAll) ListPermittedChannelIDs(context.Context, uuid.UUID, string) ([]uuid.UUID, error) {
	return nil, nil
}

func (c *GrantChecker) subject(adminID uuid.UUID) error {
	if adminID != c.adminID {
		return ErrGrantSubjectMismatch
	}
	return nil
}
