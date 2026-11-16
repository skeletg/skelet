package plugin

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Key is the field's key as carts and orders keep it, "<plugin>:<key>"
type CheckoutField struct {
	Plugin string
	Key    string
	sdk.CheckoutField
}

// CheckoutFieldsFor is the fields of every compiled-in plugin enabled on the channel, in registration order
func CheckoutFieldsFor(ctx context.Context, channelID uuid.UUID) []CheckoutField {
	var out []CheckoutField
	on := map[string]bool{}
	for _, r := range sdk.CheckoutFields() {
		enabled, asked := on[r.PluginSlug]
		if !asked {
			enabled = Enabled(ctx, r.PluginSlug, channelID)
			on[r.PluginSlug] = enabled
		}
		if enabled {
			out = append(out, CheckoutField{Plugin: r.PluginSlug, Key: r.Key(), CheckoutField: r.Field})
		}
	}
	return out
}

func (f CheckoutField) Max() int {
	if f.MaxLength > 0 {
		return f.MaxLength
	}
	return sdk.DefaultCheckoutAnswer
}

// Answer is the answer as the order keeps it - trimmed, held to the field's type, empty when not given - or why it won't do
func (f CheckoutField) Answer(ctx context.Context, channelID uuid.UUID, raw string) (v string, err error) {
	v = strings.TrimSpace(raw)
	invalid := func(why string) error {
		return apperr.New("CHECKOUT_FIELD_INVALID", apperr.ClassValidation, fmt.Sprintf("the %s %s", f.Label, why))
	}
	switch f.Type {
	case sdk.CheckoutFieldCheckbox:
		switch strings.ToLower(v) {
		case "true", "on", "1", "yes":
			v = "true"
		case "", "false", "off", "0", "no":
			v = ""
		default:
			return "", invalid("is either ticked or not")
		}
	case sdk.CheckoutFieldSelect:
		if v != "" && !slices.ContainsFunc(f.Options, func(o sdk.CheckoutFieldOption) bool { return o.Value == v }) {
			return "", invalid("has to be one of its options")
		}
	case sdk.CheckoutFieldDate:
		if _, err := time.Parse(time.DateOnly, v); v != "" && err != nil {
			return "", invalid("has to be a date")
		}
	default:
		if utf8.RuneCountInString(v) > f.Max() {
			return "", apperr.New("CHECKOUT_FIELD_TOO_LONG", apperr.ClassValidation, fmt.Sprintf("the %s is too long - at most %d characters", f.Label, f.Max()))
		}
	}
	if v == "" {
		if f.Required {
			return "", f.Missing()
		}
		return "", nil
	}
	if f.Validate == nil {
		return v, nil
	}
	ctx, finish, gate := Instrument(ctx, f.Plugin, "checkout field "+f.Key, channelID, Unmetered())
	defer finish(&err)
	if gate != nil {
		return "", gate
	}
	if err := f.Validate(ctx, sdk.CheckoutFieldCheck{ChannelID: channelID, Value: v}); err != nil {
		return "", translateUserError(err)
	}
	return v, nil
}

// Missing is a required field left empty, worded for the shopper
func (f CheckoutField) Missing() error {
	msg := "enter the " + f.Label
	if f.Type == sdk.CheckoutFieldCheckbox {
		msg = "tick " + f.Label
	}
	return apperr.New("CHECKOUT_FIELD_REQUIRED", apperr.ClassValidation, msg)
}
