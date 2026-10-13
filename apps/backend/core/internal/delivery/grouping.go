package delivery

import (
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Need is what an order has to deliver - nothing (digital only), goods, or both
type Need string

const (
	NeedNone     Need = "none"
	NeedPhysical Need = "physical"
	NeedMixed    Need = "mixed"
)

// LineProfile is what grouping needs of a line's product - whether it ships and which kinds its type allows (empty = every core kind - ship to address, store pickup, pickup point, nearest pickup point)
type LineProfile struct {
	ProductTypeID    uuid.UUID
	SKU              string
	ProductID        uuid.UUID
	RequiresShipping bool
	DeliveryKinds    []string
	TypeName         string
}

// Line is a cart line as grouping sees it
type Line struct {
	ID        uuid.UUID
	SKU       string
	Name      string
	Quantity  float64
	UnitPrice int64
}

// Group is a set of lines that travel together and take one delivery choice. Kinds are the kinds every line in it allows and the channel offers
type Group struct {
	Key   string
	Kinds []string
	Lines []Line
}

// The key of the group a cart delivered as one uses - also the key when there is only one group, so a selection survives the shopper toggling split on a single-group cart
const CombinedGroupKey = "all"

// Plan is how a cart splits for delivery
type Plan struct {
	Need    Need
	Digital []Line
	Groups  []Group
	// Whether one option could cover every physical line - the storefront offers "deliver everything together" then
	Combinable bool
	// Lines no offered kind can deliver - checkout can't complete while any remain
	Undeliverable []Line
}

// PlanGroups splits a cart's lines - digital lines out, physical lines grouped by the kinds they allow, then merged into one group when the shopper hasn't asked to split and a common kind exists. A line whose product the profiles don't know is treated as physical with standard delivery, the safe default for a product deleted mid-checkout
func PlanGroups(lines []Line, profiles map[string]LineProfile, offered []string, split bool) Plan {
	var p Plan
	type bucket struct {
		kinds []string
		lines []Line
	}
	buckets := map[string]*bucket{}
	var order []string
	for _, l := range lines {
		prof, known := profiles[l.SKU]
		if known && !prof.RequiresShipping {
			p.Digital = append(p.Digital, l)
			continue
		}
		allowed := intersect([]string{KindShipToAddress, KindStorePickup, KindParcelLocker, KindCarrierPoint}, offered)
		if known && len(prof.DeliveryKinds) > 0 {
			allowed = intersect(prof.DeliveryKinds, offered)
		}
		if len(allowed) == 0 {
			p.Undeliverable = append(p.Undeliverable, l)
			continue
		}
		key := kindsKey(allowed)
		b, ok := buckets[key]
		if !ok {
			b = &bucket{kinds: allowed}
			buckets[key] = b
			order = append(order, key)
		}
		b.lines = append(b.lines, l)
	}
	physical := len(order) > 0 || len(p.Undeliverable) > 0
	switch {
	case physical && len(p.Digital) > 0:
		p.Need = NeedMixed
	case physical:
		p.Need = NeedPhysical
	default:
		p.Need = NeedNone
	}
	if len(order) == 0 {
		return p
	}
	common := buckets[order[0]].kinds
	for _, k := range order[1:] {
		common = intersect(common, buckets[k].kinds)
	}
	p.Combinable = len(common) > 0
	if len(order) == 1 || (p.Combinable && !split) {
		var all []Line
		for _, k := range order {
			all = append(all, buckets[k].lines...)
		}
		p.Groups = []Group{{Key: CombinedGroupKey, Kinds: common, Lines: all}}
		return p
	}
	for _, k := range order {
		b := buckets[k]
		p.Groups = append(p.Groups, Group{Key: k, Kinds: b.kinds, Lines: b.lines})
	}
	return p
}

// kindsKey names a group by the kinds it allows, so the same lines land in the same group across requests
func kindsKey(kinds []string) string {
	s := slices.Clone(kinds)
	sort.Strings(s)
	return "k:" + strings.Join(s, ",")
}

// intersect keeps a's order
func intersect(a, b []string) []string {
	out := make([]string, 0, len(a))
	for _, v := range a {
		if slices.Contains(b, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// Origin is where a shipment leaves from - an inventory location, or none when the store keeps no locations
type Origin struct {
	LocationID *uuid.UUID
	Name       string
	City       string
}

// Shipment is the lines of one group leaving one origin - each is quoted on its own and the group's price is their sum, the "packages by seller or ship-from address" model
type Shipment struct {
	GroupKey string
	Origin   Origin
	Lines    []Line
}

// SplitShipments cuts each group by the origin its lines are planned to leave from, keeping the order lines arrived in
func SplitShipments(groups []Group, origins map[uuid.UUID]Origin) []Shipment {
	var out []Shipment
	for _, g := range groups {
		idx := map[string]int{}
		for _, l := range g.Lines {
			o := origins[l.ID]
			key := ""
			if o.LocationID != nil {
				key = o.LocationID.String()
			}
			i, ok := idx[key]
			if !ok {
				i = len(out)
				idx[key] = i
				out = append(out, Shipment{GroupKey: g.Key, Origin: o})
			}
			out[i].Lines = append(out[i].Lines, l)
		}
	}
	return out
}

// GroupByKey finds a plan's group
func (p Plan) GroupByKey(key string) (Group, bool) {
	i := slices.IndexFunc(p.Groups, func(g Group) bool { return g.Key == key })
	if i < 0 {
		return Group{}, false
	}
	return p.Groups[i], true
}
