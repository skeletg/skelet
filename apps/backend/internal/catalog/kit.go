package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type KitComponent struct { // one fixed component of a kind=kit product - Quantity is the count per one kit unit, and components are SIMPLE products owning the inventory the kit decrements at fulfillment
	ID                    uuid.UUID
	ProductID             uuid.UUID
	ComponentProductID    uuid.UUID
	Quantity              float64
	SortOrder             int32
	ComponentURL          string // this and the ComponentSKU/Name/Price/Currency below are display fields - populated by ListKitComponents, zero when only the raw row is loaded
	ComponentSKU          string
	ComponentName         string
	ComponentDefaultPrice int64
	ComponentCurrency     string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AddKitComponentInput struct {
	ProductID          uuid.UUID
	ComponentProductID uuid.UUID
	Quantity           float64
	SortOrder          int32
}

type UpdateKitComponentInput struct {
	ID        uuid.UUID
	Quantity  *float64
	SortOrder *int32
}

var (
	ErrKitParentNotKit       = errors.New("kit components can only be defined on products of kind KIT")
	ErrKitComponentNotSimple = errors.New("kit components must reference SIMPLE products")
	ErrKitComponentIsParent  = errors.New("a kit cannot include itself as a component")
	ErrKitComponentNotFound  = errors.New("kit component not found")
)
