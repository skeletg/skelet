package cart

import (
	"fmt"

	"github.com/google/uuid"
)

// Accessors for sdk.CartLineAdding and sdk.CartLineUpdating - a plugin reaches these events through `any` and can't
// name the types, so the methods are its contract and the fields stay core's. internal/plugin pins both at compile
// time; renaming one is a breaking SDK change. The setters take the same values a sync webhook's response may carry

func (e *LineAdding) LineCartID() uuid.UUID      { return e.CartID }
func (e *LineAdding) LineChannelID() uuid.UUID   { return e.ChannelID }
func (e *LineAdding) LineCustomerID() *uuid.UUID { return e.CustomerID }
func (e *LineAdding) LineSKU() string            { return e.ProductSku }
func (e *LineAdding) LineName() string           { return e.ProductName }
func (e *LineAdding) LineQuantity() float64      { return derefFloat(e.Quantity) }
func (e *LineAdding) LineUnitPriceCents() int64  { return derefInt(e.UnitPrice) }

func (e *LineAdding) SetLineUnitPrice(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("cart line unit price must be >= 0, got %d", cents)
	}
	e.UnitPrice = &cents
	return nil
}

func (e *LineAdding) SetLineQuantity(qty float64) error {
	if qty <= 0 {
		return fmt.Errorf("cart line quantity must be > 0, got %v", qty)
	}
	e.Quantity = &qty
	return nil
}

func (e *LineUpdating) LineCartID() uuid.UUID      { return e.CartID }
func (e *LineUpdating) LineChannelID() uuid.UUID   { return e.ChannelID }
func (e *LineUpdating) LineCustomerID() *uuid.UUID { return e.CustomerID }
func (e *LineUpdating) CartLineID() uuid.UUID      { return e.LineID }
func (e *LineUpdating) LineSKU() string            { return e.ProductSku }
func (e *LineUpdating) LineQuantity() float64      { return derefFloat(e.Quantity) }

func (e *LineUpdating) SetLineQuantity(qty float64) error {
	if qty <= 0 {
		return fmt.Errorf("cart line quantity must be > 0, got %v", qty)
	}
	e.Quantity = &qty
	return nil
}

// Every emitter sets both, so these only guard a hand-built event
func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// BundleAdding is sdk.CartLineAdding too - the same shape as a single line, the price being the bundle's
func (e *BundleAdding) LineCartID() uuid.UUID      { return e.CartID }
func (e *BundleAdding) LineChannelID() uuid.UUID   { return e.ChannelID }
func (e *BundleAdding) LineCustomerID() *uuid.UUID { return e.CustomerID }
func (e *BundleAdding) LineSKU() string            { return e.ProductSku }
func (e *BundleAdding) LineName() string           { return e.ProductName }
func (e *BundleAdding) LineQuantity() float64      { return derefFloat(e.Quantity) }
func (e *BundleAdding) LineUnitPriceCents() int64  { return derefInt(e.UnitPrice) }

func (e *BundleAdding) SetLineUnitPrice(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("cart line unit price must be >= 0, got %d", cents)
	}
	e.UnitPrice = &cents
	return nil
}

func (e *BundleAdding) SetLineQuantity(qty float64) error {
	if qty <= 0 {
		return fmt.Errorf("cart line quantity must be > 0, got %v", qty)
	}
	e.Quantity = &qty
	return nil
}

// sdk.CartLineRemoving - refuse-only, so no setters
func (e *LineRemoving) RemovingCartID() uuid.UUID      { return e.CartID }
func (e *LineRemoving) RemovingChannelID() uuid.UUID   { return e.ChannelID }
func (e *LineRemoving) RemovingCustomerID() *uuid.UUID { return e.CustomerID }
func (e *LineRemoving) RemovingLineID() uuid.UUID      { return e.LineID }
