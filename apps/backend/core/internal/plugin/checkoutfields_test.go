package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestCheckoutFieldAnswers(t *testing.T) {
	ctx, ch := context.Background(), uuid.New()
	field := func(f sdk.CheckoutField) CheckoutField { return CheckoutField{Plugin: "acme", CheckoutField: f} }
	text := field(sdk.CheckoutField{Key: "note", Label: "note", MaxLength: 5})
	tick := field(sdk.CheckoutField{Key: "agree", Label: "the gift terms", Type: sdk.CheckoutFieldCheckbox, Required: true})
	pick := field(sdk.CheckoutField{Key: "wrap", Label: "wrapping", Type: sdk.CheckoutFieldSelect, Options: []sdk.CheckoutFieldOption{{Value: "paper", Label: "Paper"}, {Value: "box", Label: "Box"}}})
	day := field(sdk.CheckoutField{Key: "day", Label: "delivery day", Type: sdk.CheckoutFieldDate})
	for name, tc := range map[string]struct {
		f        CheckoutField
		raw, ans string
		code     string
	}{
		"text trimmed":        {text, "  hi  ", "hi", ""},
		"text too long":       {text, "toolong", "", "CHECKOUT_FIELD_TOO_LONG"},
		"empty optional":      {text, "   ", "", ""},
		"checkbox ticked":     {tick, "on", "true", ""},
		"checkbox left empty": {tick, "", "", "CHECKOUT_FIELD_REQUIRED"},
		"checkbox nonsense":   {tick, "maybe", "", "CHECKOUT_FIELD_INVALID"},
		"an option":           {pick, "box", "box", ""},
		"not an option":       {pick, "bag", "", "CHECKOUT_FIELD_INVALID"},
		"a date":              {day, "2026-12-24", "2026-12-24", ""},
		"not a date":          {day, "24/12/2026", "", "CHECKOUT_FIELD_INVALID"},
		"no default ceiling":  {field(sdk.CheckoutField{Key: "n", Label: "n"}), strings.Repeat("a", 500), strings.Repeat("a", 500), ""},
		"the default ceiling": {field(sdk.CheckoutField{Key: "n", Label: "n"}), strings.Repeat("a", 501), "", "CHECKOUT_FIELD_TOO_LONG"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := tc.f.Answer(ctx, ch, tc.raw)
			if got != tc.ans || apperr.CodeOf(err) != tc.code {
				t.Errorf("Answer(%q) = %q, %v - want %q, code %q", tc.raw, got, err, tc.ans, tc.code)
			}
		})
	}

	called := 0
	vetted := field(sdk.CheckoutField{Key: "msg", Label: "message", Validate: func(_ context.Context, check sdk.CheckoutFieldCheck) error {
		called++
		if check.ChannelID != ch {
			t.Errorf("Validate saw channel %s", check.ChannelID)
		}
		if check.Value == "rude" {
			return sdk.UserErrorWithCode(sdk.ErrorValidation, "ACME_RUDE", "keep the message kind")
		}
		return nil
	}})
	if _, err := vetted.Answer(ctx, ch, ""); err != nil || called != 0 {
		t.Errorf("an empty answer: %v, Validate ran %d times - want it skipped", err, called)
	}
	_, err := vetted.Answer(ctx, ch, " rude ")
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ACME_RUDE" || ae.Class != apperr.ClassValidation || ae.Message != "keep the message kind" {
		t.Errorf("a plugin's refusal reached the shopper as %v", err)
	}
}

// One shopper flooding the details step must not refuse everyone else's answers, so a field's check skips the
// plugin's per-minute allowance
func TestAPluginOverItsAllowanceStillChecksCheckoutAnswers(t *testing.T) {
	origEnabled, origLimit := isEnabledFromContext, checkRateLimitFn
	t.Cleanup(func() { isEnabledFromContext, checkRateLimitFn = origEnabled, origLimit })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return true }
	checkRateLimitFn = func(context.Context, string) error { return ErrPluginRateLimited }
	asked := false
	f := CheckoutField{Plugin: "acme", Key: "acme:msg", CheckoutField: sdk.CheckoutField{Key: "msg", Label: "message",
		Validate: func(context.Context, sdk.CheckoutFieldCheck) error { asked = true; return nil }}}
	if v, err := f.Answer(context.Background(), uuid.New(), "hi"); err != nil || v != "hi" || !asked {
		t.Errorf("answer %q, %v, asked %v", v, err, asked)
	}
}
