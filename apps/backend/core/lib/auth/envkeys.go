package auth

import (
	"errors"
	"fmt"
	"strings"
)

// The environment the session keys load from - each *_PREVIOUS is a comma-separated list kept verifying while its tokens run out
const (
	EnvAdminSecret          = "ADMIN_JWT_SECRET"
	EnvAdminSecretPrevious  = "ADMIN_JWT_SECRET_PREVIOUS"
	EnvCustomerPrivateKey   = "CUSTOMER_JWT_PRIVATE_KEY"
	EnvCustomerPublicKey    = "CUSTOMER_JWT_PUBLIC_KEY"
	EnvCustomerPublicKeyOld = "CUSTOMER_JWT_PUBLIC_KEY_PREVIOUS"
	EnvRetiredJWTSecret     = "JWT_SECRET"
	EnvServiceSecret        = "SIGNALS_SERVICE_SECRET"
	EnvServiceSecretOld     = "SIGNALS_SERVICE_SECRET_PREVIOUS"
)

// ErrRetiredJWTSecret refuses a boot that still sets the old shared secret - it signed both session types, and a value left behind would be read as if it still meant something
var ErrRetiredJWTSecret = errors.New("JWT_SECRET is retired - remove it and set ADMIN_JWT_SECRET plus CUSTOMER_JWT_PRIVATE_KEY (core) / CUSTOMER_JWT_PUBLIC_KEY (Signals), see docs/env.md")

func RefuseRetiredSecret(getenv func(string) string) error {
	if strings.TrimSpace(getenv(EnvRetiredJWTSecret)) != "" {
		return ErrRetiredJWTSecret
	}
	return nil
}

func AdminKeysFromEnv(getenv func(string) string) (AdminKeys, error) {
	return NewAdminKeys(getenv(EnvAdminSecret), SplitKeyList(getenv(EnvAdminSecretPrevious))...)
}

// CustomerSignerFromEnv is core's half - a CUSTOMER_JWT_PUBLIC_KEY beside the private key must be its pair, or Signals would refuse every session core mints
func CustomerSignerFromEnv(getenv func(string) string) (CustomerSigner, error) {
	s, err := NewCustomerSigner(getenv(EnvCustomerPrivateKey), SplitKeyList(getenv(EnvCustomerPublicKeyOld))...)
	if err != nil {
		return CustomerSigner{}, err
	}
	if pub := strings.TrimSpace(getenv(EnvCustomerPublicKey)); pub != "" && pub != s.PublicKey() {
		return CustomerSigner{}, fmt.Errorf("%s is not the public half of %s - it should be %s", EnvCustomerPublicKey, EnvCustomerPrivateKey, s.PublicKey())
	}
	return s, nil
}

// CustomerVerifierFromEnv is the Signals tier's half - public keys only
func CustomerVerifierFromEnv(getenv func(string) string) (CustomerVerifier, error) {
	return NewCustomerVerifier(append([]string{getenv(EnvCustomerPublicKey)}, SplitKeyList(getenv(EnvCustomerPublicKeyOld))...)...)
}

// ServiceKeysFromEnv is the contract's key set - unset answers the zero value, which callers read as no suite to talk to
func ServiceKeysFromEnv(getenv func(string) string) ServiceKeys {
	k, _ := NewServiceKeys(getenv(EnvServiceSecret), SplitKeyList(getenv(EnvServiceSecretOld))...)
	return k
}

func SplitKeyList(raw string) []string {
	var out []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
