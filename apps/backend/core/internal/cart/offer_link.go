package cart

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
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

// OfferLinks signs and verifies offer link tokens. A token is "<cart id>.<link version>.<mac>" - signed rather than stored, so the console can show the link again at any time, and bumping the cart's link version revokes every link signed before. The expiry is read from the cart when the link is used, so extending an offer needs no new link
type OfferLinks struct{ key []byte }

// NewOfferLinks derives the offer key from the app secret, separated from every other use of it
func NewOfferLinks(secret []byte) *OfferLinks {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("skelet/cart-offer-link/v1"))
	return &OfferLinks{key: m.Sum(nil)}
}

func (l *OfferLinks) Sign(cartID uuid.UUID, version int32) string {
	payload := cartID.String() + "." + strconv.Itoa(int(version))
	return payload + "." + l.mac(payload)
}

func (l *OfferLinks) Verify(token string) (OfferGrant, error) {
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
			if g, err := l.Verify(tok); err == nil {
				r = r.WithContext(WithOfferGrant(r.Context(), g))
			}
		}
		next.ServeHTTP(w, r)
	})
}
