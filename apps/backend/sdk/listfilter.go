package sdk

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// A core list a plugin can filter by its own data
type ListKind string

const (
	ListProducts  ListKind = "products"
	ListCustomers ListKind = "customers"
	ListOrders    ListKind = "orders"
)

type ListFilterOption struct {
	Value, Label string
}

// A filter on one of core's admin lists by the plugin's own data - "has a warranty", "ERP sync failed". The dashboard
// offers it in the list's toolbar with its options, and core narrows the list to the records Match answers
type ListFilter struct {
	List    ListKind
	Key     string // lowercase, stable - the list asks for it as pl.Key of it ("<plugin>:has_warranty")
	Label   string
	Options []ListFilterOption // what an admin picks from, at least one
	// The ids of the list's records matching any of the picked values. Core already gated the list on its own
	// permission, and intersects your answer with its other filters
	Match func(ctx context.Context, values []string) ([]uuid.UUID, error)
}

type RegisteredListFilter struct {
	PluginSlug string
	Filter     ListFilter
}

// Key is the filter's key as the list asks for it, "<plugin>:<key>"
func (r RegisteredListFilter) Key() string { return r.PluginSlug + ":" + r.Filter.Key }

var listFilters registry[RegisteredListFilter]

// Panics on a malformed filter or a key the plugin already registered for that list
func (pl *Plugin) RegisterListFilter(f ListFilter) {
	switch {
	case !slices.Contains([]ListKind{ListProducts, ListCustomers, ListOrders}, f.List):
		panic("sdk.RegisterListFilter: no list " + string(f.List))
	case !hookKey.MatchString(f.Key):
		panic("sdk.RegisterListFilter: key " + f.Key + hookKeyRule)
	case f.Label == "":
		panic("sdk.RegisterListFilter: " + f.Key + " needs a label")
	case len(f.Options) == 0 || slices.ContainsFunc(f.Options, func(o ListFilterOption) bool { return o.Value == "" || o.Label == "" }):
		panic("sdk.RegisterListFilter: " + f.Key + " needs options, each with a value and a label")
	case f.Match == nil:
		panic("sdk.RegisterListFilter: " + f.Key + " needs a Match func")
	}
	f.Options = slices.Clone(f.Options)
	listFilters.add(RegisteredListFilter{PluginSlug: pl.slug, Filter: f},
		func(r RegisteredListFilter) bool {
			return r.PluginSlug == pl.slug && r.Filter.List == f.List && r.Filter.Key == f.Key
		},
		"sdk.RegisterListFilter: "+pl.slug+" already filters "+string(f.List)+" by "+f.Key)
}

// ListFilters is every list filter the compiled-in plugins registered, in the order they did
func ListFilters() []RegisteredListFilter {
	out := listFilters.all()
	for i := range out {
		out[i].Filter.Options = slices.Clone(out[i].Filter.Options)
	}
	return out
}
