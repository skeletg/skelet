package delivery

// The kinds every install has
const (
	KindShipToAddress = "ship_to_address"
	KindStorePickup   = "store_pickup"
	KindParcelLocker  = "parcel_locker"
	KindCarrierPoint  = "carrier_point"
)

// The point sources core ships - the channel's pickup-enabled inventory locations for store pickup, and the pickup points (lockers, parcel shops, PO boxes) the admin enters for parcel_locker
const (
	InventoryPointSource = "inventory-locations"
	ManualPointSource    = "manual-points"
)

func init() {
	registerCoreKinds()
	registerManualPointSource()
}

func registerCoreKinds() {
	RegisterKind(Kind{
		Slug:        KindShipToAddress,
		Label:       "Ship to address",
		Description: "A carrier delivers to the address the shopper enters.",
		ShipTo:      ShipToCustomer,
		Blocks: []BlockSpec{
			{Type: BlockShippingAddress, Label: "Shipping address", Floor: FloorLocked, Reason: "Where the carrier delivers"},
			{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorLocked, Reason: "Invoice - may be the same as shipping"},
			{Type: BlockPhone, Label: "Phone", Floor: FloorConfigurable, Default: ModeOptional, Reason: "For the courier"},
		},
	})
	RegisterKind(Kind{
		Slug:        KindStorePickup,
		Label:       "Store pickup",
		Description: "The shopper collects the order from a store or warehouse.",
		ShipTo:      ShipToPoint,
		PointLabel:  "Store",
		Blocks: []BlockSpec{
			{Type: BlockPoint, Label: "Store", Floor: FloorLocked, Reason: "Its address becomes the ship-to address"},
			{Type: BlockShippingAddress, Label: "Shipping address", Floor: FloorNA, Reason: "Supplied by the store"},
			{Type: BlockCollector, Label: "Who collects", Floor: FloorLocked, Reason: "The store checks the name on collection"},
			{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorConfigurable, Default: ModeOptional, Reason: "Invoice - no shopper address to copy from"},
			{Type: BlockPhone, Label: "Phone", Floor: FloorConfigurable, Default: ModeOptional, Reason: "To say the order is ready"},
		},
	})
	RegisterKind(Kind{
		Slug:        KindParcelLocker,
		Label:       "Pickup point",
		Description: "The carrier leaves the parcel at a locker, parcel shop or PO box the shopper picks.",
		ShipTo:      ShipToPoint,
		PointLabel:  "Pickup point",
		Blocks: []BlockSpec{
			{Type: BlockPoint, Label: "Pickup point", Floor: FloorLocked, Reason: "Its address becomes the ship-to address"},
			{Type: BlockShippingAddress, Label: "Shipping address", Floor: FloorNA, Reason: "Supplied by the pickup point"},
			{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorLocked, Reason: "No shopper address to copy from"},
			{Type: BlockPhone, Label: "Phone", Floor: FloorLocked, Reason: "The carrier sends the pickup code or notice"},
		},
	})
	// The carrier routes the parcel to the pickup point nearest the shopper's address - the shopper enters an address as for door delivery and never picks a point
	RegisterKind(Kind{
		Slug:        KindCarrierPoint,
		Label:       "Nearest pickup point",
		Description: "The carrier delivers to the pickup point nearest the shopper's address, and tells the shopper where to collect.",
		ShipTo:      ShipToCustomer,
		Blocks: []BlockSpec{
			{Type: BlockShippingAddress, Label: "Shipping address", Floor: FloorLocked, Reason: "The carrier picks the nearest point from it"},
			{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorLocked, Reason: "Invoice - may be the same as shipping"},
			{Type: BlockPhone, Label: "Phone", Floor: FloorLocked, Reason: "The carrier sends where and when to collect"},
		},
	})
}

// DigitalKind is the pseudo-kind a cart's digital lines check out under - never an option, but the dashboard and requirements read its blocks like any other
var DigitalKind = Kind{
	Slug:   "digital",
	Label:  "Digital",
	ShipTo: "",
	Blocks: []BlockSpec{
		{Type: BlockEmail, Label: "Email", Floor: FloorLocked, Reason: "Receipt and download links"},
		{Type: BlockBillingAddress, Label: "Billing address", Floor: FloorLocked, Reason: "Invoice"},
		{Type: BlockPhone, Label: "Phone", Floor: FloorConfigurable, Default: ModeHidden},
		{Type: BlockPayment, Label: "Payment", Floor: FloorLocked, Reason: "Every order is paid for"},
	},
}
