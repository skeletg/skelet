package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

const (
	stateTTL       = 10 * time.Minute
	discoveryTTL   = time.Hour
	statePrefix    = "sso:state:"
	providerClient = 10 * time.Second
)

var (
	ErrOff        = apperr.New("SSO_OFF", apperr.ClassValidation, "signing in through a provider isn't set up here")
	ErrExpired    = apperr.New("SSO_EXPIRED", apperr.ClassAuth, "that sign-in took too long or was already used - start again")
	ErrRefused    = apperr.New("SSO_REFUSED", apperr.ClassAuth, "the provider's sign-in couldn't be confirmed - start again")
	ErrUnverified = apperr.New("SSO_EMAIL_UNVERIFIED", apperr.ClassAuth, "the provider hasn't confirmed this email address, so it can't sign you in here")
	ErrRedirect   = apperr.New("SSO_REDIRECT", apperr.ClassValidation, "that return address isn't one this store signs in to")
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
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", issuer, err)
	}
	f.mu.Lock()
	f.providers[issuer] = cachedProvider{p: p, at: time.Now()}
	f.mu.Unlock()
	return p, nil
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

func (f *flow) start(ctx context.Context, a Audience, channelID uuid.UUID, redirectURI string) (Start, error) {
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
		BindingHash: digest(binding), Issuer: p.Issuer, ClientID: p.ClientID})
	if err != nil {
		return Start{}, err
	}
	if err := f.tokens.Put(ctx, statePrefix+state, auth.Token{Subject: channelID, Binding: string(raw)}, stateTTL); err != nil {
		return Start{}, fmt.Errorf("keep sign-in state: %w", err)
	}
	cfg := oauthConfig(p, op, redirectURI)
	return Start{AuthorizeURL: cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), State: state, Binding: binding}, nil
}

func (f *flow) complete(ctx context.Context, a Audience, channelID uuid.UUID, state, code, binding string) (Identity, provider, error) {
	tok, err := f.tokens.Consume(ctx, statePrefix+state)
	if err != nil {
		return Identity{}, provider{}, ErrExpired
	}
	var pend pending
	if json.Unmarshal([]byte(tok.Binding), &pend) != nil || pend.Audience != a || pend.ChannelID != channelID ||
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
	if err != nil || subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(pend.Nonce)) != 1 {
		return Identity{}, provider{}, refused(ctx, "id token", err)
	}
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		GivenName     string          `json:"given_name"`
		FamilyName    string          `json:"family_name"`
		Name          string          `json:"name"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, provider{}, refused(ctx, "claims", err)
	}
	id := Identity{Issuer: idt.Issuer, Subject: idt.Subject, Email: strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: strings.Trim(string(claims.EmailVerified), `"`) == "true", FirstName: claims.GivenName, LastName: claims.FamilyName}
	if id.FirstName == "" && id.LastName == "" {
		id.FirstName, id.LastName, _ = strings.Cut(strings.TrimSpace(claims.Name), " ")
	}
	return id, p, nil
}

// The provider's own words go to the log, never to the person signing in
func refused(ctx context.Context, step string, err error) error {
	applog.From(ctx).Warn("sso: sign-in refused", "step", step, "err", err)
	return ErrRefused
}

// A domain-restricted admin provider vouches for its own domains - anything else has to say it checked the address
func (p provider) trusted(id Identity) bool {
	if id.Email == "" {
		return false
	}
	if len(p.Domains) > 0 {
		_, domain, _ := strings.Cut(id.Email, "@")
		return slices.Contains(p.Domains, domain)
	}
	return id.EmailVerified
}
