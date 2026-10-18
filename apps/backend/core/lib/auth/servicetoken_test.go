package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const svcSecret = "service-token-test-secret"

var svcKeys = MustServiceKeys(svcSecret)

// A service token is a credential for one service and one slug set - widening either edge turns an analytics token into one that authorizes more of the admin API

func signalsClaims(grants ...Grant) ServiceClaims {
	return ServiceClaims{AdminID: uuid.New(), SessionID: uuid.New(), Email: "ops@example.test", Grants: grants}
}

func TestServiceTokenRoundTripsTheCallerAndTheirGrants(t *testing.T) {
	eu := uuid.New()
	want := signalsClaims(channelGrant("lens.view", eu), globalGrant("experiments.manage"))

	token, err := GenerateServiceToken(want, "signals", svcKeys, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	got, err := ValidateServiceToken(token, "signals", svcKeys)
	if err != nil {
		t.Fatalf("ValidateServiceToken: %v", err)
	}
	if got.AdminID != want.AdminID || got.SessionID != want.SessionID || got.Email != want.Email {
		t.Errorf("identity = %+v, want %+v", got, want)
	}

	// The point of carrying grants: the receiving service answers the same checks core would
	check := got.Checker()
	ctx := context.Background()
	if ok, err := check.HasChannelPermission(ctx, got.AdminID, "lens.view", eu, nil); err != nil || !ok {
		t.Errorf("lens.view on its channel = (%v, %v), want true", ok, err)
	}
	if ok, _ := check.HasChannelPermission(ctx, got.AdminID, "lens.view", uuid.New(), nil); ok {
		t.Error("a channel-scoped grant authorized another channel")
	}
	if ok, err := check.HasPermission(ctx, got.AdminID, "experiments.view"); err != nil || !ok {
		t.Errorf("manage should imply view through the token = (%v, %v)", ok, err)
	}
}

func TestServiceTokenRefusesAnotherAudience(t *testing.T) {
	token, err := GenerateServiceToken(signalsClaims(globalGrant("lens.view")), "signals", svcKeys, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	for name, aud := range map[string]string{"another service": "search", "no audience at all": ""} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateServiceToken(token, aud, svcKeys); err == nil {
				t.Error("token validated outside the audience it was minted for")
			}
		})
	}
	if _, err := GenerateServiceToken(signalsClaims(), "", svcKeys, time.Minute); err == nil {
		t.Error("minted an audience-less token, which would validate anywhere")
	}
}

// Even an operator who reused one value for both keys can't turn either token into the other - audience and type still keep them apart
func TestServiceTokenIsNotInterchangeableWithAnAdminToken(t *testing.T) {
	adminID := uuid.New()
	sameValue, _ := NewAdminKeys(svcSecret)
	adminTok, err := GenerateToken(adminID, uuid.New(), "ops@example.test", false, sameValue, time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := ValidateServiceToken(adminTok, "signals", svcKeys); err == nil {
		t.Error("an admin token passed as a service token")
	}

	svcTok, err := GenerateServiceToken(signalsClaims(globalGrant("lens.view")), "signals", svcKeys, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	if _, err := ValidateToken(svcTok, sameValue); err == nil {
		t.Error("a service token passed as an admin token, which would authorize the whole admin API")
	}
	if _, err := ValidateServiceToken(svcTok, "signals", MustServiceKeys("another-secret")); err == nil {
		t.Error("a token signed with a different secret validated")
	}
}

func TestFilterGrantsKeepsOnlyTheAudiencesSlugs(t *testing.T) {
	eu := uuid.New()
	all := []Grant{
		channelGrant("lens.view", eu),
		globalGrant("experiments.manage"),
		globalGrant("catalog.manage"), // nothing in Signals checks it
		globalGrant("orders.refund"),
	}
	got := FilterGrants(all, []string{"lens.view", "experiments.view"})

	if len(got) != 2 {
		t.Fatalf("FilterGrants kept %v, want the lens and experiments grants only", got)
	}
	for _, g := range got {
		if g.Slug == "catalog.manage" || g.Slug == "orders.refund" {
			t.Errorf("%s rode along into an analytics token", g.Slug)
		}
	}
	// experiments.manage survives a request for experiments.view, or the implication would break inside the token
	if !NewGrantSet(got).HasAny("experiments.manage") {
		t.Error("the manage grant satisfying the requested view check was dropped")
	}
}

// A superuser bypasses in the guard before any grant is read, so their token stays small rather than carrying all 86
func TestSuperuserTokenCarriesNoGrants(t *testing.T) {
	c := signalsClaims()
	c.IsSuperuser = true
	token, err := GenerateServiceToken(c, "signals", svcKeys, time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	got, err := ValidateServiceToken(token, "signals", svcKeys)
	if err != nil {
		t.Fatalf("ValidateServiceToken: %v", err)
	}
	if !got.IsSuperuser || len(got.Grants) != 0 {
		t.Errorf("superuser=%v grants=%v, want true and none", got.IsSuperuser, got.Grants)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("not a three-part JWT: %q", token)
	}
}

func TestServiceTokenExpires(t *testing.T) {
	token, err := GenerateServiceToken(signalsClaims(globalGrant("lens.view")), "signals", svcKeys, -time.Minute)
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}
	if _, err := ValidateServiceToken(token, "signals", svcKeys); err == nil {
		t.Error("an expired token validated, so revocation by TTL would never take effect")
	}
}

func TestServiceTokenRefusesNoExpiryAndOtherHMACs(t *testing.T) {
	sign := func(m jwt.SigningMethod, exp bool) string {
		c := serviceJWTClaims{Type: "service", Audience: jwt.ClaimStrings{"signals"}, AdminID: uuid.New()}
		if exp {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Minute))
		}
		tok := jwt.NewWithClaims(m, c)
		tok.Header["kid"] = svcKeys.id
		s, err := tok.SignedString([]byte(svcSecret))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if _, err := ValidateServiceToken(sign(jwt.SigningMethodHS256, false), "signals", svcKeys); err == nil {
		t.Error("a service token with no expiry validated, so a leaked one would work forever")
	}
	if _, err := ValidateServiceToken(sign(jwt.SigningMethodHS512, true), "signals", svcKeys); err == nil {
		t.Error("an HS512 service token validated, want only the HS256 core mints")
	}
	if _, err := ValidateServiceToken(sign(jwt.SigningMethodHS256, true), "signals", svcKeys); err != nil {
		t.Errorf("an HS256 token with an expiry = %v, want valid", err)
	}
}

// Rotation: tokens signed with the retiring secret keep verifying while it is listed, so core and the suite can restart one at a time
func TestARotatedServiceSecretKeepsInFlightCallsVerifying(t *testing.T) {
	old := MustServiceKeys("the-old-service-secret-0123456789")
	tok, err := GenerateServiceToken(ServiceClaims{System: true}, "signals", old, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateServiceToken(tok, "signals", MustServiceKeys(svcSecret, "the-old-service-secret-0123456789")); err != nil {
		t.Errorf("a token from the previous service secret stopped verifying: %v", err)
	}
	if _, err := ValidateServiceToken(tok, "signals", svcKeys); err == nil {
		t.Error("a token from a secret no longer listed still verified")
	}
	if _, err := GenerateServiceToken(ServiceClaims{System: true}, "signals", ServiceKeys{}, time.Minute); err == nil {
		t.Error("the zero key set minted a token")
	}
}
