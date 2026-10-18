package destination

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
)

func TestCredentialsBelongInTheSecretNotTheURL(t *testing.T) {
	s := &Service{guard: openGuard{}}
	for name, tc := range map[string]struct {
		in       UpsertInput
		creating bool
		want     string
	}{
		"ga4 key in url":     {UpsertInput{Kind: KindGA4, TargetURL: "https://ga.example/mp/collect?measurement_id=G-1&api_secret=x", Secret: "x"}, true, "api_secret"},
		"meta token in url":  {UpsertInput{Kind: KindMetaCAPI, TargetURL: "https://graph.example/v1/1/events?access_token=x", Secret: "x"}, true, "access_token"},
		"ga4 without secret": {UpsertInput{Kind: KindGA4, TargetURL: "https://ga.example/mp/collect?measurement_id=G-1"}, true, "needs its api_secret"},
		"ga4 update keeps":   {UpsertInput{Kind: KindGA4, TargetURL: "https://ga.example/mp/collect?measurement_id=G-1"}, false, ""},
		"webhook needs none": {UpsertInput{Kind: KindWebhook, TargetURL: "https://hooks.example/x"}, true, ""},
		"unknown kind":       {UpsertInput{Kind: "ftp", TargetURL: "https://hooks.example/x"}, true, "unknown kind"},
		"blank name":         {UpsertInput{Name: " ", Kind: KindWebhook, TargetURL: "https://hooks.example/x"}, true, "name is required"},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.in.Name == "" {
				tc.in.Name = "d"
			}
			tc.in.ChannelID = uuid.New()
			err := s.validate(tc.in, tc.creating)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want ErrInvalid naming %q", err, tc.want)
			}
		})
	}
}

func TestTruncateKeepsValidUTF8(t *testing.T) {
	s := strings.Repeat("a", lastErrorMax-1) + "ž" + "tail"
	got := truncate(s, lastErrorMax)
	if !utf8.ValidString(got) || len(got) > lastErrorMax {
		t.Fatalf("truncate split a rune: %q (len %d)", got[len(got)-3:], len(got))
	}
	if got := truncate("short", lastErrorMax); got != "short" {
		t.Errorf("short input changed: %q", got)
	}
}
