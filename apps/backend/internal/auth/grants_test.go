package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The permission predicates and their implication rule - the same code now answers from a database read and from a token-carried set, so a change here moves both

func globalGrant(slug string) Grant { return Grant{Slug: slug} }

func channelGrant(slug string, ch uuid.UUID) Grant {
	return Grant{Slug: slug, ChannelID: &ch}
}

func localeGrant(slug, loc string) Grant {
	return Grant{Slug: slug, Locale: &loc}
}

func channelLocaleGrant(slug string, ch uuid.UUID, loc string) Grant {
	return Grant{Slug: slug, ChannelID: &ch, Locale: &loc}
}

func TestGrantSet_HasAnyAndHasGlobal(t *testing.T) {
	eu := uuid.New()
	snap := NewGrantSet([]Grant{channelGrant("orders.view", eu), globalGrant("catalog.view")})

	if !snap.HasAny("orders.view") {
		t.Error("a channel-scoped grant should satisfy hasAny")
	}
	if !snap.HasAny("catalog.view") {
		t.Error("a global grant should satisfy hasAny")
	}
	if snap.HasAny("orders.refund") {
		t.Error("ungranted slug reported as held")
	}

	if snap.HasGlobal("orders.view") {
		t.Error("a channel-scoped grant must not count as global")
	}
	if !snap.HasGlobal("catalog.view") {
		t.Error("a (NULL, NULL) grant is global")
	}
}

// A locale-only grant is not global - it is scoped on the locale axis
func TestGrantSet_LocaleOnlyGrantIsNotGlobal(t *testing.T) {
	snap := NewGrantSet([]Grant{localeGrant("cms.manage", "de")})
	if snap.HasGlobal("cms.manage") {
		t.Error("a locale-scoped grant must not count as global")
	}
	if !snap.HasAny("cms.manage") {
		t.Error("a locale-scoped grant is still held at some scope")
	}
}

func TestGrantSet_HasStore(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	de, fr := "de", "fr"

	t.Run("global grant matches every channel and locale", func(t *testing.T) {
		snap := NewGrantSet([]Grant{globalGrant("catalog.manage")})
		if !snap.HasStore("catalog.manage", eu, nil) || !snap.HasStore("catalog.manage", us, &fr) {
			t.Error("a global grant should match anything")
		}
	})

	t.Run("channel grant is pinned to its channel", func(t *testing.T) {
		snap := NewGrantSet([]Grant{channelGrant("catalog.manage", eu)})
		if !snap.HasStore("catalog.manage", eu, &de) {
			t.Error("channel grant with a NULL locale should match any locale on that channel")
		}
		if snap.HasStore("catalog.manage", us, nil) {
			t.Error("channel grant leaked to another channel")
		}
	})

	t.Run("locale-only grant spans channels", func(t *testing.T) {
		snap := NewGrantSet([]Grant{localeGrant("cms.manage", "de")})
		if !snap.HasStore("cms.manage", eu, &de) || !snap.HasStore("cms.manage", us, &de) {
			t.Error("a translator grant should work on every channel carrying that locale")
		}
		if snap.HasStore("cms.manage", eu, &fr) {
			t.Error("de grant leaked to fr")
		}
	})

	t.Run("tightest grant needs both axes to match", func(t *testing.T) {
		snap := NewGrantSet([]Grant{channelLocaleGrant("cms.manage", eu, "de")})
		if !snap.HasStore("cms.manage", eu, &de) {
			t.Error("exact match denied")
		}
		if snap.HasStore("cms.manage", us, &de) {
			t.Error("leaked to another channel")
		}
		if snap.HasStore("cms.manage", eu, &fr) {
			t.Error("leaked to another locale")
		}
	})

	// A check with no locale means not locale-specific, and a locale-scoped grant satisfies it - the documented widening, pinned so it can't drift
	t.Run("nil requested locale matches a locale-scoped grant", func(t *testing.T) {
		snap := NewGrantSet([]Grant{channelLocaleGrant("cms.manage", eu, "de")})
		if !snap.HasStore("cms.manage", eu, nil) {
			t.Error("a nil requested locale should match any granted locale on that channel")
		}
	})

	t.Run("stacked roles take the broadest grant", func(t *testing.T) {
		snap := NewGrantSet([]Grant{channelGrant("orders.view", eu), globalGrant("orders.view")})
		if !snap.HasStore("orders.view", us, &fr) {
			t.Error("the global grant from a second role should win")
		}
	})
}

func TestGrantSet_PermittedChannelIDs(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	snap := NewGrantSet([]Grant{
		channelGrant("orders.view", eu),
		channelLocaleGrant("orders.view", eu, "de"), // same channel twice
		channelGrant("orders.view", us),
		globalGrant("orders.view"), // NULL channel is not an id
	})

	ids := snap.PermittedChannelIDs("orders.view")
	if len(ids) != 2 {
		t.Fatalf("permittedChannelIDs = %v, want 2 distinct ids", ids)
	}
	got := map[uuid.UUID]bool{ids[0]: true, ids[1]: true}
	if !got[eu] || !got[us] {
		t.Errorf("permittedChannelIDs = %v, want %s and %s", ids, eu, us)
	}
	if snap.PermittedChannelIDs("orders.refund") != nil {
		t.Error("an ungranted slug should yield no channels")
	}
}

// manage is a strict superset of view, and the implication runs one way only
func TestEffectiveSlugs(t *testing.T) {
	tests := []struct {
		slug string
		want []string
	}{
		{"orders.view", []string{"orders.view", "orders.manage"}},
		{"catalog.manage", []string{"catalog.manage"}},
		{"orders.refund", []string{"orders.refund"}},
		{"config.manage_defaults", []string{"config.manage_defaults"}},
		{"developer.playground", []string{"developer.playground"}},
	}
	for _, tt := range tests {
		got := EffectiveSlugs(tt.slug)
		if len(got) != len(tt.want) {
			t.Errorf("EffectiveSlugs(%q) = %v, want %v", tt.slug, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("EffectiveSlugs(%q) = %v, want %v", tt.slug, got, tt.want)
				break
			}
		}
	}
}

// A set belongs to one admin, and answering for another would authorize them against grants that are not theirs
func TestGrantChecker_RefusesAnotherSubject(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	c := NewGrantChecker(mine, NewGrantSet([]Grant{globalGrant("orders.manage")}))
	ctx := context.Background()

	if ok, err := c.HasPermission(ctx, mine, "orders.view"); err != nil || !ok {
		t.Fatalf("HasPermission for the subject = (%v, %v), want true", ok, err)
	}
	for name, call := range map[string]func() (bool, error){
		"HasPermission":       func() (bool, error) { return c.HasPermission(ctx, theirs, "orders.view") },
		"HasGlobalPermission": func() (bool, error) { return c.HasGlobalPermission(ctx, theirs, "orders.view") },
		"HasChannelPermission": func() (bool, error) {
			return c.HasChannelPermission(ctx, theirs, "orders.view", uuid.New(), nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := call()
			if ok {
				t.Error("authorized an admin the set does not describe")
			}
			if !errors.Is(err, ErrGrantSubjectMismatch) {
				t.Errorf("err = %v, want ErrGrantSubjectMismatch", err)
			}
		})
	}
	if ids, err := c.ListPermittedChannelIDs(ctx, theirs, "orders.view"); ids != nil || !errors.Is(err, ErrGrantSubjectMismatch) {
		t.Errorf("ListPermittedChannelIDs = (%v, %v), want (nil, ErrGrantSubjectMismatch)", ids, err)
	}
}

// Grants round-trip through the flat list a token would carry, so serializing and rebuilding answers the same
func TestGrantSetSurvivesTheRoundTripATokenWouldMake(t *testing.T) {
	eu := uuid.New()
	original := NewGrantSet([]Grant{channelGrant("orders.view", eu), globalGrant("catalog.manage"), localeGrant("cms.manage", "de")})
	rebuilt := NewGrantSet(original.Grants())

	de := "de"
	for name, check := range map[string]func(*GrantSet) bool{
		"channel-scoped": func(g *GrantSet) bool { return g.HasStore("orders.view", eu, nil) },
		"global":         func(g *GrantSet) bool { return g.HasGlobal("catalog.manage") },
		"locale-scoped":  func(g *GrantSet) bool { return g.HasStore("cms.manage", uuid.New(), &de) },
		"ungranted":      func(g *GrantSet) bool { return g.HasAny("orders.refund") },
	} {
		t.Run(name, func(t *testing.T) {
			if check(original) != check(rebuilt) {
				t.Errorf("rebuilt set disagrees with the original on %s", name)
			}
		})
	}
}
