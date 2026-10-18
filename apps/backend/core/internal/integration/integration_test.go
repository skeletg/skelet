package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Keyed by token hash, the way the real unique index is - a fake that indexed by raw token would
// hide the whole point, which is that the raw token is never stored
type fakeRepo struct {
	Repository
	byHash    map[string]Integration
	createErr error
	lookups   []string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byHash: map[string]Integration{}} }

func (r *fakeRepo) Create(_ context.Context, in CreateIntegrationInput, tokenHash string) (Integration, error) {
	if r.createErr != nil {
		return Integration{}, r.createErr
	}
	i := Integration{ID: uuid.New(), Name: in.Name, Description: in.Description, IsActive: true, ExpiresAt: in.ExpiresAt}
	for _, slug := range in.Permissions {
		i.Permissions = append(i.Permissions, Permission{ID: uuid.New(), Slug: slug})
	}
	r.byHash[tokenHash] = i
	return i, nil
}

func (r *fakeRepo) GetByHash(_ context.Context, tokenHash string) (Integration, error) {
	r.lookups = append(r.lookups, tokenHash)
	i, ok := r.byHash[tokenHash]
	if !ok {
		return Integration{}, ErrNotFound
	}
	return i, nil
}

func (r *fakeRepo) RegenerateTokenHash(_ context.Context, id uuid.UUID, newHash string) (Integration, error) {
	for hash, i := range r.byHash {
		if i.ID == id {
			delete(r.byHash, hash)
			r.byHash[newHash] = i
			return i, nil
		}
	}
	return Integration{}, ErrNotFound
}

// Replaces the stored row for an integration, keeping its hash - used to flip IsActive or the expiry
func (r *fakeRepo) put(i Integration) {
	for hash, existing := range r.byHash {
		if existing.ID == i.ID {
			r.byHash[hash] = i
			return
		}
	}
}

func newTestService(t *testing.T, perms ...string) (*Service, *fakeRepo, CreateIntegrationResult) {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(repo)
	res, err := svc.Create(context.Background(), CreateIntegrationInput{Name: "Warehouse", Permissions: perms})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	return svc, repo, res
}

// The raw token is shown once and must never be recoverable from the database - only its hash lands
func TestCreateStoresOnlyTheTokenHash(t *testing.T) {
	_, repo, res := newTestService(t, "orders.view")

	if !strings.HasPrefix(res.RawToken, "sk_") {
		t.Errorf("raw token = %q, want the sk_ prefix consumers key on", res.RawToken)
	}
	if len(res.RawToken) != 3+64 {
		t.Errorf("raw token is %d chars, want 3 + 64 hex of 32 random bytes", len(res.RawToken))
	}
	for hash := range repo.byHash {
		if strings.Contains(hash, res.RawToken) || hash == res.RawToken {
			t.Fatal("the raw token itself was stored, want only its hash")
		}
		want := sha256.Sum256([]byte(res.RawToken))
		if hash != hex.EncodeToString(want[:]) {
			t.Errorf("stored hash = %q, want the sha256 of the raw token", hash)
		}
	}
}

// Two integrations sharing a token would let one impersonate the other
func TestCreateMintsADistinctTokenEachTime(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	seen := make(map[string]bool, 20)
	for range 20 {
		res, err := svc.Create(context.Background(), CreateIntegrationInput{Name: "n"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if seen[res.RawToken] {
			t.Fatalf("token %q was minted twice", res.RawToken)
		}
		seen[res.RawToken] = true
	}
}

func TestValidateAcceptsTheMintedToken(t *testing.T) {
	svc, _, res := newTestService(t, "orders.view", "products.view")

	got, err := svc.Validate(context.Background(), res.RawToken)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ID != res.Integration.ID {
		t.Errorf("resolved integration = %v, want %v", got.ID, res.Integration.ID)
	}
	if len(got.Permissions) != 2 {
		t.Errorf("permissions = %v, want both slugs", got.Permissions)
	}
}

// A wrong token must not distinguish "no such integration" from any other failure, and it must never
// reach the repository as anything but a hash
func TestValidateRejectsUnknownTokens(t *testing.T) {
	svc, repo, res := newTestService(t)

	for name, token := range map[string]string{
		"empty":          "",
		"wrong value":    "sk_" + strings.Repeat("a", 64),
		"no prefix":      strings.TrimPrefix(res.RawToken, "sk_"),
		"truncated":      res.RawToken[:len(res.RawToken)-1],
		"case flipped":   strings.ToUpper(res.RawToken),
		"trailing space": res.RawToken + " ",
	} {
		if _, err := svc.Validate(context.Background(), token); !errors.Is(err, errTokenRefused) {
			t.Errorf("Validate with a %s token = %v, want it refused", name, err)
		}
	}
	for _, looked := range repo.lookups {
		if strings.HasPrefix(looked, "sk_") {
			t.Errorf("the repository was queried with %q, want a hash rather than a raw token", looked)
		}
	}
}

// Disabling is meant to take effect on the next request with no TTL on the decision
func TestValidateRefusesADisabledIntegration(t *testing.T) {
	svc, repo, res := newTestService(t)
	disabled := res.Integration
	disabled.IsActive = false
	repo.put(disabled)

	if _, err := svc.Validate(context.Background(), res.RawToken); err == nil {
		t.Error("a disabled integration validated, want it refused")
	}
}

func TestValidateRefusesAnExpiredToken(t *testing.T) {
	svc, repo, res := newTestService(t)

	expired := res.Integration
	expired.ExpiresAt = sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true}
	repo.put(expired)
	if _, err := svc.Validate(context.Background(), res.RawToken); err == nil {
		t.Error("an expired token validated, want it refused")
	}

	future := res.Integration
	future.ExpiresAt = sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
	repo.put(future)
	if _, err := svc.Validate(context.Background(), res.RawToken); err != nil {
		t.Errorf("a token expiring in an hour = %v, want it accepted", err)
	}

	noExpiry := res.Integration
	noExpiry.ExpiresAt = sql.NullTime{}
	repo.put(noExpiry)
	if _, err := svc.Validate(context.Background(), res.RawToken); err != nil {
		t.Errorf("a token with no expiry = %v, want it accepted", err)
	}
}

// Rotation exists to cut off a leaked token, so the old one has to stop working the moment the new
// one is issued
func TestRegenerateInvalidatesTheOldToken(t *testing.T) {
	svc, _, res := newTestService(t)

	rotated, err := svc.RegenerateToken(context.Background(), res.Integration.ID)
	if err != nil {
		t.Fatalf("RegenerateToken: %v", err)
	}
	if rotated.RawToken == res.RawToken {
		t.Fatal("rotation returned the same token")
	}
	if _, err := svc.Validate(context.Background(), res.RawToken); err == nil {
		t.Error("the old token still validates after rotation")
	}
	if _, err := svc.Validate(context.Background(), rotated.RawToken); err != nil {
		t.Errorf("the rotated token = %v, want it accepted", err)
	}
}

func TestCreatePropagatesRepositoryErrors(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = errors.New("unique violation")
	if _, err := NewService(repo).Create(context.Background(), CreateIntegrationInput{Name: "n"}); !errors.Is(err, repo.createErr) {
		t.Errorf("Create with a failing repo = %v, want %v", err, repo.createErr)
	}
}

func handlerRecordingClaims(seen **auth.ApiTokenClaims) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, ok := auth.ApiTokenFromContext(r.Context()); ok {
			*seen = c
		}
		w.WriteHeader(http.StatusOK)
	})
}

// The middleware is deliberately non-blocking - it attaches claims when the token is good and
// otherwise leaves the request anonymous for the resolver's own permission gate to refuse
func TestMiddlewareAttachesClaimsForAValidToken(t *testing.T) {
	svc, _, res := newTestService(t, "orders.view", "products.view")
	var seen *auth.ApiTokenClaims

	req := httptest.NewRequest(http.MethodPost, "/api/admin/graphql", nil)
	req.Header.Set(HeaderAPIToken, res.RawToken)
	rec := httptest.NewRecorder()
	Middleware(svc)(handlerRecordingClaims(&seen)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 - the middleware must not block", rec.Code)
	}
	if seen == nil {
		t.Fatal("no claims were attached for a valid token")
	}
	if seen.TokenID != res.Integration.ID {
		t.Errorf("TokenID = %v, want %v", seen.TokenID, res.Integration.ID)
	}
	if seen.Name != res.Integration.Name {
		t.Errorf("Name = %q, want %q - audit attribution reads it", seen.Name, res.Integration.Name)
	}
	for _, slug := range []string{"orders.view", "products.view"} {
		if _, ok := seen.Permissions[slug]; !ok {
			t.Errorf("permission %q missing from the claims, want it carried through", slug)
		}
	}
	if _, ok := seen.Permissions["orders.write"]; ok {
		t.Error("a permission the integration was never granted appeared in the claims")
	}
}

// An anonymous or bad-token request must reach the handler with NO claims - attaching an empty set
// would read as an authenticated caller holding nothing, which is a different thing
func TestMiddlewareLeavesBadTokensAnonymous(t *testing.T) {
	svc, _, res := newTestService(t, "orders.view")

	for name, token := range map[string]string{
		"no header":    "",
		"wrong prefix": "Bearer " + res.RawToken,
		"unknown sk_":  "sk_" + strings.Repeat("f", 64),
		"garbage":      "not-a-token",
	} {
		var seen *auth.ApiTokenClaims
		req := httptest.NewRequest(http.MethodPost, "/api/admin/graphql", nil)
		if token != "" {
			req.Header.Set(HeaderAPIToken, token)
		}
		rec := httptest.NewRecorder()
		Middleware(svc)(handlerRecordingClaims(&seen)).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want the request passed through unblocked", name, rec.Code)
		}
		if seen != nil {
			t.Errorf("%s: claims %v were attached, want the request left anonymous", name, seen.TokenID)
		}
	}
}

// Disabling an integration has to strip its claims on the very next request, not merely stop new
// tokens being minted
func TestMiddlewareDropsClaimsOnceTheIntegrationIsDisabled(t *testing.T) {
	svc, repo, res := newTestService(t, "orders.view")

	call := func() *auth.ApiTokenClaims {
		var seen *auth.ApiTokenClaims
		req := httptest.NewRequest(http.MethodPost, "/api/admin/graphql", nil)
		req.Header.Set(HeaderAPIToken, res.RawToken)
		rec := httptest.NewRecorder()
		Middleware(svc)(handlerRecordingClaims(&seen)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want the request passed through unblocked", rec.Code)
		}
		return seen
	}

	if call() == nil {
		t.Fatal("no claims while the integration is active")
	}
	off := res.Integration
	off.IsActive = false
	repo.put(off)
	if got := call(); got != nil {
		t.Errorf("claims %v survived disabling, want the request left anonymous", got.TokenID)
	}
}

func TestCreateRefusesABlankNameOrAPastExpiry(t *testing.T) {
	svc := NewService(newFakeRepo())
	now := time.Now()
	svc.now = func() time.Time { return now }
	ctx := context.Background()
	for name, in := range map[string]CreateIntegrationInput{
		"blank":     {Name: "   "},
		"too long":  {Name: strings.Repeat("é", maxNameRunes+1)},
		"expired":   {Name: "n", ExpiresAt: sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true}},
		"right now": {Name: "n", ExpiresAt: sql.NullTime{Time: now, Valid: true}},
	} {
		if _, err := svc.Create(ctx, in); err == nil {
			t.Errorf("%s: created, want it refused", name)
		}
	}
	res, err := svc.Create(ctx, CreateIntegrationInput{Name: "  Warehouse  ", ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}})
	if err != nil || res.Integration.Name != "Warehouse" {
		t.Fatalf("create = %+v, %v - want the trimmed name accepted", res.Integration, err)
	}
	if _, err := svc.Create(ctx, CreateIntegrationInput{Name: strings.Repeat("é", maxNameRunes)}); err != nil {
		t.Errorf("a name at the cap = %v, want it accepted", err)
	}
}

func TestUpdateRefusesABlankNameOrAPastExpiry(t *testing.T) {
	svc := NewService(newFakeRepo())
	blank, past := " ", time.Now().Add(-time.Hour)
	if _, err := svc.Update(context.Background(), UpdateIntegrationInput{Name: &blank}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("blank name = %v, want ErrNameRequired", err)
	}
	if _, err := svc.Update(context.Background(), UpdateIntegrationInput{ExpiresAt: &past}); !errors.Is(err, ErrExpiryInPast) {
		t.Errorf("past expiry = %v, want ErrExpiryInPast", err)
	}
}

type failingRepo struct{ Repository }

func (failingRepo) GetByHash(context.Context, string) (Integration, error) {
	return Integration{}, errors.New("connection refused")
}

// A lookup that failed is not a token that was refused - the middleware logs one and not the other
func TestValidateKeepsALookupFailureApartFromARefusal(t *testing.T) {
	_, err := NewService(failingRepo{}).Validate(context.Background(), tokenPrefix+strings.Repeat("a", 64))
	if err == nil || errors.Is(err, errTokenRefused) {
		t.Fatalf("Validate with the database down = %v, want the lookup error, not a refusal", err)
	}
}

// Still anonymous either way, but only a failed lookup is logged - a bad token is the caller's business
func TestMiddlewareLogsALookupFailureButNotARefusal(t *testing.T) {
	for name, tc := range map[string]struct {
		svc    *Service
		logged bool
	}{
		"database down": {NewService(failingRepo{}), true},
		"unknown token": {NewService(newFakeRepo()), false},
	} {
		var buf bytes.Buffer
		req := httptest.NewRequest(http.MethodPost, "/api/admin/graphql", nil)
		req = req.WithContext(applog.Into(req.Context(), slog.New(slog.NewTextHandler(&buf, nil))))
		req.Header.Set(HeaderAPIToken, tokenPrefix+strings.Repeat("a", 64))
		var seen *auth.ApiTokenClaims
		Middleware(tc.svc)(handlerRecordingClaims(&seen)).ServeHTTP(httptest.NewRecorder(), req)
		if seen != nil {
			t.Errorf("%s: claims attached", name)
		}
		if got := strings.Contains(buf.String(), "integration token lookup failed"); got != tc.logged {
			t.Errorf("%s: logged = %v, want %v (%s)", name, got, tc.logged, buf.String())
		}
	}
}
