package sdk

import (
	"context"
	"regexp"
	"slices"
	"sync"

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

type CheckoutField struct {
	Key      string // lowercase, stable - the order keeps the answer under it ("gift_message")
	Label    string
	Hint     string
	Type     CheckoutFieldType // empty is text
	Options  []string          // a select's choices
	Required bool
	// A text or textarea answer's ceiling in characters - 0 is 500
	MaxLength int
	// Optional, after core's own checks - an sdk.UserError's message reaches the shopper
	Validate func(ctx context.Context, channelID uuid.UUID, value string) error
}

type RegisteredCheckoutField struct {
	PluginSlug string
	Field      CheckoutField
}

var (
	checkoutFieldsMu sync.Mutex
	checkoutFields   []RegisteredCheckoutField
	checkoutFieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

// Asked on every channel the plugin is enabled for - panics on a malformed field or a key another plugin already asks
func (pl *Plugin) RegisterCheckoutField(f CheckoutField) {
	if f.Type == "" {
		f.Type = CheckoutFieldText
	}
	switch {
	case !checkoutFieldKey.MatchString(f.Key):
		panic("sdk.RegisterCheckoutField: key " + f.Key + " must be lowercase letters, digits and underscores, starting with a letter")
	case f.Label == "":
		panic("sdk.RegisterCheckoutField: " + f.Key + " needs a label")
	case !slices.Contains([]CheckoutFieldType{CheckoutFieldText, CheckoutFieldTextarea, CheckoutFieldCheckbox, CheckoutFieldSelect, CheckoutFieldDate}, f.Type):
		panic("sdk.RegisterCheckoutField: " + f.Key + " has an unknown type " + string(f.Type))
	case f.Type == CheckoutFieldSelect && len(f.Options) == 0:
		panic("sdk.RegisterCheckoutField: the select " + f.Key + " needs options")
	}
	checkoutFieldsMu.Lock()
	defer checkoutFieldsMu.Unlock()
	if slices.ContainsFunc(checkoutFields, func(r RegisteredCheckoutField) bool { return r.Field.Key == f.Key }) {
		panic("sdk.RegisterCheckoutField: another plugin already asks " + f.Key)
	}
	f.Options = slices.Clone(f.Options)
	checkoutFields = append(checkoutFields, RegisteredCheckoutField{PluginSlug: pl.slug, Field: f})
}

// CheckoutFields is every checkout field the compiled-in plugins registered, in the order they did
func CheckoutFields() []RegisteredCheckoutField {
	checkoutFieldsMu.Lock()
	defer checkoutFieldsMu.Unlock()
	out := slices.Clone(checkoutFields)
	for i := range out {
		out[i].Field.Options = slices.Clone(out[i].Field.Options)
	}
	return out
}
