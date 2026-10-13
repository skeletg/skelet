package referral

import (
	"context"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestNewCodeShapeAndAlphabet(t *testing.T) { // codes are read aloud and retyped, so the confusable 0/O/1/I/L must never appear
	for range 200 {
		code, err := newCode()
		if err != nil {
			t.Fatalf("newCode: %v", err)
		}
		if len(code) != 8 {
			t.Fatalf("code %q is %d chars, want 8", code, len(code))
		}
		for _, c := range code {
			if !strings.ContainsRune(codeAlphabet, c) {
				t.Fatalf("code %q contains %q, which is outside the confusion-safe alphabet", code, c)
			}
		}
	}
}

func TestNewCodeIsNotConstant(t *testing.T) { // a broken random source repeating one code would make every mint collide and exhaust the 5 retries
	seen := map[string]bool{}
	for range 20 {
		code, err := newCode()
		if err != nil {
			t.Fatalf("newCode: %v", err)
		}
		seen[code] = true
	}
	if len(seen) < 2 {
		t.Errorf("20 mints produced %d distinct codes, want the random source alive", len(seen))
	}
}

func TestAttachIgnoresAnAbsentCode(t *testing.T) { // most registrations carry no code, and the register flow calls this unconditionally - reaching the DB here would panic on the nil queries
	s := &Service{}
	for name, code := range map[string]string{
		"empty":      "",
		"whitespace": "   ",
		"tabs":       "\t\n",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Attach reached the database instead of returning early (%v)", name, r)
				}
			}()
			if err := s.Attach(context.Background(), [16]byte{1}, code); err != nil {
				t.Errorf("%s: err = %v, want a silent no-op", name, err)
			}
		}()
	}
}

func TestReferralConfigIsRegistered(t *testing.T) { // ProgramFor reads these three paths, and a renamed registration would surface as a runtime error on every storefront referral read
	for path, wantDefault := range map[string]string{
		"sales/referrals/enabled":               "false",
		"sales/referrals/referrer_reward_cents": "1000",
		"sales/referrals/referee_reward_cents":  "500",
	} {
		def, ok := config.Find(path)
		if !ok {
			t.Errorf("%s is not registered", path)
			continue
		}
		if def.Default != wantDefault {
			t.Errorf("%s default = %q, want %q", path, def.Default, wantDefault)
		}
		if !def.Public {
			t.Errorf("%s is not Public, so the storefront cannot read the program it renders", path)
		}
	}
	if def, ok := config.Find("sales/referrals/enabled"); ok && def.Default != "false" {
		t.Error("the program must ship disabled - rewards move real store credit")
	}
}

// 256 isn't a multiple of 31, so a plain modulo favoured the first eight symbols - with rejection sampling each symbol is near 1/31
func TestNewCodeIsUnbiased(t *testing.T) {
	counts := map[byte]int{}
	const codes = 20000
	for range codes {
		c, err := newCode()
		if err != nil {
			t.Fatal(err)
		}
		for i := range len(c) {
			counts[c[i]]++
		}
	}
	want := float64(codes*8) / float64(len(codeAlphabet))
	for i := range len(codeAlphabet) {
		if got := float64(counts[codeAlphabet[i]]); got < want*0.94 || got > want*1.06 {
			t.Errorf("%c drawn %.0f times, want about %.0f", codeAlphabet[i], got, want)
		}
	}
}
