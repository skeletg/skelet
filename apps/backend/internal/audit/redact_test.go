package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

const placeholder = "[REDACTED]"

// Every admin mutation's variables land in this map, so anything credential-shaped has to be gone before it reaches a row an operator can read back forever
func TestRedactCatchesEverySecretShapedKey(t *testing.T) {
	for _, key := range []string{
		"password", "newPassword", "password_confirmation",
		"secret", "clientSecret", "apiSecret",
		"token", "refreshToken", "access_token",
		"code", "verificationCode", "couponCode",
		"key", "apiKey", "privateKey", "encryption_key",
		"card", "cardNumber", "creditCard",
		"cvv", "cardCvv",
		"nonce", "paymentNonce",
		"PASSWORD", "ApiKey", "Card_Number", // the pattern is case-insensitive, so the usual GraphQL casings all match
	} {
		got := Redact(map[string]any{key: "sk_live_do_not_store_me"})
		if got[key] != placeholder {
			t.Errorf("Redact kept %q as %v, want %q", key, got[key], placeholder)
		}
	}
}

// Over-redacting would gut the trail, so ordinary fields have to survive untouched
func TestRedactLeavesOrdinaryFieldsAlone(t *testing.T) {
	in := map[string]any{
		"email":    "ops@example.invalid",
		"quantity": float64(3),
		"active":   true,
		"sku":      "T-1",
		"note":     nil,
	}
	got := Redact(in)
	for k, want := range in {
		if got[k] != want {
			t.Errorf("Redact changed %q from %v to %v", k, want, got[k])
		}
	}
}

// GraphQL variables nest, and a secret one level down is just as damaging as one at the top
func TestRedactRecursesThroughNestedInput(t *testing.T) {
	got := Redact(map[string]any{
		"input": map[string]any{
			"email": "ops@example.invalid",
			"credentials": map[string]any{
				"apiKey": "sk_live_x",
				"label":  "Stripe live",
			},
		},
	})

	input, ok := got["input"].(map[string]any)
	if !ok {
		t.Fatalf("input = %#v, want a nested map", got["input"])
	}
	if input["email"] != "ops@example.invalid" {
		t.Errorf("nested email = %v, want it kept", input["email"])
	}
	creds, ok := input["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("credentials = %#v, want a nested map", input["credentials"])
	}
	if creds["apiKey"] != placeholder {
		t.Errorf("nested apiKey = %v, want %q", creds["apiKey"], placeholder)
	}
	if creds["label"] != "Stripe live" {
		t.Errorf("nested label = %v, want it kept", creds["label"])
	}
}

// Bulk mutations ship arrays of inputs, and one unredacted element leaks just as badly as a scalar
func TestRedactWalksIntoSlicesOfMaps(t *testing.T) {
	got := Redact(map[string]any{
		"gateways": []any{
			map[string]any{"slug": "stripe", "secret": "sk_live_1"},
			map[string]any{"slug": "braintree", "privateKey": "pk_live_2"},
			"a bare string element",
			float64(7),
		},
	})

	list, ok := got["gateways"].([]any)
	if !ok {
		t.Fatalf("gateways = %#v, want a slice", got["gateways"])
	}
	if len(list) != 4 {
		t.Fatalf("gateways has %d elements, want 4", len(list))
	}
	first := list[0].(map[string]any)
	if first["secret"] != placeholder || first["slug"] != "stripe" {
		t.Errorf("element 0 = %v, want the secret redacted and the slug kept", first)
	}
	second := list[1].(map[string]any)
	if second["privateKey"] != placeholder {
		t.Errorf("element 1 privateKey = %v, want %q", second["privateKey"], placeholder)
	}
	if list[2] != "a bare string element" || list[3] != float64(7) {
		t.Errorf("non-map elements = %v, %v, want them passed through", list[2], list[3])
	}
}

// A secret nested under a key that is itself secret-shaped short-circuits, and the whole subtree has to go rather than being walked into and partially kept
func TestRedactDropsAnEntireSecretShapedSubtree(t *testing.T) {
	got := Redact(map[string]any{
		"apiKey": map[string]any{"value": "sk_live_x", "label": "readable"},
	})
	if got["apiKey"] != placeholder {
		t.Errorf("apiKey = %#v, want the whole subtree replaced by %q", got["apiKey"], placeholder)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "sk_live_x") {
		t.Errorf("serialized detail = %s, which still carries the secret", raw)
	}
}

// The interceptor hands the caller's own map in - mutating it would change the variables the resolver is still holding
func TestRedactDoesNotMutateItsInput(t *testing.T) {
	nested := map[string]any{"token": "tok_live"}
	in := map[string]any{"password": "hunter2", "nested": nested, "keep": "me"}

	got := Redact(in)

	if in["password"] != "hunter2" {
		t.Errorf("input password = %v, want the caller's map untouched", in["password"])
	}
	if nested["token"] != "tok_live" {
		t.Errorf("input nested token = %v, want the caller's map untouched", nested["token"])
	}
	if got["password"] != placeholder || got["keep"] != "me" {
		t.Errorf("output = %v, want the password redacted and keep intact", got)
	}
	if gotNested := got["nested"].(map[string]any); gotNested["token"] != placeholder {
		t.Errorf("output nested token = %v, want %q", gotNested["token"], placeholder)
	}
}

func TestRedactHandlesEmptyAndNilMaps(t *testing.T) {
	for name, in := range map[string]map[string]any{"nil": nil, "empty": {}} {
		got := Redact(in)
		if got == nil {
			t.Errorf("Redact(%s) = nil, want an empty map", name)
			continue
		}
		if len(got) != 0 {
			t.Errorf("Redact(%s) = %v, want it empty", name, got)
		}
	}
}

// The pattern is a substring match, so a key merely containing a secret word is caught too - the conservative direction for something that writes to a permanent trail
func TestRedactPrefersOverReachingToLeaking(t *testing.T) {
	for _, key := range []string{"keyboardLayout", "encodedValue", "barcode", "postcode"} {
		if got := Redact(map[string]any{key: "harmless"}); got[key] != placeholder {
			t.Errorf("Redact(%q) = %v, want the substring match to redact it - update this test if the pattern is tightened", key, got[key])
		}
	}
}
