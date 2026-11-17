package sdk

import (
	"context"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxFeeCents is the most one fee may charge, in minor units - what an amount on the API holds, so a fee that's stored
// is a fee that reads back
const MaxFeeCents = math.MaxInt32

// MaxFeeLabel is the longest label a fee may carry, in characters - it has a line of a checkout summary, an email and
// an invoice to fit beside its amount
const MaxFeeLabel = 60

// A charge of the plugin's own on an order - handling, a payment-method surcharge, an eco fee. Core prices it after
// shipping and before tax, taxes it under TaxClass, and keeps it on the order beside the label the shopper saw
type Fee struct {
	Key   string // lowercase, stable - the order keeps the fee under pl.Key of it ("<plugin>:handling")
	Label string // what the shopper reads beside the amount
	// The tax class core taxes it under - empty is the class a product without one gets. A remote tax provider is
	// handed the fee and taxes it its own way
	TaxClass string
	Untaxed  bool // charged without tax, whatever the class
	// What the fee comes to for this checkout, at most MaxFeeCents - 0 charges nothing and leaves no line. It runs
	// whenever the checkout is priced, so compute rather than act. An sdk.UserError's message reaches the shopper and
	// stops the order; so does any other error, since charging less than the store asked is worse than not selling
	Charge func(ctx context.Context, req FeeRequest) (int64, error)
}

// The checkout a fee is priced for. Amounts are in the channel's currency, in minor units, and hold their tax when
// TaxInclusive - as a fee you return must too
type FeeRequest struct {
	ChannelID         uuid.UUID
	CurrencyCode      string
	TaxInclusive      bool
	CustomerID        *uuid.UUID // nil for a guest
	CustomerGroupSlug string
	CountryCode       string // where the order ships, or the billing country when nothing ships
	RegionCode        string
	PaymentMethod     string // the payment method picked, empty until the shopper picks one
	ShippingCarrier   string // the carrier and method picked, empty for none or before one is
	ShippingMethod    string
	SubtotalCents     int64
	DiscountCents     int64
	ShippingCents     int64
	Lines             []FeeLine
	// The shopper's answers to checkout fields, keyed "<plugin>:<key>" - read yours with pl.FeeAnswer
	Answers map[string]string
}

type FeeLine struct {
	SKU        string
	Quantity   float64
	UnitCents  int64
	TotalCents int64
	Gift       bool // a free gift a promotion added
	Custom     bool // a charge an admin added to an offer, with no product behind it
}

type RegisteredFee struct {
	PluginSlug string
	Fee        Fee
}

// Key is the fee's key as core keeps it, "<plugin>:<key>"
func (r RegisteredFee) Key() string { return r.PluginSlug + ":" + r.Fee.Key }

var fees registry[RegisteredFee]

// Charged on every channel the plugin is enabled for, in the order plugins registered them - panics on a malformed
// fee or a key the plugin already charges
func (pl *Plugin) RegisterFee(f Fee) {
	switch {
	case !hookKey.MatchString(f.Key):
		panic("sdk.RegisterFee: key " + f.Key + hookKeyRule)
	case f.Label == "":
		panic("sdk.RegisterFee: " + f.Key + " needs a label")
	case utf8.RuneCountInString(f.Label) > MaxFeeLabel:
		panic("sdk.RegisterFee: " + f.Key + "'s label is over " + strconv.Itoa(MaxFeeLabel) + " characters")
	case f.Charge == nil:
		panic("sdk.RegisterFee: " + f.Key + " needs a Charge func")
	}
	fees.add(RegisteredFee{PluginSlug: pl.slug, Fee: f},
		func(r RegisteredFee) bool { return r.PluginSlug == pl.slug && r.Fee.Key == f.Key },
		"sdk.RegisterFee: "+pl.slug+" already charges "+f.Key)
}

// Fees is every fee the compiled-in plugins registered, in the order they did
func Fees() []RegisteredFee { return fees.all() }

// FeeAnswer reads the shopper's answer to one of this plugin's own checkout fields
func (pl *Plugin) FeeAnswer(req FeeRequest, key string) (string, bool) {
	v, ok := req.Answers[pl.Key(key)]
	return v, ok
}
