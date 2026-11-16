package sso

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
)

const (
	stateTTL        = 10 * time.Minute
	totpTTL         = 5 * time.Minute
	discoveryTTL    = time.Hour
	statePrefix     = "sso:state:"
	totpPrefix      = "sso:totp:"
	providerClient  = 10 * time.Second
	maxProviderBody = 1 << 20
)

var (
	ErrOff         = apperr.New("SSO_OFF", apperr.ClassValidation, "signing in through a provider isn't set up here")
	ErrExpired     = apperr.New("SSO_EXPIRED", apperr.ClassAuth, "that sign-in took too long or was already used - start again")
	ErrRefused     = apperr.New("SSO_REFUSED", apperr.ClassAuth, "the provider's sign-in couldn't be confirmed - start again")
	ErrUnverified  = apperr.New("SSO_EMAIL_UNVERIFIED", apperr.ClassAuth, "the provider hasn't confirmed this email address, so it can't sign you in here")
	ErrRedirect    = apperr.New("SSO_REDIRECT", apperr.ClassValidation, "that return address isn't one this store signs in to")
	ErrUnavailable = apperr.New("SSO_UNAVAILABLE", apperr.ClassUnavailable, "signing in through the provider isn't working right now - try again, or use your password")
)

type Start struct {
	AuthorizeURL string
	State        string
	Binding      string // kept by the browser that started, and sent back with the code - a code from someone else's sign-in can't complete without it
}

type Identity struct {
	Issuer, Subject     string
	Email               string
	EmailVerified       bool
	FirstName, LastName string
	MFA                 bool // the provider says it asked for more than one kind of proof
}

type pending struct {
	Audience    Audience  `json:"a"`
	ChannelID   uuid.UUID `json:"c"`
	RedirectURI string    `json:"r"`
	Verifier    string    `json:"v"`
	Nonce       string    `json:"n"`
	BindingHash string    `json:"b"`
	Issuer      string    `json:"i"`
	ClientID    string    `json:"k"`
	LinkFor     uuid.UUID `json:"l,omitzero"` // the signed-in admin connecting this provider account, nil for a sign-in
}

type flow struct {
	settings Settings
	tokens   auth.TokenBackend
	client   *http.Client

	mu        sync.Mutex
	providers map[string]cachedProvider
}

type cachedProvider struct {
	p  *oidc.Provider
	at time.Time
}

func (f *flow) discover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	f.mu.Lock()
	c, ok := f.providers[issuer]
	f.mu.Unlock()
	if ok && time.Since(c.at) < discoveryTTL {
		return c.p, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, f.client), issuer)
	if err == nil {
		err = checkEndpoints(p.Endpoint())
	}
	if err != nil {
		applog.From(ctx).Warn("sso: provider discovery failed", "issuer", issuer, "err", err)
		return nil, ErrUnavailable
	}
	f.mu.Lock()
	f.providers[issuer] = cachedProvider{p: p, at: time.Now()}
	f.mu.Unlock()
	return p, nil
}

// Discovery, keys and tokens come from an address an admin typed, so they get the webhook guard against internal targets and a size cap
func providerHTTP() *http.Client {
	return &http.Client{Timeout: providerClient, Transport: capped{urlguard.SafeTransport(urlguard.AllowPrivateFromEnv())}}
}

type capped struct{ http.RoundTripper }

func (c capped) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := c.RoundTripper.RoundTrip(r)
	if err == nil {
		res.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(res.Body, maxProviderBody), res.Body}
	}
	return res, err
}

// The browser is sent to the authorization address, so it's held to the issuer's rule - a javascript: one would run in the
// dashboard
func checkEndpoints(ep oauth2.Endpoint) error {
	for _, raw := range []string{ep.AuthURL, ep.TokenURL} {
		if err := checkIssuer(raw); err != nil || raw == "" {
			return fmt.Errorf("discovered endpoint %q: %w", raw, cmp.Or(err, errors.New("missing")))
		}
	}
	return nil
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func oauthConfig(p provider, op *oidc.Provider, redirectURI string) oauth2.Config {
	return oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, Endpoint: op.Endpoint(), RedirectURL: redirectURI,
		Scopes: []string{oidc.ScopeOpenID, "email", "profile"}}
}

func (f *flow) start(ctx context.Context, a Audience, channelID uuid.UUID, redirectURI string, linkFor uuid.UUID) (Start, error) {
	p, ok := providerFor(ctx, f.settings, a, channelID)
	if !ok {
		return Start{}, ErrOff
	}
	op, err := f.discover(ctx, p.Issuer)
	if err != nil {
		return Start{}, err
	}
	state, nonce, binding, verifier := random(), random(), random(), oauth2.GenerateVerifier()
	raw, err := json.Marshal(pending{Audience: a, ChannelID: channelID, RedirectURI: redirectURI, Verifier: verifier, Nonce: nonce,
		BindingHash: digest(binding), Issuer: p.Issuer, ClientID: p.ClientID, LinkFor: linkFor})
	if err != nil {
		return Start{}, err
	}
	// A subject of its own, so no index set gathers every sign-in a channel ever started
	if err := f.tokens.Put(ctx, statePrefix+digest(state), auth.Token{Subject: uuid.New(), Binding: string(raw)}, stateTTL); err != nil {
		return Start{}, unavailable(ctx, "keep sign-in state", err)
	}
	cfg := oauthConfig(p, op, redirectURI)
	return Start{AuthorizeURL: cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), State: state, Binding: binding}, nil
}

// linkFor is the admin connecting an account, nil for a sign-in - a state started for one can't finish as the other
func (f *flow) complete(ctx context.Context, a Audience, channelID uuid.UUID, state, code, binding string, linkFor uuid.UUID) (Identity, provider, error) {
	tok, err := f.tokens.Consume(ctx, statePrefix+digest(state))
	if errors.Is(err, auth.ErrTokenNotFound) {
		return Identity{}, provider{}, ErrExpired
	}
	if err != nil {
		return Identity{}, provider{}, unavailable(ctx, "read sign-in state", err)
	}
	var pend pending
	if json.Unmarshal([]byte(tok.Binding), &pend) != nil || pend.Audience != a || pend.ChannelID != channelID || pend.LinkFor != linkFor ||
		subtle.ConstantTimeCompare([]byte(digest(binding)), []byte(pend.BindingHash)) != 1 {
		return Identity{}, provider{}, ErrRefused
	}
	p, ok := providerFor(ctx, f.settings, a, channelID)
	if !ok || p.Issuer != pend.Issuer || p.ClientID != pend.ClientID {
		return Identity{}, provider{}, ErrExpired
	}
	op, err := f.discover(ctx, p.Issuer)
	if err != nil {
		return Identity{}, provider{}, err
	}
	cfg := oauthConfig(p, op, pend.RedirectURI)
	cctx := oidc.ClientContext(ctx, f.client)
	t, err := cfg.Exchange(cctx, code, oauth2.VerifierOption(pend.Verifier))
	if err != nil {
		return Identity{}, provider{}, refused(ctx, "token exchange", err)
	}
	rawID, _ := t.Extra("id_token").(string)
	idt, err := op.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(cctx, rawID)
	if err == nil && subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(pend.Nonce)) != 1 {
		err = errors.New("nonce mismatch")
	}
	if err != nil {
		return Identity{}, provider{}, refused(ctx, "id token", err)
	}
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		DomainOwner   json.RawMessage `json:"xms_edov"` // Microsoft Entra's word that the tenant owns the address's domain
		GivenName     string          `json:"given_name"`
		FamilyName    string          `json:"family_name"`
		Name          string          `json:"name"`
		AMR           []string        `json:"amr"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, provider{}, refused(ctx, "claims", err)
	}
	id := Identity{Issuer: idt.Issuer, Subject: idt.Subject, Email: strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: truthy(claims.EmailVerified) || truthy(claims.DomainOwner), FirstName: claims.GivenName, LastName: claims.FamilyName,
		MFA: multiFactor(claims.AMR)}
	if id.FirstName == "" && id.LastName == "" {
		id.FirstName, id.LastName, _ = strings.Cut(strings.TrimSpace(claims.Name), " ")
	}
	return id, p, nil
}

// A fault on this side goes to the log - whoever is signing in isn't signed in yet, so they see none of it
func unavailable(ctx context.Context, step string, err error) error {
	applog.From(ctx).Error("sso: sign-in failed", "step", step, "err", err)
	return ErrUnavailable
}

// The provider's own words go to the log, never to the person signing in
func refused(ctx context.Context, step string, err error) error {
	applog.From(ctx).Warn("sso: sign-in refused", "step", step, "err", err)
	return ErrRefused
}

// RFC 8176: "mfa", or two different methods - one alone, even a code or a key, is a single factor
func multiFactor(amr []string) bool {
	if slices.Contains(amr, "mfa") {
		return true
	}
	seen := map[string]bool{}
	for _, m := range amr {
		seen[m] = true
	}
	return len(seen) >= 2
}

func truthy(raw json.RawMessage) bool {
	return strings.Trim(string(raw), `"`) == "true"
}

// The provider has to say it checked the address, and a domain list narrows which addresses count
func (p provider) trusted(id Identity) bool {
	if id.Email == "" || !id.EmailVerified {
		return false
	}
	_, domain, _ := strings.Cut(id.Email, "@")
	return len(p.Domains) == 0 || slices.Contains(p.Domains, domain)
}
