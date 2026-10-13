package delivery

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Check is one line of an option's setup checklist - all must pass before the option can be enabled, except an Advisory one, which only warns. Tab names the checkout settings tab where it's fixed
type Check struct {
	Code     string
	Label    string
	OK       bool
	Detail   string
	Tab      string
	Advisory bool
}

// Readiness is everything that has to be true for checkout to offer the option: the kind is still registered, something quotes it, and its point source is live with points to pick
func (s *Service) Readiness(ctx context.Context, channelID uuid.UUID, o Option) []Check {
	k, ok := KindBySlug(o.Kind)
	if !ok {
		return []Check{{Code: "kind", Label: "Delivery kind is installed", Detail: o.Kind + " is no longer a delivery kind", Tab: "delivery"}}
	}
	out := []Check{{Code: "kind", Label: "Delivery kind is installed", OK: true, Detail: "Built in", Tab: "delivery"}}

	carrier := Check{Code: "carrier", Label: fmt.Sprintf("A carrier quotes %q for this channel", k.Label), Tab: "delivery"}
	if s.carriers != nil {
		byKind, err := s.carriers.CarrierKinds(ctx, channelID)
		names := byKind[k.Slug]
		if o.CarrierSlug != "" {
			names = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return !strings.HasPrefix(n, o.CarrierSlug+":") })
		}
		carrier.OK = err == nil && len(names) > 0
		carrier.Detail = carrierDetail(names)
		if !carrier.OK {
			carrier.Detail = "Add a shipping method of this kind, or enable a carrier that offers it, under Shipping"
		}
	} else {
		carrier.OK = true
	}
	out = append(out, carrier)

	if k.Needs(BlockPoint) {
		c := Check{Code: "point_source", Label: "Point source is active", Tab: "delivery"}
		if src, err := s.pointSourceFor(ctx, o); err == nil && src != nil {
			c.OK = true
			c.Detail = sourceName(o.PointSource, k)
		} else {
			c.Detail = "Choose where " + strings.ToLower(pointLabel(k)) + "s come from"
		}
		out = append(out, c)
		if c.OK && (o.PointSource == ManualPointSource || (o.PointSource == "" && defaultPointSource(k.Slug) == ManualPointSource)) {
			pts := Check{Code: "points", Label: "At least one pickup point is listed", Tab: "points"}
			n, err := s.repo.CountActivePoints(ctx, channelID, k.Slug)
			pts.OK = err == nil && n > 0
			if pts.OK {
				pts.Detail = fmt.Sprintf("%d listed", n)
			} else {
				pts.Detail = "Add pickup points on the Pickup points tab"
			}
			out = append(out, pts)
		}
	}
	if all, err := s.repo.ListOptions(ctx, channelID); err == nil {
		out = append(out, overlapCheck(o, all))
	}
	return out
}

// Ready is Readiness collapsed - advisory checks never hold an option back
func Ready(checks []Check) bool {
	for _, c := range checks {
		if !c.OK && !c.Advisory {
			return false
		}
	}
	return true
}

// overlapCheck warns when another enabled option of the same kind could take the same carrier's methods - checkout then lists each of those methods once per option. Naming a different carrier on each keeps them apart
func overlapCheck(o Option, all []Option) Check {
	c := Check{Code: "overlap", Label: "No other option offers the same methods", OK: true, Tab: "delivery", Advisory: true}
	var names []string
	for _, other := range Overlapping(o, all) {
		names = append(names, other.Name)
	}
	if len(names) > 0 {
		c.OK = false
		c.Detail = "Shoppers see these methods once under each of " + strings.Join(names, ", ") + " too - limit each option to a different carrier"
	}
	return c
}

// Overlapping are the enabled options of o's kind that can list a method o lists too - either names no carrier, or both name the same one
func Overlapping(o Option, all []Option) []Option {
	var out []Option
	for _, other := range all {
		if other.ID == o.ID || !other.Enabled || other.Kind != o.Kind {
			continue
		}
		if o.CarrierSlug == "" || other.CarrierSlug == "" || o.CarrierSlug == other.CarrierSlug {
			out = append(out, other)
		}
	}
	return out
}

func carrierDetail(names []string) string {
	labels := make([]string, 0, len(names))
	for _, n := range names {
		if _, label, ok := strings.Cut(n, ":"); ok {
			labels = append(labels, label)
			continue
		}
		labels = append(labels, n)
	}
	return strings.Join(slices.Compact(labels), ", ")
}

func pointLabel(k Kind) string {
	if k.PointLabel != "" {
		return k.PointLabel
	}
	return "Point"
}

func sourceName(slug string, k Kind) string {
	if slug == "" {
		slug = defaultPointSource(k.Slug)
	}
	if r, ok := pointSourceBySlug(slug); ok && r.Label != "" {
		return r.Label
	}
	return slug
}
