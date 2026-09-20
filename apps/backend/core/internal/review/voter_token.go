package review

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// A voter token with a bad or missing HMAC, or a malformed one - without the check an attacker could forge any UUID and spam anonymous votes, so the signature ties the token to a secret shared with the storefront
var ErrInvalidVoterToken = errors.New("review: voter token signature mismatch or malformed")

// Verifies the HMAC against the embedded UUID and returns it, ErrInvalidVoterToken on any failure - constant-time compare through hmac.Equal
func ParseVoterToken(secret []byte, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	idx := strings.IndexByte(raw, '.')
	if idx <= 0 || idx == len(raw)-1 {
		return uuid.Nil, ErrInvalidVoterToken
	}
	idStr := raw[:idx]
	sig := raw[idx+1:]
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, ErrInvalidVoterToken
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(id[:])
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return uuid.Nil, ErrInvalidVoterToken
	}
	return id, nil
}
