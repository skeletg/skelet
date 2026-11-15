package sso

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

var ErrNoAdmin = apperr.New("SSO_NO_ADMIN", apperr.ClassAuth, "no admin account here uses that address - ask an administrator to add you first")

type AdminDirectory interface {
	GetByID(ctx context.Context, id uuid.UUID) (admin.AdminUser, error)
	IDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	SignInVouched(ctx context.Context, id uuid.UUID) (admin.AdminUser, error)
}

type CustomerDirectory interface {
	GetByID(ctx context.Context, id uuid.UUID) (customer.Customer, error)
	FindForSignIn(ctx context.Context, channelID uuid.UUID, email string) (customer.Customer, error)
	RegisterVouched(ctx context.Context, channelID uuid.UUID, email, firstName, lastName string) (customer.Customer, error)
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
		flow:  &flow{settings: settings, tokens: tokens, client: &http.Client{Timeout: providerClient}, providers: map[string]cachedProvider{}},
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

func origin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLocal(u.Hostname()))) {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func (s *Service) StartAdmin(ctx context.Context, redirectURI string) (Start, error) {
	o, ok := origin(redirectURI)
	if !ok || !containsFold(s.adminOrigins, o) {
		return Start{}, ErrRedirect
	}
	return s.flow.start(ctx, Admins, uuid.Nil, redirectURI)
}

func (s *Service) StartCustomer(ctx context.Context, ch channel.Channel, redirectURI string) (Start, error) {
	o, ok := origin(redirectURI)
	home, homeOK := origin(ch.PublicURL)
	if !ok || !homeOK || !strings.EqualFold(o, home) {
		return Start{}, ErrRedirect
	}
	return s.flow.start(ctx, Customers, ch.ID, redirectURI)
}

// CompleteAdmin signs in the admin the provider's identity names - never creating one
func (s *Service) CompleteAdmin(ctx context.Context, state, code, binding string) (admin.AdminUser, error) {
	id, p, err := s.flow.complete(ctx, Admins, uuid.Nil, state, code, binding)
	if err != nil {
		return admin.AdminUser{}, err
	}
	live := func(id uuid.UUID) bool { _, err := s.admins.GetByID(ctx, id); return err == nil }
	adminID, err := s.linked(ctx, Admins, uuid.Nil, id, live, func() (uuid.UUID, error) {
		if !p.trusted(id) {
			if len(p.Domains) > 0 {
				return uuid.Nil, ErrNoAdmin
			}
			return uuid.Nil, ErrUnverified
		}
		found, err := s.admins.IDByEmail(ctx, id.Email)
		if errors.Is(err, admin.ErrAdminNotFound) {
			return uuid.Nil, ErrNoAdmin
		}
		return found, err
	})
	if err != nil {
		return admin.AdminUser{}, err
	}
	return s.admins.SignInVouched(ctx, adminID)
}

func (s *Service) CompleteCustomer(ctx context.Context, channelID uuid.UUID, state, code, binding string) (customer.Customer, error) {
	id, p, err := s.flow.complete(ctx, Customers, channelID, state, code, binding)
	if err != nil {
		return customer.Customer{}, err
	}
	live := func(id uuid.UUID) bool { _, err := s.customers.GetByID(ctx, id); return err == nil }
	customerID, err := s.linked(ctx, Customers, channelID, id, live, func() (uuid.UUID, error) {
		if !p.trusted(id) {
			return uuid.Nil, ErrUnverified
		}
		c, err := s.customers.FindForSignIn(ctx, channelID, id.Email)
		if errors.Is(err, customer.ErrCustomerNotFound) {
			c, err = s.customers.RegisterVouched(ctx, channelID, id.Email, id.FirstName, id.LastName)
		}
		return c.ID, err
	})
	if err != nil {
		return customer.Customer{}, err
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

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimRight(x, "/"), v) {
			return true
		}
	}
	return false
}
