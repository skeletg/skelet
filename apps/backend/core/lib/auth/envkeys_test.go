package auth

import (
	"errors"
	"testing"
)

// core reads both halves when compose hands it the public key too - a pair that doesn't match would have the Signals tiers refuse every session core mints
func TestTheCustomerKeyPairMustMatch(t *testing.T) {
	seed, pub, _ := GenerateCustomerKey()
	_, otherPub, _ := GenerateCustomerKey()
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if _, err := CustomerSignerFromEnv(env(map[string]string{EnvCustomerPrivateKey: seed, EnvCustomerPublicKey: pub})); err != nil {
		t.Fatalf("a matching pair was refused: %v", err)
	}
	if _, err := CustomerSignerFromEnv(env(map[string]string{EnvCustomerPrivateKey: seed, EnvCustomerPublicKey: otherPub})); err == nil {
		t.Fatal("a public key from another pair was accepted")
	}
	if err := RefuseRetiredSecret(env(map[string]string{EnvRetiredJWTSecret: "x"})); !errors.Is(err, ErrRetiredJWTSecret) {
		t.Fatalf("RefuseRetiredSecret = %v, want ErrRetiredJWTSecret", err)
	}
	if got := SplitKeyList(" a, ,b ,"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("SplitKeyList = %q", got)
	}
}
