package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Admin tokens are HMAC-signed with a key only core holds, customer tokens Ed25519-signed so the Signals tier verifies them with a public key and can mint nothing
// Each key carries an id derived from the key itself, stamped into the token header, so a rotation keeps the previous key verifying until its tokens expire

var (
	ErrNoAdminKey    = errors.New("ADMIN_JWT_SECRET is not set")
	ErrNoCustomerKey = errors.New("no customer session key is configured")
	ErrNoServiceKey  = errors.New("SIGNALS_SERVICE_SECRET is not set")
)

// keyID is the first 8 bytes of the key's hash - stable for a key, and naming nothing about it
func keyID(material []byte) string {
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:8])
}

// An HMAC key set - the current secret signs, it and any retiring one verify
type hmacKeys struct {
	id     string
	secret []byte
	verify map[string][]byte
}

func newHMACKeys(current string, previous []string) (hmacKeys, bool) {
	current = strings.TrimSpace(current)
	if current == "" {
		return hmacKeys{}, false
	}
	k := hmacKeys{id: keyID([]byte(current)), secret: []byte(current), verify: map[string][]byte{}}
	k.verify[k.id] = k.secret
	for _, p := range previous {
		if p = strings.TrimSpace(p); p != "" {
			k.verify[keyID([]byte(p))] = []byte(p)
		}
	}
	return k, true
}

func (k hmacKeys) configured() bool { return len(k.secret) > 0 }

// AdminKeys signs admin access tokens with the current secret and verifies against it and any previous one
type AdminKeys struct{ hmacKeys }

// NewAdminKeys takes the current secret and the ones still being retired, empty entries ignored
func NewAdminKeys(current string, previous ...string) (AdminKeys, error) {
	k, ok := newHMACKeys(current, previous)
	if !ok {
		return AdminKeys{}, ErrNoAdminKey
	}
	return AdminKeys{k}, nil
}

// ServiceKeys sign the tokens core and the Signals suite call each other with - the zero value means no suite is configured
type ServiceKeys struct{ hmacKeys }

func NewServiceKeys(current string, previous ...string) (ServiceKeys, error) {
	k, ok := newHMACKeys(current, previous)
	if !ok {
		return ServiceKeys{}, ErrNoServiceKey
	}
	return ServiceKeys{k}, nil
}

// Configured is false for the zero value - a suite that is switched off or unaddressed
func (k ServiceKeys) Configured() bool { return k.configured() }

// CustomerVerifier checks customer access tokens - all the Signals tier ever holds
type CustomerVerifier struct {
	keys map[string]ed25519.PublicKey
}

// NewCustomerVerifier takes base64 Ed25519 public keys, the current one first, empty entries ignored
func NewCustomerVerifier(publicKeys ...string) (CustomerVerifier, error) {
	v := CustomerVerifier{keys: map[string]ed25519.PublicKey{}}
	for _, raw := range publicKeys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return CustomerVerifier{}, fmt.Errorf("customer session public key must be base64 of %d bytes", ed25519.PublicKeySize)
		}
		v.keys[keyID(b)] = ed25519.PublicKey(b)
	}
	if len(v.keys) == 0 {
		return CustomerVerifier{}, ErrNoCustomerKey
	}
	return v, nil
}

// CustomerSigner mints customer access tokens - core alone holds it, and its verifier also accepts the public keys being retired
type CustomerSigner struct {
	id       string
	priv     ed25519.PrivateKey
	verifier CustomerVerifier
}

// NewCustomerSigner takes the base64 Ed25519 seed and the base64 public keys of retiring private keys
func NewCustomerSigner(privateSeed string, previousPublic ...string) (CustomerSigner, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateSeed))
	if err != nil || len(seed) != ed25519.SeedSize {
		return CustomerSigner{}, fmt.Errorf("CUSTOMER_JWT_PRIVATE_KEY must be base64 of a %d-byte Ed25519 seed", ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	v, err := NewCustomerVerifier(append([]string{base64.StdEncoding.EncodeToString(pub)}, previousPublic...)...)
	if err != nil {
		return CustomerSigner{}, err
	}
	return CustomerSigner{id: keyID(pub), priv: priv, verifier: v}, nil
}

func (s CustomerSigner) Verifier() CustomerVerifier { return s.verifier }

// PublicKey is what the Signals tier is configured with, base64
func (s CustomerSigner) PublicKey() string {
	return base64.StdEncoding.EncodeToString(s.priv.Public().(ed25519.PublicKey))
}

// GenerateCustomerKey mints a fresh Ed25519 key, as the base64 seed core keeps and the base64 public key the Signals tier gets
func GenerateCustomerKey() (seed, public string, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub), nil
}

func init() {
	apperr.RegisterSentinel(ErrNoAdminKey, "ADMIN_SESSION_KEY_MISSING", apperr.ClassInternal)
	apperr.RegisterSentinel(ErrNoCustomerKey, "CUSTOMER_SESSION_KEY_MISSING", apperr.ClassInternal)
	apperr.RegisterSentinel(ErrNoServiceKey, "SERVICE_KEY_MISSING", apperr.ClassInternal)
	apperr.RegisterSentinel(ErrRetiredJWTSecret, "JWT_SECRET_RETIRED", apperr.ClassInternal)
}

// MustServiceKeys is NewServiceKeys for a value known to be set - tests and fixed fixtures
func MustServiceKeys(current string, previous ...string) ServiceKeys {
	k, err := NewServiceKeys(current, previous...)
	if err != nil {
		panic(err)
	}
	return k
}
