package cart

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// OfferLinkHeader carries an offer link's token from the storefront - the storefront keeps it in an httpOnly cookie and forwards it on every call for the offer cart
const OfferLinkHeader = "X-Cart-Offer"

// OfferLinkPath is the storefront route an offer email links to, the token appended
const OfferLinkPath = "/offer/"

var ErrOfferLinkInvalid = errors.New("this offer link is not valid")

// OfferLinks resolves opaque offer credentials. The key only verifies legacy
// signed links already handed out; all newly generated links use the stored token.
type OfferLinks struct {
	key  []byte
	repo Repository
}

// NewOfferLinks derives the offer key from the app secret, separated from every other use of it
func NewOfferLinks(secret []byte, repo Repository) *OfferLinks {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("skelet/cart-offer-link/v1"))
	return &OfferLinks{key: m.Sum(nil), repo: repo}
}

// signLegacy reproduces previously issued links for compatibility tests.
func (l *OfferLinks) signLegacy(cartID uuid.UUID, version int32) string {
	payload := cartID.String() + "." + strconv.Itoa(int(version))
	return payload + "." + l.mac(payload)
}

func (l *OfferLinks) Verify(ctx context.Context, token string) (OfferGrant, error) {
	if strings.Contains(token, ".") {
		return l.verifyLegacy(token)
	}
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

func (l *OfferLinks) verifyLegacy(token string) (OfferGrant, error) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(l.mac(payload))) {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	idPart, verPart, ok := strings.Cut(payload, ".")
	if !ok {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	id, err := uuid.Parse(idPart)
	if err != nil {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	v, err := strconv.ParseInt(verPart, 10, 32)
	if err != nil {
		return OfferGrant{}, ErrOfferLinkInvalid
	}
	return OfferGrant{CartID: id, LinkVersion: int32(v)}, nil
}

func (l *OfferLinks) mac(payload string) string {
	m := hmac.New(sha256.New, l.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
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
