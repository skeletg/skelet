package admin

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/auth"
)

// Only ListAllGrants is exercised here - the embedded interface keeps the other ~30 Repository methods out of the way
type stubRepo struct {
	Repository
	grants []PermissionGrant
	err    error
	calls  int
}

func (s *stubRepo) ListAllGrants(_ context.Context, _ uuid.UUID) ([]PermissionGrant, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.grants, nil
}

func global(slug string) PermissionGrant { return PermissionGrant{Slug: slug} }

func onChannel(slug string, ch uuid.UUID) PermissionGrant {
	return PermissionGrant{Slug: slug, ChannelID: &ch}
}

func onLocale(slug, loc string) PermissionGrant {
	return PermissionGrant{Slug: slug, Locale: &loc}
}

func onChannelLocale(slug string, ch uuid.UUID, loc string) PermissionGrant {
	return PermissionGrant{Slug: slug, ChannelID: &ch, Locale: &loc}
}

func TestGrantSnapshot_HasAnyAndHasGlobal(t *testing.T) {
	eu := uuid.New()
	snap := newGrantSnapshot([]PermissionGrant{onChannel("orders.view", eu), global("catalog.view")})

	if !snap.hasAny("orders.view") {
		t.Error("a channel-scoped grant should satisfy hasAny")
	}
	if !snap.hasAny("catalog.view") {
		t.Error("a global grant should satisfy hasAny")
	}
	if snap.hasAny("orders.refund") {
		t.Error("ungranted slug reported as held")
	}

	if snap.hasGlobal("orders.view") {
		t.Error("a channel-scoped grant must not count as global")
	}
	if !snap.hasGlobal("catalog.view") {
		t.Error("a (NULL, NULL) grant is global")
	}
}

// A locale-only grant is not global - it is scoped on the locale axis
func TestGrantSnapshot_LocaleOnlyGrantIsNotGlobal(t *testing.T) {
	snap := newGrantSnapshot([]PermissionGrant{onLocale("cms.manage", "de")})
	if snap.hasGlobal("cms.manage") {
		t.Error("a locale-scoped grant must not count as global")
	}
	if !snap.hasAny("cms.manage") {
		t.Error("a locale-scoped grant is still held at some scope")
	}
}

func TestGrantSnapshot_HasStore(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	de, fr := "de", "fr"

	t.Run("global grant matches every channel and locale", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{global("catalog.manage")})
		if !snap.hasStore("catalog.manage", eu, nil) || !snap.hasStore("catalog.manage", us, &fr) {
			t.Error("a global grant should match anything")
		}
	})

	t.Run("channel grant is pinned to its channel", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{onChannel("catalog.manage", eu)})
		if !snap.hasStore("catalog.manage", eu, &de) {
			t.Error("channel grant with a NULL locale should match any locale on that channel")
		}
		if snap.hasStore("catalog.manage", us, nil) {
			t.Error("channel grant leaked to another channel")
		}
	})

	t.Run("locale-only grant spans channels", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{onLocale("cms.manage", "de")})
		if !snap.hasStore("cms.manage", eu, &de) || !snap.hasStore("cms.manage", us, &de) {
			t.Error("a translator grant should work on every channel carrying that locale")
		}
		if snap.hasStore("cms.manage", eu, &fr) {
			t.Error("de grant leaked to fr")
		}
	})

	t.Run("tightest grant needs both axes to match", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{onChannelLocale("cms.manage", eu, "de")})
		if !snap.hasStore("cms.manage", eu, &de) {
			t.Error("exact match denied")
		}
		if snap.hasStore("cms.manage", us, &de) {
			t.Error("leaked to another channel")
		}
		if snap.hasStore("cms.manage", eu, &fr) {
			t.Error("leaked to another locale")
		}
	})

	// A check with no locale means "not locale-specific", and a locale-scoped grant satisfies it -
	// this is the documented widening, pinned so it can't change by accident
	t.Run("nil requested locale matches a locale-scoped grant", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{onChannelLocale("cms.manage", eu, "de")})
		if !snap.hasStore("cms.manage", eu, nil) {
			t.Error("a nil requested locale should match any granted locale on that channel")
		}
	})

	t.Run("stacked roles take the broadest grant", func(t *testing.T) {
		snap := newGrantSnapshot([]PermissionGrant{onChannel("orders.view", eu), global("orders.view")})
		if !snap.hasStore("orders.view", us, &fr) {
			t.Error("the global grant from a second role should win")
		}
	})
}

func TestGrantSnapshot_PermittedChannelIDs(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	snap := newGrantSnapshot([]PermissionGrant{
		onChannel("orders.view", eu),
		onChannelLocale("orders.view", eu, "de"), // same channel twice
		onChannel("orders.view", us),
		global("orders.view"), // NULL channel is not an id
	})

	ids := snap.permittedChannelIDs("orders.view")
	if len(ids) != 2 {
		t.Fatalf("permittedChannelIDs = %v, want 2 distinct ids", ids)
	}
	got := map[uuid.UUID]bool{ids[0]: true, ids[1]: true}
	if !got[eu] || !got[us] {
		t.Errorf("permittedChannelIDs = %v, want %s and %s", ids, eu, us)
	}
	if snap.permittedChannelIDs("orders.refund") != nil {
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
		got := effectiveSlugs(tt.slug)
		if len(got) != len(tt.want) {
			t.Errorf("effectiveSlugs(%q) = %v, want %v", tt.slug, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("effectiveSlugs(%q) = %v, want %v", tt.slug, got, tt.want)
				break
			}
		}
	}
}

func TestService_ManageImpliesView(t *testing.T) {
	eu := uuid.New()
	svc := NewService(&stubRepo{grants: []PermissionGrant{onChannel("orders.manage", eu)}})
	ctx, adminID := context.Background(), uuid.New()

	if ok, err := svc.HasPermission(ctx, adminID, "orders.view"); err != nil || !ok {
		t.Errorf("HasPermission(orders.view) = (%v, %v), want true - manage implies view", ok, err)
	}
	if ok, err := svc.HasChannelPermission(ctx, adminID, "orders.view", eu, nil); err != nil || !ok {
		t.Errorf("HasChannelPermission(orders.view) = (%v, %v), want true", ok, err)
	}
	if ids, err := svc.ListPermittedChannelIDs(ctx, adminID, "orders.view"); err != nil || len(ids) != 1 || ids[0] != eu {
		t.Errorf("ListPermittedChannelIDs(orders.view) = (%v, %v), want [%s]", ids, err, eu)
	}
}

func TestService_ViewDoesNotImplyManage(t *testing.T) {
	eu := uuid.New()
	svc := NewService(&stubRepo{grants: []PermissionGrant{global("orders.view")}})
	ctx, adminID := context.Background(), uuid.New()

	if ok, _ := svc.HasPermission(ctx, adminID, "orders.manage"); ok {
		t.Error("a view grant must not satisfy a manage check")
	}
	if ok, _ := svc.HasGlobalPermission(ctx, adminID, "orders.manage"); ok {
		t.Error("a view grant must not satisfy a global manage check")
	}
	if ok, _ := svc.HasChannelPermission(ctx, adminID, "orders.manage", eu, nil); ok {
		t.Error("a view grant must not satisfy a store manage check")
	}
}

func TestService_ListPermittedChannelIDs_UnionsViewAndManageWithoutDuplicates(t *testing.T) {
	eu, us, apac := uuid.New(), uuid.New(), uuid.New()
	svc := NewService(&stubRepo{grants: []PermissionGrant{
		onChannel("orders.view", eu),
		onChannel("orders.view", us),
		onChannel("orders.manage", us), // overlaps view
		onChannel("orders.manage", apac),
	}})

	ids, err := svc.ListPermittedChannelIDs(context.Background(), uuid.New(), "orders.view")
	if err != nil {
		t.Fatalf("ListPermittedChannelIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("got %v, want 3 distinct channels", ids)
	}
	want := []string{apac.String(), eu.String(), us.String()}
	sort.Strings(want)
	gotStrs := []string{ids[0].String(), ids[1].String(), ids[2].String()}
	sort.Strings(gotStrs)
	for i := range want {
		if gotStrs[i] != want[i] {
			t.Errorf("channels = %v, want %v", gotStrs, want)
			break
		}
	}
}

func TestService_GrantsLoadOncePerRequest(t *testing.T) {
	eu := uuid.New()
	repo := &stubRepo{grants: []PermissionGrant{onChannel("orders.view", eu)}}
	svc := NewService(repo)
	adminID := uuid.New()

	ctx := auth.WithRequestCache(context.Background())
	for range 5 {
		if _, err := svc.HasPermission(ctx, adminID, "orders.view"); err != nil {
			t.Fatalf("HasPermission: %v", err)
		}
		if _, err := svc.HasChannelPermission(ctx, adminID, "orders.view", eu, nil); err != nil {
			t.Fatalf("HasChannelPermission: %v", err)
		}
	}
	if repo.calls != 1 {
		t.Errorf("repo hit %d times behind a request cache, want 1 - this is the per-row N+1 guard", repo.calls)
	}

	// Without the middleware's cache every check loads - same answers, no memoization
	repo.calls = 0
	if _, err := svc.HasPermission(context.Background(), adminID, "orders.view"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HasPermission(context.Background(), adminID, "orders.view"); err != nil {
		t.Fatal(err)
	}
	if repo.calls != 2 {
		t.Errorf("repo hit %d times with no request cache, want 2", repo.calls)
	}
}

func TestService_GrantLoadErrorDeniesRatherThanGrants(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&stubRepo{err: boom})
	ctx, adminID := context.Background(), uuid.New()

	ok, err := svc.HasPermission(ctx, adminID, "orders.view")
	if ok {
		t.Error("a failed grant load must not report the permission as held")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the repo error", err)
	}
	if ids, err := svc.ListPermittedChannelIDs(ctx, adminID, "orders.view"); ids != nil || !errors.Is(err, boom) {
		t.Errorf("ListPermittedChannelIDs = (%v, %v), want (nil, the repo error)", ids, err)
	}
}
