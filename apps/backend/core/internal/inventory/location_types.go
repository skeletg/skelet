package inventory

import (
	"strings"
)

const (
	LocationRetail       = "retail"
	LocationWarehouse    = "warehouse"
	LocationStorage      = "storage"
	LocationDropshipper  = "drop-shipper"
	LocationFulfillment  = "fulfillment-center"
	LocationUnclassified = "unclassified"
)

// NormalizeLocationType validates a type; unset means unclassified. Legacy free-form
// values were mapped once by migration 000041, so no aliases are accepted here.
func NormalizeLocationType(value string) (string, error) {
	switch value {
	case "":
		return LocationUnclassified, nil
	case LocationRetail, LocationWarehouse, LocationStorage, LocationDropshipper, LocationFulfillment, LocationUnclassified:
		return value, nil
	default:
		return "", ErrInvalidLocationType
	}
}

type LocationFilter struct {
	Types                                      []string
	City                                       string
	IsActive, StorefrontVisible, PickupEnabled *bool
}

// normalizeLocationQuery trims and validates once, so the page and its count always filter identically
func normalizeLocationQuery(search string, filter LocationFilter) (string, LocationFilter, error) {
	filter.City = strings.TrimSpace(filter.City)
	types := make([]string, len(filter.Types))
	for i, typ := range filter.Types {
		v, err := NormalizeLocationType(typ)
		if err != nil {
			return "", filter, err
		}
		types[i] = v
	}
	filter.Types = types
	return strings.TrimSpace(search), filter, nil
}
