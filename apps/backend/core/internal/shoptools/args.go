package shoptools

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// The SDK hands arguments over unchecked against the schema, so anything a caller could make the store choke on is refused here
const (
	maxQueryLen     = 200
	maxRefLen       = 500
	maxFilterFacets = 10
	maxFilterValues = 20
	maxPrice        = 1e9
)

type args struct {
	Query      string              `json:"query"`
	Category   string              `json:"category"`
	Collection string              `json:"collection"`
	PriceMin   *float64            `json:"price_min"`
	PriceMax   *float64            `json:"price_max"`
	Filters    map[string][]string `json:"filters"`
	Sort       string              `json:"sort"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	Product    string              `json:"product"`
	Parent     string              `json:"parent"`
	Country    string              `json:"country"`
	Region     string              `json:"region"`
	PostalCode string              `json:"postal_code"`
	CartID     string              `json:"cart_id"`
	Lines      []lineArg           `json:"lines"`
	Coupon     string              `json:"coupon"`
}

type lineArg struct {
	SKU      string   `json:"sku"`
	Quantity *float64 `json:"quantity"`
}

func (a args) check() error {
	for name, v := range map[string]string{"query": a.Query, "sort": a.Sort, "coupon": a.Coupon, "region": a.Region, "postal_code": a.PostalCode} {
		if utf8.RuneCountInString(v) > maxQueryLen {
			return refusal(fmt.Sprintf("%s is too long - keep it under %d characters.", name, maxQueryLen))
		}
	}
	if len(a.Product) > maxRefLen {
		return refusal("product is too long for an id, SKU or link.")
	}
	for name, p := range map[string]*float64{"price_min": a.PriceMin, "price_max": a.PriceMax} {
		if p != nil && (*p < 0 || *p > maxPrice || math.IsNaN(*p)) {
			return refusal(fmt.Sprintf("%s takes a price between 0 and %.0f.", name, maxPrice))
		}
	}
	if len(a.Filters) > maxFilterFacets {
		return refusal(fmt.Sprintf("Filter on at most %d facets at once.", maxFilterFacets))
	}
	for slug, values := range a.Filters {
		if len(values) > maxFilterValues || len(slug) > maxQueryLen {
			return refusal(fmt.Sprintf("Pick at most %d values per facet.", maxFilterValues))
		}
	}
	if len(a.Lines) > maxLinesPerUpdate {
		return refusal(fmt.Sprintf("Send at most %d lines at a time.", maxLinesPerUpdate))
	}
	return nil
}
