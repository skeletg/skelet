package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Counts calls so the superuser tests can assert the checker was never consulted
type stubChecker struct {
	granted map[string]bool
	err     error
	calls   int
}

func (s *stubChecker) HasPermission(_ context.Context, _ uuid.UUID, slug string) (bool, error) {
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	return s.granted[slug], nil
}

type scopeKey struct {
	slug      string
	channelID uuid.UUID
	locale    string
}

type stubScoped struct {
	stubChecker
	global map[string]bool
	store  map[scopeKey]bool
	asked  []scopeKey
}

func (s *stubScoped) HasGlobalPermission(_ context.Context, _ uuid.UUID, slug string) (bool, error) {
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	return s.global[slug], nil
}

func (s *stubScoped) HasChannelPermission(_ context.Context, _ uuid.UUID, slug string, channelID uuid.UUID, locale *string) (bool, error) {
	s.calls++
	loc := ""
	if locale != nil {
		loc = *locale
	}
	k := scopeKey{slug: slug, channelID: channelID, locale: loc}
	s.asked = append(s.asked, k)
	if s.err != nil {
		return false, s.err
	}
	return s.store[k], nil
}

func adminCtx(superuser bool) context.Context {
	return WithAdmin(context.Background(), &Claims{AdminID: uuid.New(), Email: "a@example.com", IsSuperuser: superuser})
}

func TestRequireAuth(t *testing.T) {
	if _, err := RequireAuth(context.Background()); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("RequireAuth without claims = %v, want ErrUnauthenticated", err)
	}
	claims, err := RequireAuth(adminCtx(false))
	if err != nil || claims == nil {
		t.Fatalf("RequireAuth with claims = (%v, %v), want claims and nil", claims, err)
	}
}

// A revoked session is stamped as a nil *Claims by AdminSessionGuard - the guards must read that as absent, not deref it
func TestRequirePermission_NilClaimsIsUnauthenticated(t *testing.T) {
	ctx := WithAdmin(context.Background(), nil)
	checker := &stubChecker{}
	if err := RequirePermission(ctx, checker, "orders.view"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("RequirePermission with nil claims = %v, want ErrUnauthenticated", err)
	}
	if checker.calls != 0 {
		t.Errorf("checker consulted %d times for an unauthenticated caller, want 0", checker.calls)
	}
}

func TestRequirePermission_Unauthenticated(t *testing.T) {
	checker := &stubChecker{granted: map[string]bool{"orders.view": true}}
	if err := RequirePermission(context.Background(), checker, "orders.view"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("RequirePermission = %v, want ErrUnauthenticated", err)
	}
	if checker.calls != 0 {
		t.Errorf("checker consulted %d times before authentication, want 0", checker.calls)
	}
}

func TestRequirePermission_SuperuserBypassesChecker(t *testing.T) {
	checker := &stubChecker{} // grants nothing
	if err := RequirePermission(adminCtx(true), checker, "orders.view"); err != nil {
		t.Errorf("RequirePermission for a superuser = %v, want nil", err)
	}
	if checker.calls != 0 {
		t.Errorf("checker consulted %d times for a superuser, want 0", checker.calls)
	}
}

func TestRequirePermission_GrantedAndDenied(t *testing.T) {
	checker := &stubChecker{granted: map[string]bool{"orders.view": true}}
	if err := RequirePermission(adminCtx(false), checker, "orders.view"); err != nil {
		t.Errorf("granted slug = %v, want nil", err)
	}

	err := RequirePermission(adminCtx(false), checker, "orders.update")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("denied slug = %v, want ErrForbidden", err)
	}
	var denial *ScopedDenial
	if !errors.As(err, &denial) {
		t.Fatal("denial should be a *ScopedDenial so the dev presenter can explain it")
	}
	if denial.Slug != "orders.update" {
		t.Errorf("denial slug = %q, want %q", denial.Slug, "orders.update")
	}
	if denial.ChannelID != nil || denial.Locale != nil {
		t.Errorf("unscoped denial carried scope: %+v", denial)
	}
}

// A lookup failure must not read as a denial - a DB blip that silently became FORBIDDEN would look like a permissions bug forever
func TestRequirePermission_CheckerErrorIsNotADenial(t *testing.T) {
	boom := errors.New("db down")
	checker := &stubChecker{err: boom}
	err := RequirePermission(adminCtx(false), checker, "orders.view")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the checker error wrapped", err)
	}
	if errors.Is(err, ErrForbidden) {
		t.Error("a checker error must not classify as ErrForbidden")
	}
}

func TestRequireGlobalPermission(t *testing.T) {
	scoped := &stubScoped{global: map[string]bool{"config.manage_defaults": true}}

	if err := RequireGlobalPermission(context.Background(), scoped, "config.manage_defaults"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("unauthenticated = %v, want ErrUnauthenticated", err)
	}
	if err := RequireGlobalPermission(adminCtx(false), scoped, "config.manage_defaults"); err != nil {
		t.Errorf("global grant = %v, want nil", err)
	}
	if err := RequireGlobalPermission(adminCtx(false), scoped, "config.manage"); !errors.Is(err, ErrForbidden) {
		t.Errorf("missing global grant = %v, want ErrForbidden", err)
	}

	before := scoped.calls
	if err := RequireGlobalPermission(adminCtx(true), scoped, "config.manage"); err != nil {
		t.Errorf("superuser = %v, want nil", err)
	}
	if scoped.calls != before {
		t.Error("superuser should not consult the checker")
	}
}

func TestRequireChannelPermission_ScopeReachesTheCheckerAndTheDenial(t *testing.T) {
	eu, us := uuid.New(), uuid.New()
	de := "de"
	scoped := &stubScoped{store: map[scopeKey]bool{{slug: "catalog.manage", channelID: eu, locale: "de"}: true}}

	if err := RequireChannelPermission(adminCtx(false), scoped, "catalog.manage", eu, &de); err != nil {
		t.Errorf("matching store+locale = %v, want nil", err)
	}
	if len(scoped.asked) != 1 || scoped.asked[0].channelID != eu || scoped.asked[0].locale != "de" {
		t.Errorf("checker asked %+v, want the requested channel and locale forwarded verbatim", scoped.asked)
	}

	err := RequireChannelPermission(adminCtx(false), scoped, "catalog.manage", us, &de)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("other channel = %v, want ErrForbidden", err)
	}
	var denial *ScopedDenial
	if !errors.As(err, &denial) {
		t.Fatal("want a *ScopedDenial")
	}
	if denial.ChannelID == nil || *denial.ChannelID != us {
		t.Errorf("denial channel = %v, want %s", denial.ChannelID, us)
	}
	if denial.Locale == nil || *denial.Locale != "de" {
		t.Errorf("denial locale = %v, want de", denial.Locale)
	}
}

func TestScopedDenial_DetailsAndUnwrap(t *testing.T) {
	cid := uuid.New()
	loc := "fr"
	d := &ScopedDenial{Slug: "cms.manage", ChannelID: &cid, Locale: &loc}
	if !errors.Is(d, ErrForbidden) {
		t.Error("ScopedDenial must unwrap to ErrForbidden so existing callers keep working")
	}
	if d.Error() != ErrForbidden.Error() {
		t.Errorf("Error() = %q, want it to read exactly like ErrForbidden", d.Error())
	}
	slug, channelID, locale := d.DenialDetails()
	if slug != "cms.manage" || channelID != cid.String() || locale != "fr" {
		t.Errorf("DenialDetails = (%q, %q, %q)", slug, channelID, locale)
	}

	bare := &ScopedDenial{Slug: "cms.manage"}
	if _, channelID, locale := bare.DenialDetails(); channelID != "" || locale != "" {
		t.Errorf("unscoped denial rendered channel=%q locale=%q, want both empty", channelID, locale)
	}
}

func tokenCtx(slugs ...string) context.Context {
	perms := make(map[string]struct{}, len(slugs))
	for _, s := range slugs {
		perms[s] = struct{}{}
	}
	return WithApiToken(context.Background(), &ApiTokenClaims{TokenID: uuid.New(), Name: "ERP Bridge", Permissions: perms})
}

// An integration token's grant is global, so one granted slug must satisfy the unscoped, global AND store-scoped guards - and never touch the admin checker, whose tables don't know the token
func TestGuards_IntegrationTokenGrantIsGlobal(t *testing.T) {
	ctx := tokenCtx("products.manage")
	plain := &stubChecker{}
	scoped := &stubScoped{}

	if err := RequirePermission(ctx, plain, "products.manage"); err != nil {
		t.Errorf("RequirePermission with a granted token = %v, want nil", err)
	}
	if err := RequireGlobalPermission(ctx, scoped, "products.manage"); err != nil {
		t.Errorf("RequireGlobalPermission with a granted token = %v, want nil", err)
	}
	if err := RequireChannelPermission(ctx, scoped, "products.manage", uuid.New(), nil); err != nil {
		t.Errorf("RequireChannelPermission with a granted token = %v, want nil", err)
	}
	if err := RequireAllListedChannelsPermission(ctx, scoped, "products.manage", []uuid.UUID{uuid.New(), uuid.New()}, "de"); err != nil {
		t.Errorf("RequireAllListedChannelsPermission with a granted token = %v, want nil", err)
	}
	if plain.calls != 0 || scoped.calls != 0 {
		t.Errorf("admin checker consulted (%d, %d) times for a token caller, want 0 - token grants live on the claims", plain.calls, scoped.calls)
	}
}

func TestGuards_IntegrationTokenUngrantedSlugIsForbidden(t *testing.T) {
	ctx := tokenCtx("products.view")
	scoped := &stubScoped{}
	for name, err := range map[string]error{
		"unscoped": RequirePermission(ctx, &stubChecker{}, "orders.refund"),
		"global":   RequireGlobalPermission(ctx, scoped, "orders.refund"),
		"store":    RequireChannelPermission(ctx, scoped, "orders.refund", uuid.New(), nil),
		"listed":   RequireAllListedChannelsPermission(ctx, scoped, "orders.refund", []uuid.UUID{uuid.New()}, ""),
	} {
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("%s guard with an ungranted token slug = %v, want ErrForbidden", name, err)
		}
	}
}

// A request carrying both credentials authorizes purely as the admin - the token must never rescue an admin's denial, or a leaked low-privilege login could be escalated by attaching a broad token
func TestGuards_AdminClaimsWinOverToken(t *testing.T) {
	ctx := WithApiToken(adminCtx(false), &ApiTokenClaims{TokenID: uuid.New(), Permissions: map[string]struct{}{"orders.refund": {}}})
	checker := &stubChecker{} // grants the admin nothing
	if err := RequirePermission(ctx, checker, "orders.refund"); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin denied + token granted = %v, want the admin's ErrForbidden", err)
	}
	if checker.calls != 1 {
		t.Errorf("checker consulted %d times, want 1 - the admin path must run", checker.calls)
	}
}

// A revoked token stamp (nil claims) reads as anonymous, mirroring the AdminSessionGuard convention
func TestGuards_NilTokenClaimsIsUnauthenticated(t *testing.T) {
	ctx := WithApiToken(context.Background(), nil)
	if err := RequirePermission(ctx, &stubChecker{}, "orders.view"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("nil token claims = %v, want ErrUnauthenticated", err)
	}
}

func TestRequireScopedOrGlobal_AnUntargetedRunNeedsTheGlobalGrant(t *testing.T) {
	ch := uuid.New()
	scoped := &stubScoped{store: map[scopeKey]bool{{slug: "customers.manage", channelID: ch}: true}}
	scoped.granted = map[string]bool{"customers.manage": true}
	if err := RequireScopedOrGlobal(adminCtx(false), scoped, "customers.manage", ch, false); err != nil {
		t.Errorf("targeted run on the granted channel = %v, want nil", err)
	}
	if err := RequireScopedOrGlobal(adminCtx(false), scoped, "customers.manage", uuid.Nil, false); !errors.Is(err, ErrForbidden) {
		t.Errorf("untargeted run on a channel-scoped grant = %v, want ErrForbidden - it reaches every channel", err)
	}
	if err := RequireScopedOrGlobal(adminCtx(false), scoped, "customers.manage", ch, true); !errors.Is(err, ErrForbidden) {
		t.Errorf("explicitly global run on a channel-scoped grant = %v, want ErrForbidden", err)
	}
	scoped.global = map[string]bool{"customers.manage": true}
	if err := RequireScopedOrGlobal(adminCtx(false), scoped, "customers.manage", uuid.Nil, false); err != nil {
		t.Errorf("untargeted run on a global grant = %v, want nil", err)
	}
}
