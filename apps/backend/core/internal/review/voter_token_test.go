package review

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var voterSecret = []byte("voter-token-secret")

func TestVoterTokenRoundTrips(t *testing.T) {
	id := uuid.New()
	got, err := ParseVoterToken(voterSecret, signVoterToken(voterSecret, id))
	if err != nil {
		t.Fatalf("ParseVoterToken on a freshly signed token: %v", err)
	}
	if got != id {
		t.Errorf("id = %s, want %s", got, id)
	}
}

func TestVoterTokenIsDeterministic(t *testing.T) { // the storefront re-mints it on every vote, so a drifting signature would log the same visitor out of their own votes
	id := uuid.New()
	if a, b := signVoterToken(voterSecret, id), signVoterToken(voterSecret, id); a != b {
		t.Errorf("two signings of the same id differ:\n%s\n%s", a, b)
	}
}

func TestVoterTokenIsCookieSafe(t *testing.T) { // it travels in a cookie, so standard base64 padding or slashes would need escaping and come back mangled
	tok := signVoterToken(voterSecret, uuid.New())
	if strings.ContainsAny(tok, "=+/ ;,\"\\") {
		t.Errorf("token = %q, want no characters a cookie value has to escape", tok)
	}
	_, sig, ok := strings.Cut(tok, ".")
	if !ok || sig == "" {
		t.Fatalf("token = %q, want <uuid>.<signature>", tok)
	}
	if _, err := base64.RawURLEncoding.DecodeString(sig); err != nil {
		t.Errorf("signature %q is not unpadded base64url: %v", sig, err)
	}
}

func TestVoterTokenRejectsAnotherSecret(t *testing.T) { // the signature is the only thing stopping a forged voter id, so a token minted elsewhere must not verify here
	tok := signVoterToken([]byte("some-other-deployment"), uuid.New())
	if _, err := ParseVoterToken(voterSecret, tok); !errors.Is(err, ErrInvalidVoterToken) {
		t.Errorf("err = %v, want ErrInvalidVoterToken", err)
	}
}

func TestVoterTokenRejectsASwappedID(t *testing.T) { // keeping a valid signature and pasting someone else's id is the actual attack - vote as them, or mint unlimited identities
	victim := uuid.New()
	tok := signVoterToken(voterSecret, victim)
	_, sig, _ := strings.Cut(tok, ".")

	forged := uuid.New().String() + "." + sig
	if _, err := ParseVoterToken(voterSecret, forged); !errors.Is(err, ErrInvalidVoterToken) {
		t.Errorf("err = %v on a token whose id was swapped under a valid signature, want ErrInvalidVoterToken", err)
	}
}

func TestVoterTokenRejectsAMangledSignature(t *testing.T) {
	id := uuid.New()
	tok := signVoterToken(voterSecret, id)
	idStr, sig, _ := strings.Cut(tok, ".")

	for name, bad := range map[string]string{
		"truncated":     idStr + "." + sig[:len(sig)-1],
		"one bit off":   idStr + "." + flipLast(sig),
		"padded":        idStr + "." + sig + "=",
		"another id's":  idStr + "." + mustSig(t, uuid.New()),
		"empty":         idStr + ".",
		"not base64":    idStr + ".!!!!",
		"double signed": tok + "." + sig,
	} {
		if _, err := ParseVoterToken(voterSecret, bad); !errors.Is(err, ErrInvalidVoterToken) {
			t.Errorf("%s: err = %v, want ErrInvalidVoterToken", name, err)
		}
	}
}

func TestVoterTokenRejectsMalformedInput(t *testing.T) { // the raw value comes straight off a request header, so every shape has to fail closed rather than panic or resolve to uuid.Nil
	for name, raw := range map[string]string{
		"empty":             "",
		"whitespace":        "   ",
		"no separator":      uuid.NewString(),
		"leading dot":       "." + mustSig(t, uuid.New()),
		"trailing dot":      uuid.NewString() + ".",
		"just a dot":        ".",
		"not a uuid":        "not-a-uuid." + mustSig(t, uuid.New()),
		"empty uuid part":   ".sig",
		"nil uuid":          uuid.Nil.String() + ".",
		"json":              `{"id":"x"}`,
		"path traversal":    "../../etc/passwd",
		"very long garbage": strings.Repeat("a", 4096),
	} {
		id, err := ParseVoterToken(voterSecret, raw)
		if !errors.Is(err, ErrInvalidVoterToken) {
			t.Errorf("%s: err = %v, want ErrInvalidVoterToken", name, err)
		}
		if id != uuid.Nil {
			t.Errorf("%s: id = %s, want uuid.Nil on failure", name, id)
		}
	}
}

func TestVoterTokenTrimsSurroundingWhitespace(t *testing.T) { // header and cookie values pick up stray spaces in transit, and dropping the visitor's identity over one would double-count their vote
	id := uuid.New()
	tok := signVoterToken(voterSecret, id)
	for _, raw := range []string{" " + tok, tok + " ", "\t" + tok + "\n"} {
		got, err := ParseVoterToken(voterSecret, raw)
		if err != nil {
			t.Errorf("ParseVoterToken(%q): %v", raw, err)
			continue
		}
		if got != id {
			t.Errorf("id = %s, want %s", got, id)
		}
	}
}

func TestVoterTokenSignsTheRawBytesNotTheEncoding(t *testing.T) { // the mac covers id[:], so any spelling uuid.Parse accepts verifies - worth pinning so nobody later assumes the canonical string was authenticated
	id := uuid.New()
	_, sig, _ := strings.Cut(signVoterToken(voterSecret, id), ".")

	braced := "{" + id.String() + "}." + sig
	got, err := ParseVoterToken(voterSecret, braced)
	if err != nil {
		t.Fatalf("a braced uuid spelling: %v", err)
	}
	if got != id {
		t.Errorf("id = %s, want %s", got, id)
	}
}

func TestVoterTokenWithAnEmptySecret(t *testing.T) { // an unconfigured secret must still produce a verifiable token rather than an empty signature everyone can forge locally
	id := uuid.New()
	tok := signVoterToken(nil, id)
	if _, sig, _ := strings.Cut(tok, "."); sig == "" {
		t.Fatalf("token = %q, want a signature even with no secret", tok)
	}
	if _, err := ParseVoterToken(nil, tok); err != nil {
		t.Errorf("ParseVoterToken with an empty secret: %v", err)
	}
	if _, err := ParseVoterToken(voterSecret, tok); !errors.Is(err, ErrInvalidVoterToken) {
		t.Error("a token signed with no secret verified under a real one")
	}
}

func mustSig(t *testing.T, id uuid.UUID) string {
	t.Helper()
	_, sig, ok := strings.Cut(signVoterToken(voterSecret, id), ".")
	if !ok {
		t.Fatal("SignVoterToken did not emit a separator")
	}
	return sig
}

func flipLast(sig string) string {
	last := sig[len(sig)-1]
	if last == 'A' {
		return sig[:len(sig)-1] + "B"
	}
	return sig[:len(sig)-1] + "A"
}

// The storefront mints voter tokens - this mirrors its HMAC so the verify tests have something to verify
func signVoterToken(secret []byte, id uuid.UUID) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(id[:])
	return id.String() + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
