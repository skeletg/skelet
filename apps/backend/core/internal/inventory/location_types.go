package inventory

import (
	"context"
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

// NormalizeLocationType accepts established aliases at the domain boundary so
// existing integrations can migrate without creating arbitrary classifications.
func NormalizeLocationType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", "-", " ", "-").Replace(value)
	switch value {
	case "", LocationUnclassified:
		return LocationUnclassified, nil
	case "retail-store", "store", "shop", LocationRetail:
		return LocationRetail, nil
	case LocationWarehouse, LocationStorage:
		return value, nil
	case "dropshipper", LocationDropshipper:
		return LocationDropshipper, nil
	case "fulfillment-partner", LocationFulfillment:
		return LocationFulfillment, nil
	default:
		return "", ErrInvalidLocationType
	}
}

type LocationFilter struct {
	Types                                      []string
	City                                       string
	IsActive, StorefrontVisible, PickupEnabled *bool
}

func (s *Service) FilteredLocationsPage(ctx context.Context, search string, filter LocationFilter, limit, offset int32) ([]Location, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset = max(0, offset)
	search = strings.TrimSpace(search)
	filter.City = strings.TrimSpace(filter.City)
	for i, typ := range filter.Types {
		v, err := NormalizeLocationType(typ)
		if err != nil {
			return nil, 0, err
		}
		filter.Types[i] = v
	}
	rows, err := s.repo.ListFilteredLocationsPage(ctx, search, filter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.repo.CountFilteredLocations(ctx, search, filter)
	return rows, count, err
}

func (s *Service) CountFilteredLocations(ctx context.Context, search string, filter LocationFilter) (int64, error) {
	filter.City = strings.TrimSpace(filter.City)
	for i, typ := range filter.Types {
		value, err := NormalizeLocationType(typ)
		if err != nil {
			return 0, err
		}
		filter.Types[i] = value
	}
	return s.repo.CountFilteredLocations(ctx, strings.TrimSpace(search), filter)
}
