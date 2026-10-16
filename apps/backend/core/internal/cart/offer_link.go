package cart

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
)

// OfferLinkHeader carries an offer link's token from the storefront - the storefront keeps it in an httpOnly cookie and forwards it on every call for the offer cart
const OfferLinkHeader = "X-Cart-Offer"

// OfferLinkPath is the storefront route an offer email links to, the token appended
const OfferLinkPath = "/offer/"

var ErrOfferLinkInvalid = errors.New("this offer link is not valid")

// OfferLinks resolves stored opaque offer credentials.
type OfferLinks struct{ repo Repository }

func NewOfferLinks(repo Repository) *OfferLinks { return &OfferLinks{repo: repo} }

func (l *OfferLinks) Verify(ctx context.Context, token string) (OfferGrant, error) {
	if len(token) != 22 || l.repo == nil {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 16 || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	c, err := l.repo.GetOfferByToken(ctx, token)
	if err != nil {
		if IsSQLNoRows(err) {
			return OfferGrant{}, ErrOfferLinkInvalid
		}
		return OfferGrant{}, fmt.Errorf("resolve offer link: %w", err)
	}
	if !c.IsOffer() {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	return OfferGrant{CartID: c.ID, LinkVersion: c.Offer.LinkVersion}, nil
}

// OfferGrantMiddleware is the links' Middleware, read when the request arrives - the links are wired after the HTTP stack is assembled
func (s *Service) OfferGrantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.links == nil {
			next.ServeHTTP(w, r)
			return
		}
		s.links.Middleware(next).ServeHTTP(w, r)
	})
}

// Middleware turns a valid offer token on the request into a grant on its context. A missing or bad token adds nothing rather than failing the request - the cart access check is what refuses
func (l *OfferLinks) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.Header.Get(OfferLinkHeader); tok != "" {
			if g, err := l.Verify(r.Context(), tok); err == nil {
				r = r.WithContext(WithOfferGrant(r.Context(), g))
			}
		}
		next.ServeHTTP(w, r)
	})
}
