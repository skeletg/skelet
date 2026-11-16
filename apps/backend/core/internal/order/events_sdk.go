package order

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Accessors for sdk.CheckoutValidating and sdk.OrderPlacing, both built on sdk.CheckoutReview - the methods are the
// plugin contract, pinned in internal/plugin, and the changes follow the sync webhook's rules: the discount adds, the
// risk score only rises, tags append

func (e *CheckoutValidating) CheckoutOrderID() uuid.UUID         { return e.OrderID }
func (e *CheckoutValidating) CheckoutChannelID() uuid.UUID       { return e.ChannelID }
func (e *CheckoutValidating) CheckoutCustomerID() *uuid.UUID     { return e.CustomerID }
func (e *CheckoutValidating) CheckoutEmail() string              { return e.Email }
func (e *CheckoutValidating) CheckoutCartID() uuid.UUID          { return e.Cart.ID }
func (e *CheckoutValidating) CheckoutSubtotalCents() int64       { return e.Subtotal }
func (e *CheckoutValidating) CheckoutDiscountCents() int64       { return e.DiscountCents }
func (e *CheckoutValidating) CheckoutRiskScore() int             { return e.RiskScore }
func (e *CheckoutValidating) CheckoutTags() []string             { return append([]string(nil), e.Tags...) }
func (e *CheckoutValidating) CheckoutPaymentProvider() string    { return e.PaymentProvider }
func (e *CheckoutValidating) CheckoutEngineDiscountCents() int64 { return e.EngineDiscountCents }
func (e *CheckoutValidating) AddCheckoutDiscount(cents int64)    { e.DiscountCents += cents }
func (e *CheckoutValidating) RaiseCheckoutRiskScore(score int)   { e.RiskScore = max(e.RiskScore, score) }
func (e *CheckoutValidating) AddCheckoutTags(tags ...string)     { e.Tags = append(e.Tags, tags...) }

func (o *OrderPlacing) CheckoutOrderID() uuid.UUID       { return o.OrderID }
func (o *OrderPlacing) CheckoutChannelID() uuid.UUID     { return o.ChannelID }
func (o *OrderPlacing) CheckoutCustomerID() *uuid.UUID   { return o.CustomerID }
func (o *OrderPlacing) CheckoutEmail() string            { return o.Email }
func (o *OrderPlacing) CheckoutCartID() uuid.UUID        { return o.Cart.ID }
func (o *OrderPlacing) CheckoutSubtotalCents() int64     { return o.Subtotal }
func (o *OrderPlacing) CheckoutDiscountCents() int64     { return o.DiscountCents }
func (o *OrderPlacing) CheckoutRiskScore() int           { return o.RiskScore }
func (o *OrderPlacing) CheckoutTags() []string           { return append([]string(nil), o.Tags...) }
func (o *OrderPlacing) AddCheckoutDiscount(cents int64)  { o.DiscountCents += cents }
func (o *OrderPlacing) RaiseCheckoutRiskScore(score int) { o.RiskScore = max(o.RiskScore, score) }
func (o *OrderPlacing) AddCheckoutTags(tags ...string)   { o.Tags = append(o.Tags, tags...) }
func (o *OrderPlacing) PlacementTx() *sql.Tx             { return o.Tx }

func (o *OrderPlacing) SetPlacingTaxCents(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("order tax must be >= 0, got %d", cents)
	}
	o.TaxCents = &cents
	return nil
}

func (o *OrderPlacing) SetPlacingShippingCents(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("order shipping must be >= 0, got %d", cents)
	}
	o.ShippingCents = &cents
	return nil
}

// sdk.OrderCancelling - the note appends like the webhook's, sdk.OrderFulfilling - carrier and tracking replace
func (o *OrderCancelling) CancellingOrderID() uuid.UUID   { return o.OrderID }
func (o *OrderCancelling) CancellingChannelID() uuid.UUID { return o.ChannelID }
func (o *OrderCancelling) CancellingFromStatus() string   { return o.FromStatus }
func (o *OrderCancelling) CancellingNote() string         { return o.Note }
func (o *OrderCancelling) AddCancellingNote(note string) {
	if note = strings.TrimSpace(note); note != "" {
		o.Note = strings.TrimSpace(o.Note + " " + note)
	}
}

func (o *OrderFulfilling) FulfillingOrderID() uuid.UUID     { return o.OrderID }
func (o *OrderFulfilling) FulfillingChannelID() uuid.UUID   { return o.ChannelID }
func (o *OrderFulfilling) FulfillingFromStatus() string     { return o.FromStatus }
func (o *OrderFulfilling) FulfillingCarrier() string        { return o.Carrier }
func (o *OrderFulfilling) FulfillingTrackingNumber() string { return o.TrackingNumber }

func (o *OrderFulfilling) SetFulfillingCarrier(carrier string) error {
	if strings.TrimSpace(carrier) == "" {
		return fmt.Errorf("fulfillment carrier must not be empty")
	}
	o.Carrier = carrier
	return nil
}

func (o *OrderFulfilling) SetFulfillingTrackingNumber(number string) error {
	if strings.TrimSpace(number) == "" {
		return fmt.Errorf("fulfillment tracking number must not be empty")
	}
	o.TrackingNumber = number
	return nil
}

func (e *CheckoutValidating) CheckoutFieldValue(key string) (string, bool) {
	return answer(e.Answers, key)
}
func (o *OrderPlacing) CheckoutFieldValue(key string) (string, bool) { return answer(o.Answers, key) }

func answer(answers map[string]string, key string) (string, bool) {
	v, ok := answers[key]
	return v, ok && v != ""
}

func answers(fields []CheckoutFieldAnswer) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.Key] = f.Value
	}
	return out
}

var metadataKey = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}$`) // ":" for the <plugin>:<key> SetOrderMetadata writes

const maxMetadataValue = 2000

// The key is the plugin's to namespace, and "tracking" is core's own snapshot of the shopper
func (o *OrderPlacing) SetPlacingMetadata(key, value string) error {
	switch {
	case key == "tracking" || !metadataKey.MatchString(key):
		return fmt.Errorf("order metadata key %q: lowercase, up to 64 characters, and not \"tracking\"", key)
	case utf8.RuneCountInString(value) > maxMetadataValue:
		return fmt.Errorf("order metadata %q: at most %d characters", key, maxMetadataValue)
	}
	if o.Metadata == nil {
		o.Metadata = map[string]string{}
	}
	o.Metadata[key] = value
	return nil
}

func (o *OrderTransitioning) TransitioningOrderID() uuid.UUID   { return o.OrderID }
func (o *OrderTransitioning) TransitioningChannelID() uuid.UUID { return o.ChannelID }
func (o *OrderTransitioning) TransitioningFromStatus() string   { return o.FromStatus }
func (o *OrderTransitioning) TransitioningToStatus() string     { return o.ToStatus }
func (o *OrderTransitioning) TransitioningFromStage() string    { return o.FromStage }
func (o *OrderTransitioning) TransitioningToStage() string      { return o.ToStage }
