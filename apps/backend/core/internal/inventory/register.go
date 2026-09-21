package inventory

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrLocationNotFound, "INVENTORY_LOCATION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrItemNotFound, "INVENTORY_ITEM_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrLevelNotFound, "INVENTORY_LEVEL_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrLocationSlugTaken, "INVENTORY_LOCATION_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrItemAlreadyExists, "INVENTORY_ITEM_EXISTS", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInsufficientStock, "INSUFFICIENT_STOCK", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrNoLocationsForChannel, "NO_LOCATIONS_FOR_CHANNEL", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrNegativeQuantity, "INVENTORY_NEGATIVE_QUANTITY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFlatModePerLocation, "INVENTORY_FLAT_MODE_PER_LOCATION", apperr.ClassValidation)
}
