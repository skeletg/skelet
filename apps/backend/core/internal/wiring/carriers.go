package wiring

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

func RegisterBuiltinCarriers(sh *shipping.Service, inv *inventory.Service) { // explicit rather than init() - both wrap a constructed service, so they can't sit with the plugin carriers that do register from init()
	shipping.RegisterCarrier(shipping.CarrierRegistration{
		Slug:        shipping.BuiltinCarrierSlug,
		Carrier:     shipping.NewBuiltinCarrier(sh),
		DisplayName: "Store rates",
		Kinds:       []string{shipping.DefaultKind, "parcel_locker", "carrier_point"}, // a method names its kind - door delivery, a picked pickup point, or the nearest one
	})
	shipping.RegisterCarrier(shipping.CarrierRegistration{ // click-and-collect - every pickup-enabled location becomes a $0 option at checkout
		Slug:        shipping.PickupCarrierSlug,
		Carrier:     shipping.NewPickupCarrier(NewInventoryPickupLister(inv)),
		DisplayName: "Store pickup",
		Kinds:       []string{"store_pickup"},
	})
}
