// Package emailtrack is the shared, leaf-level email open and click tracking primitive - an HMAC-signed token referencing an email_sends id, plus pixel and link-rewrite injection
// Campaign and flow sends both use it, so they produce identical tokens the single /e/o and /e/c routes resolve - Depends only on stdlib and uuid, and sits beside internal/ because the suite signs what core verifies
package emailtrack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid tracking token")

// The canonical 1x1 transparent GIF the open pixel returns - a compile-time constant rather than a hosted file, since a CDN would cache it off-origin and the open would never reach us
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}

func GIF() []byte { return transparentGIF }

type Payload struct {
	S uuid.UUID `json:"s"`           // email_sends id
	U string    `json:"u,omitempty"` // click destination (signed; clicks only)
	L int       `json:"l,omitempty"` // link index (clicks only)
}

// Signs and verifies tracking tokens with an HMAC key, the app's unsubKey
type Signer struct{ key []byte }

func NewSigner(key []byte) *Signer { return &Signer{key: key} }

// Produces a URL-safe `payload.mac` token
func (s *Signer) Sign(p Payload) string {
	raw, _ := json.Marshal(p)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Signer) Verify(token string) (Payload, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return Payload{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return Payload{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Payload{}, ErrInvalid
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Payload{}, ErrInvalid
	}
	return p, nil
}

var hrefRe = regexp.MustCompile(`(?i)href="(https?://[^"]+)"`)

// Appends the open pixel and rewrites outbound links through the click route, each getting its own index for per-link reporting - baseURL is the public host the mail client can reach, the storefront, which proxies /e/
// With no base URL it returns the html unchanged rather than emit broken links
func (s *Signer) Inject(html, baseURL string, sendID uuid.UUID) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return html
	}

	linkIndex := 0
	html = hrefRe.ReplaceAllStringFunc(html, func(m string) string {
		url := hrefRe.FindStringSubmatch(m)[1]
		if strings.Contains(url, "/unsubscribe/") || strings.Contains(url, "/e/o/") || strings.Contains(url, "/e/c/") {
			return m
		}
		tok := s.Sign(Payload{S: sendID, U: url, L: linkIndex})
		linkIndex++
		return `href="` + base + "/e/c/" + tok + `"`
	})

	return s.InjectPixel(html, base, sendID)
}

// Appends only the open pixel, leaving links untouched - for security-sensitive transactional mail like verification and password reset, where rewriting the single-use-token link would add a hop on the auth path and let link-prefetchers burn the token
// Those emails get open tracking but keep their original links
func (s *Signer) InjectPixel(html, baseURL string, sendID uuid.UUID) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return html
	}
	pixel := `<img src="` + base + "/e/o/" + s.Sign(Payload{S: sendID}) +
		`" width="1" height="1" alt="" style="display:none" />`
	if i := strings.LastIndex(strings.ToLower(html), "</body>"); i >= 0 {
		return html[:i] + pixel + html[i:]
	}
	return html + pixel
}

// The unsubscribe link is signed the same way, over the customer id alone - the suite mints it into
// every marketing mail and core, where consent lives, verifies it when the link is followed
func (s *Signer) SignUnsubscribe(customerID uuid.UUID) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(customerID.String()))
	return payload + "." + s.mac(payload)
}

// VerifyUnsubscribe answers the customer a valid link names, ErrInvalid for anything else
func (s *Signer) VerifyUnsubscribe(token string) (uuid.UUID, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || !hmac.Equal([]byte(s.mac(parts[0])), []byte(parts[1])) {
		return uuid.Nil, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil, ErrInvalid
	}
	id, err := uuid.Parse(string(raw))
	if err != nil {
		return uuid.Nil, ErrInvalid
	}
	return id, nil
}

func (s *Signer) mac(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
