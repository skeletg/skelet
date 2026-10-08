package content

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	sentinel := errors.New("invalid slug")
	ok := []string{"hello", "hello-world", "a_b.c", "p123", "2024-summer-sale", "...", strings.Repeat("a", MaxSlugLength)}
	for _, s := range ok {
		if err := ValidateSlug(s, sentinel); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{"", "Hello", "white space", "café", "a/b", "UPPER", ".", "..", strings.Repeat("a", MaxSlugLength+1)}
	for _, s := range bad {
		err := ValidateSlug(s, sentinel)
		if err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", s)
			continue
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("ValidateSlug(%q) error does not wrap the caller's sentinel", s)
		}
	}
}

func TestDecodePublishVeto(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantReject bool
		wantReason string
		wantErr    bool
	}{
		{name: "empty body allows", body: "", wantReject: false},
		{name: "reject false allows", body: `{"reject":false}`, wantReject: false},
		{name: "reject true vetoes with reason", body: `{"reject":true,"reason":"legal hold"}`, wantReject: true, wantReason: "legal hold"},
		{name: "malformed json errors", body: `{not json`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejected, reason, err := DecodePublishVeto([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rejected != tc.wantReject || reason != tc.wantReason {
				t.Errorf("= (%v, %q), want (%v, %q)", rejected, reason, tc.wantReject, tc.wantReason)
			}
		})
	}
}
