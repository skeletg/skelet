package delivery

import (
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Status is where a requirement stands
type Status string

const (
	StatusSatisfied Status = "satisfied"
	StatusMissing   Status = "missing"
	StatusInvalid   Status = "invalid"
	StatusOptional  Status = "optional" // asked, not required, and not given
)

// The scope of a requirement the whole order owes - a group's requirements carry its key
const ScopeOrder = "order"

// Requirement is one block checkout asks for, with what the cart holds against it
type Requirement struct {
	Scope   string
	Type    string
	Label   string
	Mode    Mode
	Status  Status
	Code    string // the error code placement refuses with while it's missing
	Message string
}

func (r Requirement) Blocking() bool { return r.Status == StatusMissing || r.Status == StatusInvalid }

// Snapshot is what the cart holds, gathered by the caller so requirements stay a pure function of it
type Snapshot struct {
	Plan              Plan
	ChannelPhoneMode  Mode // sales/checkout/phone_field
	Email             bool
	Phone             bool
	ShippingAddress   bool // the shopper's own ship-to address
	Billing           bool // a billing address placement can use, mirrored from shipping or given
	BillingFloor      Mode // the least the store asks of billing whatever the options say - someone else collecting a pickup names no payer
	BillingMirrorable bool // billing may be "same as shipping" - some group ships to the shopper
	Payment           bool
	Groups            []GroupSnapshot
}

// GroupSnapshot is one group's selection
type GroupSnapshot struct {
	Group        Group
	Option       *Option // nil until chosen, or when the chosen one is no longer offered
	Kind         *Kind
	MethodChosen bool // a carrier method is chosen and still quoted for the group
	Choices      int  // how many option × method choices the group has
	Point        bool
	Collector    bool
}

var (
	codeNoDelivery = "CHECKOUT_DELIVERY_UNAVAILABLE"
	codeOption     = "CHECKOUT_SHIPPING_METHOD_REQUIRED"
	codePoint      = "CHECKOUT_DELIVERY_POINT_REQUIRED"
	codeShipping   = "CHECKOUT_SHIPPING_ADDRESS_REQUIRED"
	codeBilling    = "CHECKOUT_BILLING_ADDRESS_REQUIRED"
	codeEmail      = "CHECKOUT_EMAIL_REQUIRED"
	codePhone      = "CHECKOUT_PHONE_REQUIRED"
	codeCollector  = "CHECKOUT_PICKUP_CONTACT_REQUIRED"
	codePayment    = "CHECKOUT_PAYMENT_REQUIRED"
)

// OrderModes are the order-wide blocks' modes - the strictest the channel, the digital lines and every chosen option ask - and whether some chosen delivery ships to the shopper's own address. Billing starts at the store's floor and rises with the options, so a pickup-only order asks it only when its option or the floor does
func OrderModes(s Snapshot) (phone, billing Mode, shipToShopper bool) {
	phone, billing = s.ChannelPhoneMode, s.BillingFloor
	if !phone.Valid() {
		phone = ModeHidden
	}
	if !billing.Valid() {
		billing = ModeHidden
	}
	if len(s.Plan.Digital) > 0 {
		billing = Stricter(billing, DigitalKind.ModeOf(nil, BlockBillingAddress))
		phone = Stricter(phone, DigitalKind.ModeOf(nil, BlockPhone))
	}
	for _, g := range s.Groups {
		if g.Option == nil || g.Kind == nil {
			// Nothing quotes the group yet - all it could take is delivery to the shopper's address, so keep asking for one
			if g.Choices == 0 {
				shipToShopper = true
			}
			continue
		}
		phone = Stricter(phone, g.Kind.ModeOf(g.Option.BlockModes, BlockPhone))
		billing = Stricter(billing, g.Kind.ModeOf(g.Option.BlockModes, BlockBillingAddress))
		if g.Kind.ModeOf(g.Option.BlockModes, BlockShippingAddress) != ModeHidden {
			shipToShopper = true
		}
	}
	return phone, billing, shipToShopper
}

// Requirements lists everything the cart owes, delivery groups first in the order the storefront asks them, then the order-wide blocks. Hidden blocks are left out; optional ones are listed so the storefront knows to ask
func Requirements(s Snapshot) []Requirement {
	var out []Requirement
	add := func(r Requirement) { out = append(out, r) }
	status := func(mode Mode, given bool) Status {
		switch {
		case given:
			return StatusSatisfied
		case mode == ModeRequired:
			return StatusMissing
		}
		return StatusOptional
	}

	phone, billing, shipToShopper := OrderModes(s)

	for _, l := range s.Plan.Undeliverable {
		add(Requirement{Scope: ScopeOrder, Type: BlockDeliveryOption, Label: l.Name, Mode: ModeRequired, Status: StatusInvalid, Code: codeNoDelivery,
			Message: l.Name + " can't be delivered with the options this store offers"})
	}

	for _, g := range s.Groups {
		scope := g.Group.Key
		if g.Option == nil || g.Kind == nil || !g.MethodChosen {
			// Nothing quotes the group - placement would have nothing to price it with, so it can't place until something does
			if g.Choices == 0 && g.Option == nil {
				add(Requirement{Scope: scope, Type: BlockDeliveryOption, Label: "Delivery", Mode: ModeRequired, Status: StatusMissing, Code: codeNoDelivery,
					Message: "These items can't be delivered here with the options this store offers"})
				continue
			}
			add(Requirement{Scope: scope, Type: BlockDeliveryOption, Label: "Delivery", Mode: ModeRequired, Status: StatusMissing, Code: codeOption, Message: "Choose a delivery or pickup option"})
			continue
		}
		add(Requirement{Scope: scope, Type: BlockDeliveryOption, Label: g.Option.Name, Mode: ModeRequired, Status: StatusSatisfied})
		k := *g.Kind
		for _, rb := range k.Resolve(g.Option.BlockModes) {
			if rb.Mode == ModeHidden {
				continue
			}
			t := rb.Spec.Type
			switch t {
			case BlockPoint:
				label := k.PointLabel
				if label == "" {
					label = rb.Spec.Label
				}
				add(Requirement{Scope: scope, Type: t, Label: label, Mode: rb.Mode, Status: status(rb.Mode, g.Point), Code: codePoint, Message: "Choose a " + lower(label)})
			case BlockCollector:
				add(Requirement{Scope: scope, Type: t, Label: rb.Spec.Label, Mode: rb.Mode, Status: status(rb.Mode, g.Collector), Code: codeCollector, Message: "Enter who will collect the order"})
			case BlockShippingAddress, BlockBillingAddress, BlockPhone:
				// order-wide - OrderModes folded them in
			}
		}
	}

	add(Requirement{Scope: ScopeOrder, Type: BlockEmail, Label: "Email", Mode: ModeRequired, Status: status(ModeRequired, s.Email), Code: codeEmail, Message: "Enter an email address"})
	if phone != ModeHidden {
		add(Requirement{Scope: ScopeOrder, Type: BlockPhone, Label: "Phone", Mode: phone, Status: status(phone, s.Phone), Code: codePhone, Message: "Enter a phone number"})
	}
	if shipToShopper {
		add(Requirement{Scope: ScopeOrder, Type: BlockShippingAddress, Label: "Shipping address", Mode: ModeRequired, Status: status(ModeRequired, s.ShippingAddress), Code: codeShipping, Message: "Enter a shipping address"})
	}
	if billing != ModeHidden {
		add(Requirement{Scope: ScopeOrder, Type: BlockBillingAddress, Label: "Billing address", Mode: billing, Status: status(billing, s.Billing), Code: codeBilling, Message: "Enter a billing address"})
	}
	add(Requirement{Scope: ScopeOrder, Type: BlockPayment, Label: "Payment", Mode: ModeRequired, Status: status(ModeRequired, s.Payment), Code: codePayment, Message: "Choose how to pay"})
	return out
}

// Blocking is the first requirement the cart still owes, as the error placement refuses with - nil when it can be placed
func Blocking(reqs []Requirement) error {
	for _, r := range reqs {
		if r.Blocking() {
			return apperr.New(r.Code, apperr.ClassValidation, r.Message)
		}
	}
	return nil
}

func lower(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}
