package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubScopeProvider struct {
	stubScoped
	permitted   []uuid.UUID
	listErr     error
	listCalls   int
	globalCalls int
}

func (s *stubScopeProvider) HasGlobalPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error) {
	s.globalCalls++
	return s.stubScoped.HasGlobalPermission(ctx, adminID, slug)
}

func (s *stubScopeProvider) ListPermittedChannelIDs(_ context.Context, _ uuid.UUID, _ string) ([]uuid.UUID, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.permitted, nil
}

// No admin in context means an internal caller - background jobs and the CLI have no claims to scope by
func TestLoadPermittedChannels_NoAdminBypasses(t *testing.T) {
	svc := &stubScopeProvider{}
	p, err := LoadPermittedChannels(context.Background(), svc, "orders.view")
	if err != nil {
		t.Fatalf("LoadPermittedChannels: %v", err)
	}
	if !p.Bypass() {
		t.Error("no admin should bypass")
	}
	if svc.globalCalls != 0 || svc.listCalls != 0 {
		t.Error("no admin should not hit the service at all")
	}
}

func TestLoadPermittedChannels_SuperuserBypasses(t *testing.T) {
	svc := &stubScopeProvider{}
	p, err := LoadPermittedChannels(adminCtx(true), svc, "orders.view")
	if err != nil {
		t.Fatalf("LoadPermittedChannels: %v", err)
	}
	if !p.Bypass() {
		t.Error("superuser should bypass")
	}
	if svc.globalCalls != 0 || svc.listCalls != 0 {
		t.Error("superuser should not hit the service at all")
	}
}

func TestLoadPermittedChannels_GlobalGrantBypassesWithoutListing(t *testing.T) {
	svc := &stubScopeProvider{global: map[string]bool{"orders.view": true}}
	p, err := LoadPermittedChannels(adminCtx(false), svc, "orders.view")
	if err != nil {
		t.Fatalf("LoadPermittedChannels: %v", err)
	}
	if !p.Bypass() {
		t.Error("a global grant should bypass")
	}
	if svc.listCalls != 0 {
		t.Error("a global grant should short-circuit before listing channels")
	}
	if !p.Allows(uuid.New()) {
		t.Error("bypass should allow any channel, including ones never granted")
	}
}

func TestLoadPermittedChannels_ScopedSet(t *testing.T) {
	eu, us, apac := uuid.New(), uuid.New(), uuid.New()
	svc := &stubScopeProvider{permitted: []uuid.UUID{eu, us}}

	p, err := LoadPermittedChannels(adminCtx(false), svc, "orders.view")
	if err != nil {
		t.Fatalf("LoadPermittedChannels: %v", err)
	}
	if p.Bypass() {
		t.Fatal("a channel-scoped admin must not bypass")
	}
	if !p.Allows(eu) || !p.Allows(us) {
		t.Error("granted channels should be allowed")
	}
	if p.Allows(apac) {
		t.Error("ungranted channel allowed")
	}
	if len(p.IDs()) != 2 {
		t.Errorf("IDs() = %v, want the two granted channels", p.IDs())
	}
}

func TestLoadPermittedChannels_ErrorsPropagate(t *testing.T) {
	boom := errors.New("db down")

	svc := &stubScopeProvider{err: boom}
	if _, err := LoadPermittedChannels(adminCtx(false), svc, "orders.view"); !errors.Is(err, boom) {
		t.Errorf("global lookup error = %v, want it propagated", err)
	}

	svc2 := &stubScopeProvider{listErr: boom}
	p, err := LoadPermittedChannels(adminCtx(false), svc2, "orders.view")
	if !errors.Is(err, boom) {
		t.Errorf("list error = %v, want it propagated", err)
	}
	if p.Bypass() {
		t.Error("the zero PermittedChannels returned on error must not bypass")
	}
}

// The nil-vs-empty distinction is load-bearing: callers pass IDs() straight into WHERE channel_id = ANY(...)
// and read nil as "no filter". An empty set collapsing to nil would show a zero-channel admin every channel
func TestPermittedChannels_IDsNilOnlyOnBypass(t *testing.T) {
	bypass := PermittedChannels{bypass: true}
	if bypass.IDs() != nil {
		t.Errorf("bypass IDs() = %v, want nil (no channel filter)", bypass.IDs())
	}

	none := PermittedChannels{ids: map[uuid.UUID]struct{}{}}
	if ids := none.IDs(); ids == nil {
		t.Error("an admin permitted on zero channels must return a non-nil empty slice, not nil")
	} else if len(ids) != 0 {
		t.Errorf("IDs() = %v, want empty", ids)
	}
	if none.Allows(uuid.New()) {
		t.Error("an empty non-bypass set must allow nothing")
	}
}

func TestPermittedChannels_AllowsOptional(t *testing.T) {
	eu := uuid.New()
	scoped := PermittedChannels{ids: map[uuid.UUID]struct{}{eu: {}}}

	if scoped.AllowsOptional(nil) {
		t.Error("a nil channel is a global row - a channel-scoped admin must not reach it")
	}
	if !scoped.AllowsOptional(&eu) {
		t.Error("granted channel should be allowed")
	}
	other := uuid.New()
	if scoped.AllowsOptional(&other) {
		t.Error("ungranted channel allowed")
	}
	if !(PermittedChannels{bypass: true}).AllowsOptional(nil) {
		t.Error("bypass should reach global rows")
	}
}

func TestFilterByChannel(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	type row struct{ ch uuid.UUID }
	items := []row{{eu}, {us}, {eu}}
	chOf := func(r row) uuid.UUID { return r.ch }

	kept := FilterByChannel(items, PermittedChannels{bypass: true}, chOf)
	if len(kept) != 3 {
		t.Errorf("bypass filtered %d of 3 rows", 3-len(kept))
	}

	kept = FilterByChannel(items, PermittedChannels{ids: map[uuid.UUID]struct{}{eu: {}}}, chOf)
	if len(kept) != 2 {
		t.Fatalf("got %d rows, want the 2 EU rows", len(kept))
	}
	for _, r := range kept {
		if r.ch != eu {
			t.Errorf("leaked a row from channel %s", r.ch)
		}
	}

	if got := FilterByChannel(items, PermittedChannels{ids: map[uuid.UUID]struct{}{}}, chOf); len(got) != 0 {
		t.Errorf("zero-channel admin saw %d rows, want 0", len(got))
	}
}

func TestRequireGlobalOrChannelPermission(t *testing.T) {
	eu := uuid.New()
	svc := &stubScoped{
		global: map[string]bool{"promotions.manage": true},
		store:  map[scopeKey]bool{{slug: "promotions.manage", channelID: eu}: true},
	}

	// nil channel is a global rule - it needs the global grant, not a channel one
	if err := RequireGlobalOrChannelPermission(adminCtx(false), svc, "promotions.manage", nil); err != nil {
		t.Errorf("global rule with a global grant = %v, want nil", err)
	}
	if err := RequireGlobalOrChannelPermission(adminCtx(false), svc, "promotions.manage", &eu); err != nil {
		t.Errorf("channel rule with a channel grant = %v, want nil", err)
	}

	scopedOnly := &stubScoped{store: map[scopeKey]bool{{slug: "promotions.manage", channelID: eu}: true}}
	if err := RequireGlobalOrChannelPermission(adminCtx(false), scopedOnly, "promotions.manage", nil); !errors.Is(err, ErrForbidden) {
		t.Errorf("a channel-scoped admin editing a global rule = %v, want ErrForbidden", err)
	}
}

// Shared locale content has no channel column, so an (EU, de) grant must not reach a US-listed entity
func TestRequireAllListedChannelsPermission_DemandsEveryListedChannel(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	svc := &stubScoped{store: map[scopeKey]bool{{slug: "blog.manage", channelID: eu, locale: "de"}: true}}

	if err := RequireAllListedChannelsPermission(adminCtx(false), svc, "blog.manage", []uuid.UUID{eu}, "de"); err != nil {
		t.Errorf("only-listed channel granted = %v, want nil", err)
	}

	err := RequireAllListedChannelsPermission(adminCtx(false), svc, "blog.manage", []uuid.UUID{eu, us}, "de")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("EU-only grant on an EU+US listing = %v, want ErrForbidden", err)
	}
}

func TestRequireAllListedChannelsPermission_GlobalAndSuperuserShortCircuit(t *testing.T) {
	eu, us := uuid.New(), uuid.New()

	global := &stubScoped{global: map[string]bool{"blog.manage": true}}
	if err := RequireAllListedChannelsPermission(adminCtx(false), global, "blog.manage", []uuid.UUID{eu, us}, "de"); err != nil {
		t.Errorf("global grant = %v, want nil", err)
	}
	if len(global.asked) != 0 {
		t.Error("a global grant should not fall through to per-channel checks")
	}

	su := &stubScoped{}
	if err := RequireAllListedChannelsPermission(adminCtx(true), su, "blog.manage", []uuid.UUID{eu}, "de"); err != nil {
		t.Errorf("superuser = %v, want nil", err)
	}
	if su.calls != 0 {
		t.Error("superuser should not consult the checker")
	}

	if err := RequireAllListedChannelsPermission(context.Background(), su, "blog.manage", nil, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("unauthenticated = %v, want ErrUnauthenticated", err)
	}
}

// An entity listed nowhere has no leak surface, so it falls back to a bare slug check
func TestRequireAllListedChannelsPermission_UnlistedFallsBackToAnyScope(t *testing.T) {
	svc := &stubScoped{granted: map[string]bool{"blog.manage": true}}
	if err := RequireAllListedChannelsPermission(adminCtx(false), svc, "blog.manage", nil, "de"); err != nil {
		t.Errorf("unlisted entity with the slug at some scope = %v, want nil", err)
	}

	denied := &stubScoped{}
	if err := RequireAllListedChannelsPermission(adminCtx(false), denied, "blog.manage", nil, "de"); !errors.Is(err, ErrForbidden) {
		t.Errorf("unlisted entity without the slug = %v, want ErrForbidden", err)
	}
}

// An empty locale means "not locale-specific" and must reach the checker as nil, not as the empty-string locale
func TestRequireAllListedChannelsPermission_EmptyLocaleIsNil(t *testing.T) {
	eu := uuid.New()
	svc := &stubScoped{store: map[scopeKey]bool{{slug: "blog.manage", channelID: eu}: true}}
	if err := RequireAllListedChannelsPermission(adminCtx(false), svc, "blog.manage", []uuid.UUID{eu}, ""); err != nil {
		t.Errorf("empty locale = %v, want nil", err)
	}
	if len(svc.asked) != 1 || svc.asked[0].locale != "" {
		t.Errorf("checker asked %+v, want a nil locale", svc.asked)
	}
}
