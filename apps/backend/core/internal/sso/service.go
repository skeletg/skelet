package sso

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

var (
	ErrNoAdmin    = apperr.New("SSO_NO_ADMIN", apperr.ClassAuth, "this provider account can't sign in to an admin here - its address isn't at one of the allowed domains")
	ErrNotLinked  = apperr.New("SSO_NOT_LINKED", apperr.ClassAuth, "this provider account isn't connected to an admin here - sign in with your password and connect it under Preferences")
	ErrLinkTaken  = apperr.New("SSO_LINK_TAKEN", apperr.ClassConflict, "that provider account is already connected to another admin")
	ErrLinkAbsent = apperr.New("SSO_LINK_NOT_FOUND", apperr.ClassNotFound, "no such connected account")
)

// Where a provider sends the browser back - one fixed page each, so a code can't be steered to some other page on the origin
const (
	adminReturnPath    = "/login/sso"
	customerReturnPath = "/account/sso"
)

type AdminDirectory interface {
	GetByID(ctx context.Context, id uuid.UUID) (admin.AdminUser, error)
	SignInVouched(ctx context.Context, id uuid.UUID, totpCode string, providerMFA bool) (admin.AdminUser, error)
}

type CustomerDirectory interface {
	GetByID(ctx context.Context, id uuid.UUID) (customer.Customer, error)
	FindForSignIn(ctx context.Context, channelID uuid.UUID, email string) (customer.Customer, error)
	RegisterVouched(ctx context.Context, channelID uuid.UUID, email, firstName, lastName string) (customer.Customer, error)
	AttemptVouched(ctx context.Context, channelID uuid.UUID, email string) error
	ClaimVouched(ctx context.Context, c customer.Customer) (customer.Customer, error)
	SignInVouched(ctx context.Context, c customer.Customer) error
}

type Service struct {
	flow         *flow
	links        *db.Queries
	admins       AdminDirectory
	customers    CustomerDirectory
	adminOrigins []string
}

func NewService(settings Settings, tokens auth.TokenBackend, links *db.Queries, admins AdminDirectory, customers CustomerDirectory, adminOrigins []string) *Service {
	return &Service{
		flow:  &flow{settings: settings, tokens: tokens, client: providerHTTP(), providers: map[string]cachedProvider{}},
		links: links, admins: admins, customers: customers, adminOrigins: adminOrigins,
	}
}

func (s *Service) Label(ctx context.Context, a Audience, channelID uuid.UUID) string {
	p, ok := providerFor(ctx, s.flow.settings, a, channelID)
	if !ok {
		return ""
	}
	return p.Label
}

// origin is the address's scheme and host, and whether it's one a provider may send a browser back to: https, or http on
// this machine, at the page named by path - the dashboard may sit under a base path, so it's matched as a suffix
func origin(raw, path string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLocal(u.Hostname()))) {
		return "", false
	}
	if path != "" && (!strings.HasSuffix(u.Path, path) || u.RawQuery != "" || u.Fragment != "") {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func (s *Service) adminRedirect(redirectURI string) bool {
	o, ok := origin(redirectURI, adminReturnPath)
	return ok && containsFold(s.adminOrigins, o)
}

func (s *Service) StartAdmin(ctx context.Context, redirectURI string) (Start, error) {
	if !s.adminRedirect(redirectURI) {
		return Start{}, ErrRedirect
	}
	return s.flow.start(ctx, Admins, uuid.Nil, redirectURI, uuid.Nil)
}

// StartAdminLink begins connecting a provider account to the signed-in admin - the one way an admin's provider account
// comes to sign them in
func (s *Service) StartAdminLink(ctx context.Context, adminID uuid.UUID, redirectURI string) (Start, error) {
	if !s.adminRedirect(redirectURI) {
		return Start{}, ErrRedirect
	}
	return s.flow.start(ctx, Admins, uuid.Nil, redirectURI, adminID)
}

func (s *Service) StartCustomer(ctx context.Context, ch channel.Channel, redirectURI string) (Start, error) {
	o, ok := origin(redirectURI, customerReturnPath)
	home, homeOK := origin(ch.PublicURL, "")
	if !ok || !homeOK || !strings.EqualFold(o, home) {
		return Start{}, ErrRedirect
	}
	return s.flow.start(ctx, Customers, ch.ID, redirectURI, uuid.Nil)
}

// AdminSignIn is the signed-in admin, or the ticket that finishes signing them in once they give their own second factor
type AdminSignIn struct {
	User       admin.AdminUser
	TotpTicket string
}

// CompleteAdmin signs in the admin who connected this provider account - never one matched by email, and never creating one
func (s *Service) CompleteAdmin(ctx context.Context, state, code, binding string) (AdminSignIn, error) {
	id, p, err := s.flow.complete(ctx, Admins, uuid.Nil, state, code, binding, uuid.Nil)
	if err != nil {
		return AdminSignIn{}, err
	}
	link, err := s.links.GetIdentityLink(ctx, db.GetIdentityLinkParams{Issuer: id.Issuer, Subject: id.Subject, Kind: string(Admins)})
	if errors.Is(err, sql.ErrNoRows) {
		return AdminSignIn{}, ErrNotLinked
	}
	if err != nil {
		return AdminSignIn{}, unavailable(ctx, "read identity link", err)
	}
	if _, err := s.admins.GetByID(ctx, link.UserID); err != nil {
		return AdminSignIn{}, ErrNotLinked
	}
	// The domain list holds at every sign-in, so narrowing it shuts out accounts connected before
	if len(p.Domains) > 0 && !p.trusted(id) {
		return AdminSignIn{}, ErrNoAdmin
	}
	_ = s.links.TouchIdentityLink(ctx, db.TouchIdentityLinkParams{ID: link.ID, Email: id.Email})
	u, err := s.admins.SignInVouched(ctx, link.UserID, "", p.EnforcesMFA && id.MFA)
	if !errors.Is(err, admin.ErrTotpRequired) {
		return AdminSignIn{User: u}, err
	}
	ticket := random()
	if err := s.flow.tokens.Put(ctx, totpPrefix+digest(ticket), auth.Token{Subject: link.UserID}, totpTTL); err != nil {
		return AdminSignIn{}, unavailable(ctx, "keep sign-in ticket", err)
	}
	return AdminSignIn{TotpTicket: ticket}, nil
}

// AdminLink is a provider account connected to an admin
type AdminLink struct {
	ID         uuid.UUID
	Issuer     string
	Email      string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// CompleteAdminLink connects the provider account to the admin who started connecting it, in place of any they had at
// that provider - refused for one another admin already connected
func (s *Service) CompleteAdminLink(ctx context.Context, adminID uuid.UUID, state, code, binding string) (AdminLink, error) {
	id, p, err := s.flow.complete(ctx, Admins, uuid.Nil, state, code, binding, adminID)
	if err != nil {
		return AdminLink{}, err
	}
	if len(p.Domains) > 0 && !p.trusted(id) {
		return AdminLink{}, ErrNoAdmin
	}
	existing, err := s.links.GetIdentityLink(ctx, db.GetIdentityLinkParams{Issuer: id.Issuer, Subject: id.Subject, Kind: string(Admins)})
	switch {
	case err == nil && existing.UserID != adminID:
		return AdminLink{}, ErrLinkTaken
	case err == nil:
		return toAdminLink(existing), nil
	case !errors.Is(err, sql.ErrNoRows):
		return AdminLink{}, unavailable(ctx, "read identity link", err)
	}
	if err := s.links.DeleteIdentityLinksForUserIssuer(ctx, db.DeleteIdentityLinksForUserIssuerParams{Kind: string(Admins), UserID: adminID, Issuer: id.Issuer}); err != nil {
		return AdminLink{}, unavailable(ctx, "replace identity link", err)
	}
	row, err := s.links.CreateIdentityLink(ctx, db.CreateIdentityLinkParams{Issuer: id.Issuer, Subject: id.Subject, Kind: string(Admins), UserID: adminID, Email: id.Email})
	if errors.Is(err, sql.ErrNoRows) {
		return AdminLink{}, ErrLinkTaken
	}
	if err != nil {
		return AdminLink{}, unavailable(ctx, "connect identity", err)
	}
	return toAdminLink(row), nil
}

func (s *Service) AdminLinks(ctx context.Context, adminID uuid.UUID) ([]AdminLink, error) {
	rows, err := s.links.ListIdentityLinksForUser(ctx, db.ListIdentityLinksForUserParams{Kind: string(Admins), UserID: adminID})
	if err != nil {
		return nil, err
	}
	out := make([]AdminLink, len(rows))
	for i, r := range rows {
		out[i] = toAdminLink(r)
	}
	return out, nil
}

func (s *Service) UnlinkAdmin(ctx context.Context, adminID, linkID uuid.UUID) error {
	n, err := s.links.DeleteIdentityLinkForUser(ctx, db.DeleteIdentityLinkForUserParams{ID: linkID, Kind: string(Admins), UserID: adminID})
	if err == nil && n == 0 {
		return ErrLinkAbsent
	}
	return err
}

func toAdminLink(r db.IdentityLink) AdminLink {
	return AdminLink{ID: r.ID, Issuer: r.Issuer, Email: r.Email, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt}
}

// CompleteAdminTotp finishes a provider sign-in with the admin's own code. A wrong code leaves the ticket for another
// try, the account's two-factor lockout bounding how many
func (s *Service) CompleteAdminTotp(ctx context.Context, ticket, code string) (admin.AdminUser, error) {
	key := totpPrefix + digest(ticket)
	tok, err := s.flow.tokens.Get(ctx, key)
	if errors.Is(err, auth.ErrTokenNotFound) {
		return admin.AdminUser{}, ErrExpired
	}
	if err != nil {
		return admin.AdminUser{}, unavailable(ctx, "read sign-in ticket", err)
	}
	u, err := s.admins.SignInVouched(ctx, tok.Subject, code, false)
	if err == nil || errors.Is(err, admin.ErrTotpLocked) {
		_ = s.flow.tokens.Delete(ctx, key)
	}
	return u, err
}

func (s *Service) CompleteCustomer(ctx context.Context, channelID uuid.UUID, state, code, binding string) (customer.Customer, error) {
	id, p, err := s.flow.complete(ctx, Customers, channelID, state, code, binding, uuid.Nil)
	if err != nil {
		return customer.Customer{}, err
	}
	// The same veto a password sign-in faces, before any account is opened or claimed
	if err := s.customers.AttemptVouched(ctx, channelID, id.Email); err != nil {
		return customer.Customer{}, err
	}
	live := func(id uuid.UUID) bool { _, err := s.customers.GetByID(ctx, id); return err == nil }
	customerID, err := s.linked(ctx, Customers, channelID, id, live, func() (uuid.UUID, error) {
		if !p.trusted(id) {
			return uuid.Nil, ErrUnverified
		}
		c, err := s.customers.FindForSignIn(ctx, channelID, id.Email)
		switch {
		case errors.Is(err, customer.ErrCustomerNotFound):
			c, err = s.customers.RegisterVouched(ctx, channelID, id.Email, id.FirstName, id.LastName)
		case err == nil && !c.IsActive:
			return uuid.Nil, customer.ErrAccountDisabled
		case err == nil:
			c, err = s.customers.ClaimVouched(ctx, c)
		}
		return c.ID, err
	})
	if err != nil {
		return customer.Customer{}, s.opaque(ctx, err)
	}
	c, err := s.customers.GetByID(ctx, customerID)
	if err != nil {
		return customer.Customer{}, err
	}
	if c.ChannelID != channelID {
		return customer.Customer{}, ErrRefused
	}
	return c, s.customers.SignInVouched(ctx, c)
}

// linked is the account an identity was linked to before, else the one match finds, linked from now on - so a changed
// email at the provider keeps reaching the same account. A link whose account is gone is dropped and matched afresh
func (s *Service) linked(ctx context.Context, a Audience, channelID uuid.UUID, id Identity, live func(uuid.UUID) bool, match func() (uuid.UUID, error)) (uuid.UUID, error) {
	ch := uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}
	link, err := s.links.GetIdentityLink(ctx, db.GetIdentityLinkParams{Issuer: id.Issuer, Subject: id.Subject, Kind: string(a), ChannelID: ch})
	switch {
	case err == nil && live(link.UserID):
		_ = s.links.TouchIdentityLink(ctx, db.TouchIdentityLinkParams{ID: link.ID, Email: id.Email})
		return link.UserID, nil
	case err == nil:
		if err := s.links.DeleteIdentityLink(ctx, link.ID); err != nil {
			return uuid.Nil, err
		}
	case !errors.Is(err, sql.ErrNoRows):
		return uuid.Nil, err
	}
	userID, err := match()
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := s.links.CreateIdentityLink(ctx, db.CreateIdentityLinkParams{Issuer: id.Issuer, Subject: id.Subject, Kind: string(a),
		ChannelID: ch, UserID: userID, Email: id.Email}); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, err
	}
	return userID, nil
}

// Forget drops every outside identity linked to the account, the email it carries with it - a removed admin
func (s *Service) Forget(ctx context.Context, a Audience, userID uuid.UUID) error {
	return s.links.DeleteIdentityLinksForUser(ctx, db.DeleteIdentityLinksForUserParams{Kind: string(a), UserID: userID})
}

func (s *Service) SubscribeToCustomerEvents(bus *events.Bus) {
	forget := func(ctx context.Context, e events.Event) error {
		switch ev := e.(type) {
		case *customer.CustomerDeleted:
			return s.Forget(ctx, Customers, ev.CustomerID)
		case *customer.CustomerErased:
			return s.Forget(ctx, Customers, ev.CustomerID)
		}
		return nil
	}
	bus.Subscribe(customer.EventDeleted, forget)
	bus.Subscribe(customer.EventErased, forget)
}

// Errors the caller can act on pass through - anything else is this side's fault, logged rather than shown to someone not
// signed in yet
func (s *Service) opaque(ctx context.Context, err error) error {
	switch apperr.ClassOf(err) {
	case apperr.ClassUnknown, apperr.ClassInternal:
		return unavailable(ctx, "match account", err)
	}
	return err
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimRight(x, "/"), v) {
			return true
		}
	}
	return false
}
