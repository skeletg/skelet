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

// Signs tracking and unsubscribe tokens with EMAIL_LINK_SECRET and verifies against it and any retiring key, so a rotation leaves links already in inboxes working
type Signer struct {
	key    []byte
	verify [][]byte
}

func NewSigner(key []byte, previous ...[]byte) *Signer {
	s := &Signer{key: key, verify: [][]byte{key}}
	for _, p := range previous {
		if len(p) > 0 {
			s.verify = append(s.verify, p)
		}
	}
	return s
}

// Produces a URL-safe `payload.mac` token
func (s *Signer) Sign(p Payload) string {
	raw, _ := json.Marshal(p)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + macWith(s.key, payload)
}

// signed reports whether any key in the set produced mac over payload
func (s *Signer) signed(payload, mac string) bool {
	for _, k := range s.verify {
		if hmac.Equal([]byte(macWith(k, payload)), []byte(mac)) {
			return true
		}
	}
	return false
}

func (s *Signer) Verify(token string) (Payload, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || !s.signed(parts[0], parts[1]) {
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
	return payload + "." + macWith(s.key, payload)
}

// VerifyUnsubscribe answers the customer a valid link names, ErrInvalid for anything else
func (s *Signer) VerifyUnsubscribe(token string) (uuid.UUID, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || !s.signed(parts[0], parts[1]) {
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

func macWith(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

const (
	EnvLinkSecret         = "EMAIL_LINK_SECRET"
	EnvLinkSecretPrevious = "EMAIL_LINK_SECRET_PREVIOUS"
)

var ErrNoLinkSecret = errors.New("EMAIL_LINK_SECRET is not set - it signs the unsubscribe, open and click links")

// SignerFromEnv is the one key set core and the Signals tier both build, EMAIL_LINK_SECRET_PREVIOUS a comma-separated list
func SignerFromEnv(getenv func(string) string) (*Signer, error) {
	key := strings.TrimSpace(getenv(EnvLinkSecret))
	if key == "" {
		return nil, ErrNoLinkSecret
	}
	var previous [][]byte
	for p := range strings.SplitSeq(getenv(EnvLinkSecretPrevious), ",") {
		if p = strings.TrimSpace(p); p != "" {
			previous = append(previous, []byte(p))
		}
	}
	return NewSigner([]byte(key), previous...), nil
}
