package sdk

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// Something a plugin asks the shopper at checkout - the storefront draws it, core keeps the answer on the order with the
// label the shopper saw, and placement waits for a required one
type CheckoutFieldType string

const (
	CheckoutFieldText     CheckoutFieldType = "text"
	CheckoutFieldTextarea CheckoutFieldType = "textarea"
	CheckoutFieldCheckbox CheckoutFieldType = "checkbox" // "true" when ticked, empty otherwise - Required means it must be ticked
	CheckoutFieldSelect   CheckoutFieldType = "select"
	CheckoutFieldDate     CheckoutFieldType = "date" // YYYY-MM-DD
)

const (
	DefaultCheckoutAnswer = 500  // a text or textarea answer's ceiling when the field names none
	MaxCheckoutAnswer     = 5000 // the most characters a field may allow
)

// The value is what the order keeps and stays put; the label is what the shopper reads, and can be reworded
type CheckoutFieldOption struct {
	Value, Label string
}

type CheckoutField struct {
	Key      string // lowercase, stable - the order keeps the answer under pl.Key of it ("<plugin>:gift_message")
	Label    string
	Hint     string
	Type     CheckoutFieldType     // empty is text
	Options  []CheckoutFieldOption // a select's choices
	Required bool
	// A text or textarea answer's ceiling in characters - 0 is DefaultCheckoutAnswer, at most MaxCheckoutAnswer
	MaxLength int
	// Optional, after core's own checks - an sdk.UserError's message reaches the shopper
	Validate func(ctx context.Context, check CheckoutFieldCheck) error
}

// What a field's Validate is asked about
type CheckoutFieldCheck struct {
	ChannelID uuid.UUID
	Value     string
}

type RegisteredCheckoutField struct {
	PluginSlug string
	Field      CheckoutField
}

// Key is the field's key as core keeps it, "<plugin>:<key>"
func (r RegisteredCheckoutField) Key() string { return r.PluginSlug + ":" + r.Field.Key }

var checkoutFields registry[RegisteredCheckoutField]

// Asked on every channel the plugin is enabled for - panics on a malformed field or a key the plugin already asks
func (pl *Plugin) RegisterCheckoutField(f CheckoutField) {
	if f.Type == "" {
		f.Type = CheckoutFieldText
	}
	switch {
	case !hookKey.MatchString(f.Key):
		panic("sdk.RegisterCheckoutField: key " + f.Key + hookKeyRule)
	case f.Label == "":
		panic("sdk.RegisterCheckoutField: " + f.Key + " needs a label")
	case !slices.Contains([]CheckoutFieldType{CheckoutFieldText, CheckoutFieldTextarea, CheckoutFieldCheckbox, CheckoutFieldSelect, CheckoutFieldDate}, f.Type):
		panic("sdk.RegisterCheckoutField: " + f.Key + " has an unknown type " + string(f.Type))
	case f.Type == CheckoutFieldSelect && len(f.Options) == 0:
		panic("sdk.RegisterCheckoutField: the select " + f.Key + " needs options")
	case slices.ContainsFunc(f.Options, func(o CheckoutFieldOption) bool { return o.Value == "" || o.Label == "" }):
		panic("sdk.RegisterCheckoutField: every option of " + f.Key + " needs a value and a label")
	case f.MaxLength < 0 || f.MaxLength > MaxCheckoutAnswer:
		panic("sdk.RegisterCheckoutField: " + f.Key + "'s MaxLength is past what an order keeps")
	}
	f.Options = slices.Clone(f.Options)
	checkoutFields.add(RegisteredCheckoutField{PluginSlug: pl.slug, Field: f},
		func(r RegisteredCheckoutField) bool { return r.PluginSlug == pl.slug && r.Field.Key == f.Key },
		"sdk.RegisterCheckoutField: "+pl.slug+" already asks "+f.Key)
}

// CheckoutFields is every checkout field the compiled-in plugins registered, in the order they did
func CheckoutFields() []RegisteredCheckoutField {
	out := checkoutFields.all()
	for i := range out {
		out[i].Field.Options = slices.Clone(out[i].Field.Options)
	}
	return out
}

// CheckoutAnswer reads the shopper's answer to one of this plugin's own fields
func (pl *Plugin) CheckoutAnswer(answers CheckoutFieldAnswers, key string) (string, bool) {
	return answers.CheckoutFieldValue(pl.Key(key))
}

// SetOrderMetadata records a value in the placing order's metadata under pl.Key(key), so no two plugins overwrite each
// other's keys
func (pl *Plugin) SetOrderMetadata(meta OrderPlacingMetadata, key, value string) error {
	return meta.SetPlacingMetadata(pl.Key(key), value)
}
