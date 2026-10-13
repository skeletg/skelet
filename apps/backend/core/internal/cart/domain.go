// Package cart is shopping-cart logic - customer-owned and guest carts, plus the merge when a customer logs in holding a guest cart
package cart

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"

	"github.com/google/uuid"
)

var ErrCartAlreadyConverted = errors.New("cart already converted")

// Deliberately one sentinel for "no such cart" and "not yours" - telling them apart lets anyone enumerate cart UUIDs
var ErrCartForbidden = errors.New("cart not found or not accessible")

var ErrCartNotActive = errors.New("cart is no longer active")

var ErrCartLineNotFound = errors.New("cart line not found")

var ErrCartStoreMismatch = errors.New("cart store mismatch")

var ErrQuantityOutOfBounds = errors.New("quantity outside the allowed range for this product")

var ErrProductNotPurchasable = errors.New("this product kind cannot be added to a cart directly") // simple, kit, gift-card and subscription can be direct lines - bundles go through AddBundleToCart with their selections, configurables resolve to a child SKU first

var ErrProductNotBundle = errors.New("AddBundleToCart requires a product of kind 'bundle'")

var ErrProductNotPublished = errors.New("product is not published on this store")

var ErrBundleSelectionMissing = errors.New("bundle requires a selection for a required option")

var ErrBundleSelectionInvalid = errors.New("bundle selection does not belong to this bundle")

var ErrBundleSelectionCardinality = errors.New("bundle option does not allow this many selections")

var ErrBundleOptionUnknown = errors.New("bundle option does not belong to this bundle")

var (
	ErrProductOptionUnknown     = errors.New("product option does not belong to this product")
	ErrProductOptionMissing     = errors.New("a required product option was not provided")
	ErrProductOptionsRequired   = errors.New("this product requires options - configure it on the product page")
	ErrLineConfigured           = errors.New("this item is in your cart with a configuration - adjust its quantity from the cart") // a plain add colliding with an existing configured line for the same SKU
	ErrProductOptionInvalid     = errors.New("invalid product option selection")
	ErrProductOptionHidden      = errors.New("product option is not available with the current selections") // loud by design - dropping it silently would desync the client's price from the server's
	ErrProductOptionQtyConflict = errors.New("selected options force conflicting quantities")               // two picked values force DIFFERENT line quantities
	ErrProductOptionQtyForced   = errors.New("quantity is set by the selected product options")
	ErrProductOptionCardinality = errors.New("product option does not allow this many values")
	ErrOptionsUnsupportedKind   = errors.New("product kind does not support custom options") // sentinel the catalog adapter returns for kinds that never carry options (bundle, grouped, ...) - gates treat it as "no options" while any OTHER resolve error fails closed instead of skipping enforcement
)

// Enough catalog contract to enforce the kind invariant on add and materialize bundle structure - declared here so cart doesn't import catalog
type ProductResolver interface {
	ResolveSimplePricing(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (SimplePricing, error)             // canonical name and price, so the cart ignores client-supplied pricing - errors when the SKU has no listing or can't be a direct line
	ResolveBundleBySKU(ctx context.Context, channelID uuid.UUID, sku string) (BundleSnapshot, error)                                     // errors with ErrProductNotPublished unless the bundle is published on the channel
	ResolveProductOptions(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (ProductOptionsResolution, error) // customerID scopes pricing to B2B contract prices and customer-group tiers - nil for guests
}

type FileAssetResolver interface { // resolving also confirms the upload actually landed
	ResolveFileAsset(ctx context.Context, id uuid.UUID) (FileAsset, error)
}

// Copied onto the line at add time so the client can't tamper with name or price
type SimplePricing struct {
	ProductName string
	UnitPrice   int64
	MinQty      *float64
	MaxQty      *float64
	QtyStep     float64
	CostCents   int64
	Tiers       []pricing.Tier // already scope-filtered by channel and customer group - the cart applies the lowest tier the line quantity reaches
}

type BundleSnapshot struct { // typed strings and ints so the cart doesn't import catalog domain types
	ProductID   uuid.UUID
	ProductName string
	Options     []BundleOptionSnapshot
}

type BundleOptionSnapshot struct {
	ID            uuid.UUID
	InputType     string // "radio" | "dropdown" | "checkbox" | "multiselect"
	IsRequired    bool
	MinSelections int32
	MaxSelections *int32
	Selections    []BundleSelectionSnapshot
}

type BundleSelectionSnapshot struct {
	ID                uuid.UUID
	ProductID         uuid.UUID
	ProductSKU        string
	ProductName       string
	UnitPrice         int64
	QuantityPerBundle float64
	IsDefault         bool
}

// CustomerID is nil for guests - every cart belongs to one store, and a customer holds one active cart per store
type Cart struct {
	ID         uuid.UUID
	CustomerID *uuid.UUID
	ChannelID  uuid.UUID
	Status     string
	Email      string
	Note       string
	Checkout   Checkout
	Lines      []CartLine
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Checkout is what the shopper has chosen so far, saved step by step so a reload resumes where they were - the cart stores it as given, and the resolvers validate each step before it lands here. Placement re-prices and re-validates all of it
type Checkout struct {
	ShippingAddress       *Address
	ShippingAddressID     *uuid.UUID // the saved address the snapshot came from, nil when typed in
	BillingSameAsShipping bool
	BillingAddress        *Address
	BillingAddressID      *uuid.UUID
	ShippingCarrierSlug   string // with ShippingMethodCode, empty when nothing is chosen
	ShippingMethodCode    string
	PaymentProviderSlug   string
	PONumber              string
	UseStoreCredit        bool
	GiftCardCodes         []string
	// Destination is where the order is headed before a full address exists - the first step quotes from it, and saving a shipping address overwrites it with that address's own
	Destination   Destination
	ContactPhone  string
	PurchaseType  string            // PurchasePersonal or PurchaseBusiness
	Company       string            // business purchases only, with BusinessIDs
	BusinessIDs   map[string]string // the company identifiers the country asks for, by field key ("pib" -> "123456789")
	PickupContact *PickupContact    // who collects a pickup order, nil until the details step of one
	// Deliveries holds the shopper's choice per delivery group, keyed by the group's key - a cart delivered as one uses delivery.CombinedGroupKey. Groups are recomputed from the lines on every read, so a selection whose group no longer exists is simply ignored
	Deliveries map[string]DeliverySelection
	// SplitDelivery is the shopper asking to choose per group when one option could cover every line
	SplitDelivery bool
}

// DeliverySelection is one group's delivery - the option and carrier method, and the point when the option's kind needs one. The point is a snapshot, re-checked at placement
type DeliverySelection struct {
	OptionID    uuid.UUID       `json:"optionId"`
	CarrierSlug string          `json:"carrierSlug,omitempty"`
	MethodCode  string          `json:"methodCode,omitempty"`
	Point       *delivery.Point `json:"point,omitempty"`
}

// Delivery is the group's selection - the zero value when nothing is chosen
func (c Checkout) Delivery(groupKey string) (DeliverySelection, bool) {
	d, ok := c.Deliveries[groupKey]
	return d, ok && d.OptionID != uuid.Nil
}

// SetDelivery stores a group's selection
func (c *Checkout) SetDelivery(groupKey string, d DeliverySelection) {
	if c.Deliveries == nil {
		c.Deliveries = map[string]DeliverySelection{}
	}
	c.Deliveries[groupKey] = d
}

const (
	PurchasePersonal = "personal"
	PurchaseBusiness = "business"
)

// Destination is the country, region and postcode shipping and tax are quoted for
type Destination struct {
	CountryCode string
	Region      string
	PostalCode  string
}

func (d Destination) IsZero() bool { return d.CountryCode == "" }

// PickupContact names who collects a pickup order - the shopper, or someone they send
type PickupContact struct {
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	SomeoneElse bool   `json:"someoneElse"`
}

// Address is a snapshot in the AddressInput shape - the storage format of the cart's address columns
type Address struct {
	FirstName   string  `json:"firstName"`
	LastName    string  `json:"lastName"`
	Company     *string `json:"company,omitempty"`
	Street1     string  `json:"street1"`
	Street2     *string `json:"street2,omitempty"`
	City        string  `json:"city"`
	State       *string `json:"state,omitempty"`
	PostalCode  string  `json:"postalCode"`
	CountryCode string  `json:"countryCode"`
	Phone       *string `json:"phone,omitempty"`
}

// EffectiveBilling is the billing address placement uses - the shipping snapshot when billing mirrors it
func (c Checkout) EffectiveBilling() (*Address, *uuid.UUID) {
	if c.BillingSameAsShipping {
		return c.ShippingAddress, c.ShippingAddressID
	}
	return c.BillingAddress, c.BillingAddressID
}

// QuoteDestination is what shipping and tax are quoted for - the shipping address once there is one, else the destination the first step saved
func (c Checkout) QuoteDestination() Destination {
	if c.ShippingAddress != nil {
		a := c.ShippingAddress
		d := Destination{CountryCode: a.CountryCode, PostalCode: a.PostalCode}
		if a.State != nil {
			d.Region = *a.State
		}
		return d
	}
	return c.Destination
}

// ShippingCountry is the destination's country, empty until the first step is saved
func (c Checkout) ShippingCountry() string {
	return c.QuoteDestination().CountryCode
}

// A BUNDLE line carries the bundle total with picks in Components, one row per component - simple lines leave it empty
type CartLine struct {
	ID          uuid.UUID
	CartID      uuid.UUID
	ProductName string
	ProductSku  string
	UnitPrice   int64
	Quantity    float64
	IsGift      bool // a promotion free gift - order placement appends these in memory after the engine runs, so they reach order_lines and the stock reservation without ever being a cart_lines row the shopper could edit
	Components  []CartLineComponent
	Options     []CartLineOption
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CartLineOptionInput struct { // ValueIDs for select-family options, TextValue for free-text, ISO-date and the file reference
	OptionID    uuid.UUID
	ValueIDs    []uuid.UUID
	TextValue   string
	FileAssetID *uuid.UUID // transient - the server resolves it to validate and snapshot the URL plus filename, the id itself isn't persisted
}

type CartLineOption struct { // PriceDelta is the cents added to the UNIT price, already multiplied by the value's quantity
	ID              uuid.UUID
	ProductOptionID uuid.UUID
	OptionValueID   *uuid.UUID
	InputType       string
	OptionTitle     string
	ValueLabel      string
	TextValue       string
	PriceDelta      int64
	Quantity        float64
	SortOrder       int32
	FileURL         string
}

type AddConfiguredToCartInput struct { // re-adding the same SKU replaces the configuration rather than summing, like AddBundleToCartInput
	ProductSku string
	Quantity   float64
	Options    []CartLineOptionInput
}

// Quantity is per-bundle-unit, multiplied by the parent at reservation - the option and selection ids are snapshots rather than FKs, so editing the bundle later can't break the line
type CartLineComponent struct {
	ID                uuid.UUID
	CartLineID        uuid.UUID
	BundleOptionID    uuid.UUID
	BundleSelectionID uuid.UUID
	ProductID         uuid.UUID
	ProductSku        string
	ProductName       string
	UnitPrice         int64
	Quantity          float64
	SortOrder         int32
	CreatedAt         time.Time
}

type AddCartLineInput struct {
	ProductName string
	ProductSku  string
	UnitPrice   int64
	Quantity    float64
}

type AddBundleLineInput struct {
	ProductName string
	ProductSku  string
	UnitPrice   int64
	Quantity    float64
	Components  []AddCartLineComponentInput
}

type AddConfiguredLineInput struct {
	ProductName string
	ProductSku  string
	UnitPrice   int64
	Quantity    float64
	Options     []CartLineOption
}

type SavedItem struct { // parked off the cart via save-for-later - cart-scoped and guest-capable, simple lines only in v1
	ID          uuid.UUID
	CartID      uuid.UUID
	ProductSku  string
	ProductName string
	Quantity    float64
	CreatedAt   time.Time
}

type AddCartLineComponentInput struct {
	BundleOptionID    uuid.UUID
	BundleSelectionID uuid.UUID
	ProductID         uuid.UUID
	ProductSku        string
	ProductName       string
	UnitPrice         int64
	Quantity          float64
	SortOrder         int32
}

type Repository interface {
	GetCartByID(ctx context.Context, cartID uuid.UUID) (Cart, error)
	GetCartByCustomerAndChannel(ctx context.Context, customerID, channelID uuid.UUID) (Cart, error)
	CreateCart(ctx context.Context, customerID *uuid.UUID, channelID uuid.UUID) (Cart, error)
	ReassignCart(ctx context.Context, cartID, customerID uuid.UUID) (Cart, error)
	GetCartLineByID(ctx context.Context, lineID uuid.UUID) (CartLine, error)
	ListCartLinesByCart(ctx context.Context, cartID uuid.UUID) ([]CartLine, error)
	UpsertCartLineBySku(ctx context.Context, cartID uuid.UUID, input AddCartLineInput) (CartLine, error)            // SUMS an existing same-SKU line, and the incoming unit_price wins
	UpsertBundleCartLine(ctx context.Context, cartID uuid.UUID, input AddBundleLineInput) (CartLine, error)         // on conflict quantity and unit_price REPLACE - a fresh selection, not more of the old mix - and the components are replaced in place
	UpsertConfiguredCartLine(ctx context.Context, cartID uuid.UUID, input AddConfiguredLineInput) (CartLine, error) // REPLACES quantity and unit_price, and replaces the option rows in place
	UpdateCartLineQuantity(ctx context.Context, lineID uuid.UUID, quantity float64) (CartLine, error)
	UpdateCartLineQuantityPrice(ctx context.Context, lineID uuid.UUID, quantity float64, unitPrice int64) (CartLine, error)
	RemoveCartLine(ctx context.Context, lineID uuid.UUID) error
	ClearCartLines(ctx context.Context, cartID uuid.UUID) error
	ConvertCart(ctx context.Context, cartID uuid.UUID) error
	SetCartMerged(ctx context.Context, cartID uuid.UUID) error

	ListCarts(ctx context.Context, f CartFilter) ([]CartListItem, error)
	CountCarts(ctx context.Context, f CartFilter) (int32, error)

	ListAbandoned(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]AbandonedCart, error) // one channel's active non-empty carts last touched between maxAge and threshold, and not notified since
	MarkAbandonedNotified(ctx context.Context, ids []uuid.UUID) error

	SetEmail(ctx context.Context, cartID uuid.UUID, email string) error

	SetNote(ctx context.Context, cartID uuid.UUID, note string) error

	LockCart(ctx context.Context, cartID uuid.UUID) (Cart, error) // GetCartByID under FOR UPDATE, for a checkout read-modify-write inside a transaction
	SetCheckout(ctx context.Context, cartID uuid.UUID, c Checkout) error

	PruneStale(ctx context.Context, cutoff time.Time) (int64, error)

	SaveItem(ctx context.Context, cartID uuid.UUID, sku, name string, qty float64) (SavedItem, error)
	GetSavedItem(ctx context.Context, id uuid.UUID) (SavedItem, error)
	DeleteSavedItem(ctx context.Context, id, cartID uuid.UUID) error
	ListSavedItems(ctx context.Context, cartID uuid.UUID) ([]SavedItem, error)

	WithTx(tx *sql.Tx) Repository
}

type AbandonedCart struct { // exactly one of CustomerID / Email identifies the recipient - the flow engine prefers the customer
	ID           uuid.UUID
	ChannelID    uuid.UUID
	CustomerID   uuid.UUID // uuid.Nil for guest carts
	Email        string    // set for guest carts, may be empty for customer carts
	LastActivity time.Time // when the cart or a line was last touched - the abandonment episode, a cart touched and left again is a new one
}

type CartFilter struct {
	ChannelID     uuid.UUID
	CustomerID    *uuid.UUID
	Status        *string
	UpdatedBefore *time.Time
	Search        *string
	Limit         int32
	Offset        int32
}

type CartListItem struct {
	ID                uuid.UUID
	CustomerID        *uuid.UUID
	ChannelID         uuid.UUID
	Status            string
	CustomerEmail     *string
	CustomerFirstName *string
	CustomerLastName  *string
	ItemCount         int32
	Subtotal          int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
