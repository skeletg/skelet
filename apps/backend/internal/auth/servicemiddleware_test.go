package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The token is the only evidence a service API has that the caller is who they say and holds what they claim, so any way past this middleware without one is a bypass

// Stands in for a resolver on the Signals surface, reporting what its guard decided
func guardedHandler(slug string) (http.Handler, *error) {
	var got error
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = RequirePermission(r.Context(), GrantCheckerFrom(r.Context()), slug)
	})
	return h, &got
}

func serviceRequest(t *testing.T, h http.Handler, token string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/signals/graphql", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestServiceMiddlewareOnlyAdmitsATokenForThisAudience(t *testing.T) {
	adminID := uuid.New()
	held := ServiceClaims{AdminID: adminID, Email: "ops@example.test", Grants: []Grant{globalGrant("experiments.view")}}

	forUs, err := GenerateServiceToken(held, "signals", svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	forSomethingElse, err := GenerateServiceToken(held, "search", svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	adminTok, err := GenerateToken(adminID, uuid.New(), "ops@example.test", false, svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	expired, err := GenerateServiceToken(held, "signals", svcSecret, -time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}

	for name, tc := range map[string]struct {
		token   string
		wantErr error
	}{
		"no token at all":             {"", ErrUnauthenticated},
		"a token for another service": {forSomethingElse, ErrUnauthenticated},
		"an admin token":              {adminTok, ErrUnauthenticated},
		"a token signed elsewhere":    {"not.a.token", ErrUnauthenticated},
		"an expired token":            {expired, ErrUnauthenticated},
		"a token minted for this one": {forUs, nil},
	} {
		t.Run(name, func(t *testing.T) {
			h, got := guardedHandler("experiments.view")
			serviceRequest(t, ServiceMiddleware("signals", svcSecret)(h), tc.token)
			if tc.wantErr == nil {
				if *got != nil {
					t.Errorf("guard refused a valid caller: %v", *got)
				}
				return
			}
			if !errors.Is(*got, tc.wantErr) {
				t.Errorf("guard returned %v, want %v", *got, tc.wantErr)
			}
		})
	}
}

func TestServiceTokenAuthorizesOnlyWhatItCarries(t *testing.T) {
	adminID := uuid.New()
	viewer := ServiceClaims{AdminID: adminID, Grants: []Grant{globalGrant("experiments.view")}}
	super := ServiceClaims{AdminID: adminID, IsSuperuser: true}

	for name, tc := range map[string]struct {
		claims ServiceClaims
		slug   string
		wantOK bool
	}{
		"the grant it carries":           {viewer, "experiments.view", true},
		"a grant it does not":            {viewer, "experiments.manage", false},
		"a slug from another suite":      {viewer, "lens.view", false},
		"a superuser carrying no grants": {super, "experiments.manage", true},
	} {
		t.Run(name, func(t *testing.T) {
			token, err := GenerateServiceToken(tc.claims, "signals", svcSecret, time.Minute)
			if err != nil {
				t.Fatalf("GenerateServiceToken: %v", err)
			}
			h, got := guardedHandler(tc.slug)
			serviceRequest(t, ServiceMiddleware("signals", svcSecret)(h), token)
			if tc.wantOK != (*got == nil) {
				t.Errorf("guard on %s returned %v, want allowed=%v", tc.slug, *got, tc.wantOK)
			}
		})
	}
}

// A service token is self-describing, so nothing in it changes when core kills the session - the shared revocation
// list is the only thing standing between a logged-out operator and the rest of their token's life
type fakeRevocations struct {
	killed map[uuid.UUID]bool
	fail   bool
}

func (f *fakeRevocations) Revoke(_ context.Context, id uuid.UUID) { f.killed[id] = true }

func (f *fakeRevocations) Revoked(_ context.Context, id uuid.UUID) bool {
	if f.fail {
		return false // an unreachable list fails open, same stance as AdminSessionGuard
	}
	return f.killed[id]
}

func TestServiceSessionGuardDropsARevokedSession(t *testing.T) {
	sessionID := uuid.New()
	claims := ServiceClaims{AdminID: uuid.New(), SessionID: sessionID, Grants: []Grant{globalGrant("experiments.view")}}
	token, err := GenerateServiceToken(claims, "signals", svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}

	for name, tc := range map[string]struct {
		list    *fakeRevocations
		allowed bool
	}{
		"a live session":                 {&fakeRevocations{killed: map[uuid.UUID]bool{}}, true},
		"a session core killed":          {&fakeRevocations{killed: map[uuid.UUID]bool{sessionID: true}}, false},
		"another admin's killed session": {&fakeRevocations{killed: map[uuid.UUID]bool{uuid.New(): true}}, true},
		"an unreachable list":            {&fakeRevocations{killed: map[uuid.UUID]bool{sessionID: true}, fail: true}, true},
	} {
		t.Run(name, func(t *testing.T) {
			h, got := guardedHandler("experiments.view")
			chain := ServiceMiddleware("signals", svcSecret)(ServiceSessionGuard(tc.list)(h))
			serviceRequest(t, chain, token)
			if tc.allowed != (*got == nil) {
				t.Errorf("guard returned %v, want allowed=%v", *got, tc.allowed)
			}
		})
	}
}

// A token minted with no session row behind it (dev-impersonate, and any path that skips one) has nothing to revoke
func TestServiceSessionGuardPassesASessionlessToken(t *testing.T) {
	claims := ServiceClaims{AdminID: uuid.New(), Grants: []Grant{globalGrant("experiments.view")}}
	token, err := GenerateServiceToken(claims, "signals", svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	h, got := guardedHandler("experiments.view")
	chain := ServiceMiddleware("signals", svcSecret)(ServiceSessionGuard(&fakeRevocations{killed: map[uuid.UUID]bool{}})(h))
	serviceRequest(t, chain, token)
	if *got != nil {
		t.Errorf("a token with no session id was refused: %v", *got)
	}
}
