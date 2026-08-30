package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/auth"
)

// An admin's grants in memory, loaded once per request - the predicates below answer in memory rather than one SQL query per check, and effectiveSlugs expansion stays in the Service methods calling them
type grantSnapshot struct {
	bySlug map[string][]grantScope
}

type grantScope struct {
	channelID *uuid.UUID
	locale    *string
}

func newGrantSnapshot(grants []PermissionGrant) *grantSnapshot {
	bySlug := make(map[string][]grantScope, len(grants))
	for _, g := range grants {
		bySlug[g.Slug] = append(bySlug[g.Slug], grantScope{channelID: g.ChannelID, locale: g.Locale})
	}
	return &grantSnapshot{bySlug: bySlug}
}

func (g *grantSnapshot) hasAny(slug string) bool { // holds slug at ANY scope
	return len(g.bySlug[slug]) > 0
}

func (g *grantSnapshot) hasGlobal(slug string) bool { // a scope row for slug whose channel AND locale are both NULL
	for _, s := range g.bySlug[slug] {
		if s.channelID == nil && s.locale == nil {
			return true
		}
	}
	return false
}

// A scope row matches when its channel matches or is NULL, AND its locale matches or is NULL or the requested locale is NULL
func (g *grantSnapshot) hasStore(slug string, channelID uuid.UUID, locale *string) bool {
	for _, s := range g.bySlug[slug] {
		channelOK := s.channelID == nil || *s.channelID == channelID
		localeOK := s.locale == nil || locale == nil || *s.locale == *locale
		if channelOK && localeOK {
			return true
		}
	}
	return false
}

func (g *grantSnapshot) permittedChannelIDs(slug string) []uuid.UUID { // distinct non-NULL channel ids only - a NULL row means "any" and can't be enumerated
	var out []uuid.UUID
	seen := make(map[uuid.UUID]struct{})
	for _, s := range g.bySlug[slug] {
		if s.channelID == nil {
			continue
		}
		if _, ok := seen[*s.channelID]; ok {
			continue
		}
		seen[*s.channelID] = struct{}{}
		out = append(out, *s.channelID)
	}
	return out
}

// Loaded at most once per request through the auth request cache - with no cache on the context (background jobs, tests bypassing the middleware) it loads on every call, same result minus the memoization
func (s *Service) grantsFor(ctx context.Context, adminID uuid.UUID) (*grantSnapshot, error) {
	load := func() (any, error) {
		grants, err := s.repo.ListAllGrants(ctx, adminID)
		if err != nil {
			return nil, err
		}
		return newGrantSnapshot(grants), nil
	}
	if rc := auth.RequestCacheFrom(ctx); rc != nil {
		v, err := rc.GetOrLoad("admin-grants:"+adminID.String(), load)
		if err != nil {
			return nil, err
		}
		return v.(*grantSnapshot), nil
	}
	v, err := load()
	if err != nil {
		return nil, err
	}
	return v.(*grantSnapshot), nil
}
