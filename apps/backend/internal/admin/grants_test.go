package admin

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
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

// Catches the two paths drifting apart, which would be a permission an admin holds on one service and not the other - the shared predicates are covered in auth
func TestTokenBackedCheckerAgreesWithTheDatabaseOne(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	de, fr := "de", "fr"
	grants := []PermissionGrant{
		onChannel("orders.view", eu),
		onChannelLocale("cms.manage", eu, "de"),
		onLocale("emails.manage", "fr"),
		global("catalog.manage"),
		onChannel("lens.view", us),
	}
	adminID := uuid.New()
	fromDB := NewService(&stubRepo{grants: grants})
	fromToken := auth.NewGrantChecker(adminID, auth.NewGrantSet(grants))
	ctx := context.Background()

	slugs := []string{"orders.view", "orders.manage", "cms.manage", "cms.view", "emails.manage", "emails.view", "catalog.view", "catalog.manage", "lens.view", "orders.refund"}
	channels := []uuid.UUID{eu, us, uuid.New()}
	locales := []*string{nil, &de, &fr}

	for _, slug := range slugs {
		wantAny, err := fromDB.HasPermission(ctx, adminID, slug)
		if err != nil {
			t.Fatalf("HasPermission: %v", err)
		}
		gotAny, err := fromToken.HasPermission(ctx, adminID, slug)
		if err != nil {
			t.Fatalf("token HasPermission: %v", err)
		}
		if gotAny != wantAny {
			t.Errorf("HasPermission(%q): token %v, database %v", slug, gotAny, wantAny)
		}

		wantGlobal, _ := fromDB.HasGlobalPermission(ctx, adminID, slug)
		gotGlobal, _ := fromToken.HasGlobalPermission(ctx, adminID, slug)
		if gotGlobal != wantGlobal {
			t.Errorf("HasGlobalPermission(%q): token %v, database %v", slug, gotGlobal, wantGlobal)
		}

		wantIDs, _ := fromDB.ListPermittedChannelIDs(ctx, adminID, slug)
		gotIDs, _ := fromToken.ListPermittedChannelIDs(ctx, adminID, slug)
		if len(gotIDs) != len(wantIDs) {
			t.Errorf("ListPermittedChannelIDs(%q): token %v, database %v", slug, gotIDs, wantIDs)
		}

		for _, ch := range channels {
			for _, loc := range locales {
				wantStore, _ := fromDB.HasChannelPermission(ctx, adminID, slug, ch, loc)
				gotStore, _ := fromToken.HasChannelPermission(ctx, adminID, slug, ch, loc)
				if gotStore != wantStore {
					t.Errorf("HasChannelPermission(%q, %s, %v): token %v, database %v", slug, ch, loc, gotStore, wantStore)
				}
			}
		}
	}
}
