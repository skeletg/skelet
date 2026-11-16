package sso

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type Audience string

const (
	Admins    Audience = "admin"
	Customers Audience = "customer"
)

func groupFor(a Audience) string {
	if a == Admins {
		return "general/admin_sign_in/"
	}
	return "general/customer_sign_in/"
}

func init() {
	config.RegisterGroup(config.Group{Section: "general", Slug: "admin_sign_in", Label: "Admin sign-in", SortOrder: 9, Scoping: config.ScopingGlobal})
	config.RegisterGroup(config.Group{Section: "general", Slug: "customer_sign_in", Label: "Customer sign-in", SortOrder: 10, Scoping: config.ScopingChannel})
	for _, a := range []struct {
		aud   Audience
		who   string
		label string
	}{
		{Admins, "admins", "Sign in with your company account"},
		{Customers, "shoppers", "Sign in with your account"},
	} {
		g := groupFor(a.aud)
		config.Register(config.Definition{Path: g + "oidc_enabled", Type: config.TypeBool, Default: "false", Label: "Sign in through an OpenID Connect provider", SuperuserOnly: true,
			Description: "Offers " + a.who + " a button that signs them in with an outside provider, as well as their password - Google, Microsoft, Okta or any other that speaks OpenID Connect. Register this store's sign-in page as the redirect address at the provider."})
		config.Register(config.Definition{Path: g + "oidc_issuer", Type: config.TypeString, Label: "Issuer URL", SuperuserOnly: true, MaxLen: 512, Check: checkIssuer, Clears: []string{g + "oidc_client_secret"},
			Description: "The provider's issuer, as its discovery document names it - https://accounts.google.com, say. Changing it clears the client secret, so a secret meant for one provider is never sent to another."})
		config.Register(config.Definition{Path: g + "oidc_client_id", Type: config.TypeString, Label: "Client ID", SuperuserOnly: true, MaxLen: 256})
		config.Register(config.Definition{Path: g + "oidc_client_secret", Type: config.TypeSecret, Label: "Client secret", SuperuserOnly: true,
			Description: "Used only with an issuer set at the same level - a channel with its own issuer needs its own secret."})
		config.Register(config.Definition{Path: g + "oidc_button_label", Type: config.TypeString, Default: a.label, Label: "Button label", MaxLen: 80})
	}
	config.Register(config.Definition{Path: groupFor(Admins) + "oidc_email_domains", Type: config.TypeString, Label: "Email domains", SuperuserOnly: true, MaxLen: 1000,
		Description: "Comma-separated. When set, an admin's provider account has to have an address the provider verified at one of these domains, checked when it's connected and at every sign-in. Each admin connects their own provider account under Preferences, signed in with their password - nobody becomes an admin, or reaches another admin's account, by signing in."})
	config.Register(config.Definition{Path: groupFor(Admins) + "oidc_enforces_mfa", Type: config.TypeBool, Default: "false", Label: "The provider asks for a second factor", SuperuserOnly: true,
		Description: "On, an admin whose sign-in the provider marks as multi-factor (an amr claim naming mfa, or two different methods) skips this store's own two-factor code. Off, or without that mark, admins with two-factor on enter their code after the provider, as after a password."})
}

// https only, except a provider running on this machine
func checkIssuer(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return errors.New("the issuer is a full URL, like https://accounts.google.com")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocal(u.Hostname())) {
		return errors.New("the issuer has to be an https address")
	}
	if tenant, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/"); strings.EqualFold(u.Hostname(), "login.microsoftonline.com") && slices.Contains(sharedTenants, strings.ToLower(tenant)) {
		return errors.New("use your own tenant's issuer, https://login.microsoftonline.com/<tenant id>/v2.0 - a shared one lets in any Microsoft account")
	}
	return nil
}

var sharedTenants = []string{"common", "organizations", "consumers"}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type Settings interface {
	Get(ctx context.Context, path string, scope config.Scope) (config.Entry, error)
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

type provider struct {
	Issuer, ClientID, ClientSecret, Label string
	Domains                               []string
	EnforcesMFA                           bool
}

func providerFor(ctx context.Context, s Settings, a Audience, channelID uuid.UUID) (provider, bool) {
	if s == nil {
		return provider{}, false
	}
	scope := config.DefaultScope
	if a == Customers {
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	}
	g := groupFor(a)
	if on, err := s.GetBool(ctx, g+"oidc_enabled", scope); err != nil || !on {
		return provider{}, false
	}
	read := func(name string) string {
		v, _ := s.GetString(ctx, g+name, scope)
		return strings.TrimSpace(v)
	}
	issuer, err := s.Get(ctx, g+"oidc_issuer", scope)
	if err != nil {
		return provider{}, false
	}
	secret, err := s.Get(ctx, g+"oidc_client_secret", scope)
	if err != nil {
		return provider{}, false
	}
	// A secret inherited from another level belongs to that level's issuer, never to one set here
	if secret.Value != "" && config.FormatScope(secret.Scope) != config.FormatScope(issuer.Scope) {
		applog.From(ctx).Warn("sso: the client secret is set at another level than the issuer, so the provider stays off", "group", g)
		return provider{}, false
	}
	p := provider{Issuer: strings.TrimSpace(issuer.Value), ClientID: read("oidc_client_id"), ClientSecret: strings.TrimSpace(secret.Value), Label: read("oidc_button_label")}
	if a == Admins {
		p.EnforcesMFA, _ = s.GetBool(ctx, g+"oidc_enforces_mfa", scope)
		for d := range strings.SplitSeq(read("oidc_email_domains"), ",") {
			if d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@")); d != "" {
				p.Domains = append(p.Domains, d)
			}
		}
	}
	if p.Issuer == "" || p.ClientID == "" || checkIssuer(p.Issuer) != nil {
		return provider{}, false
	}
	return p, true
}
