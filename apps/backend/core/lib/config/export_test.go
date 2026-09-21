package config

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

// IsolateRegistries is the registry reset for the external tests in this directory
func IsolateRegistries(t *testing.T) { isolateRegistries(t) }

// CipherForTest is the throwaway key the secret round-trip test encrypts with
func CipherForTest(t *testing.T) *secrets.Cipher { return testCipher(t) }
