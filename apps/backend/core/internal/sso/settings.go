package sso

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"

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
		config.Register(config.Definition{Path: g + "oidc_enabled", Type: config.TypeBool, Default: "false", Label: "Sign in through an OpenID Connect provider",
			Description: "Offers " + a.who + " a button that signs them in with an outside provider, as well as their password - Google, Microsoft, Okta or any other that speaks OpenID Connect. Register this store's sign-in page as the redirect address at the provider."})
		config.Register(config.Definition{Path: g + "oidc_issuer", Type: config.TypeString, Label: "Issuer URL",
			Description: "The provider's issuer, as its discovery document names it - https://accounts.google.com, say.", Check: checkIssuer})
		config.Register(config.Definition{Path: g + "oidc_client_id", Type: config.TypeString, Label: "Client ID"})
		config.Register(config.Definition{Path: g + "oidc_client_secret", Type: config.TypeSecret, Label: "Client secret"})
		config.Register(config.Definition{Path: g + "oidc_button_label", Type: config.TypeString, Default: a.label, Label: "Button label"})
	}
	config.Register(config.Definition{Path: groupFor(Admins) + "oidc_email_domains", Type: config.TypeString, Label: "Email domains",
		Description: "Comma-separated. When set, only admins with an address at one of these domains can sign in this way, and the provider is trusted for them. Empty takes any address the provider says it verified. Nobody becomes an admin by signing in - the account has to exist."})
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
	return nil
}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type Settings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

type provider struct {
	Issuer, ClientID, ClientSecret, Label string
	Domains                               []string
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
	p := provider{Issuer: read("oidc_issuer"), ClientID: read("oidc_client_id"), ClientSecret: read("oidc_client_secret"), Label: read("oidc_button_label")}
	if a == Admins {
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
