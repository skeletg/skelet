// Package notification is the admin inbox, a curated store-ops notification center
// Reads are shared but scoped per admin by permission and channel, with per-admin read state
package notification

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

const (
	CategoryReturn    = "return"
	CategoryReview    = "review"
	CategoryInventory = "inventory"
	CategoryPayment   = "payment"
	CategoryOrder     = "order"
	CategorySystem    = "system"
	CategoryForm      = "form"
)

// Permission slugs gating visibility - producers stamp one onto each notification, and the read API checks which the acting admin holds - A bounded set, so it's a few HasPermission checks
const (
	PermOrders    = "orders.view"
	PermReviews   = "reviews.view"
	PermInventory = "inventory.view"
	PermSystem    = "scheduler.view"
	PermForms     = "webforms.submissions_view"
)

var CategoryPermissions = []string{PermOrders, PermReviews, PermInventory, PermSystem, PermForms}

// Per-admin preference key holding a JSON array of muted categories - a personal choice, so it is applied on top of the permission scope rather than being part of it, and it applies to superusers too. Deliberately plain strings: category is free text in the DB with no enum, so an unknown value simply matches nothing
const MutedCategoriesPref = "notif:mutedCategories"

type Input struct {
	Category           string
	Severity           string
	Title              string
	Body               string
	RequiredPermission string
	ChannelID          *uuid.UUID // nil = global / system
	EntityType         string
	EntityID           string
	ActionURL          string
	DedupeKey          string
}

type Notification struct {
	ID         uuid.UUID
	Category   string
	Severity   string
	Title      string
	Body       string
	ChannelID  *uuid.UUID
	EntityType string
	EntityID   string
	ActionURL  string
	CreatedAt  time.Time
	Read       bool
	// Populated on the live-push path only, so subscription fan-out can apply Scope.CanSee without a DB round trip - the List read model leaves it empty, since its SQL already filters
	RequiredPermission string `json:",omitempty"`
}

// The acting admin's visibility filter, resolved once per request from their permissions and permitted channels - Bypass short-circuits both filters, so superusers and all-channel admins see everything
type Scope struct {
	AdminID      uuid.UUID
	Bypass       bool
	Perms        []string
	GlobalPerms  []string
	PermChannels map[string][]uuid.UUID
	Muted        []string // categories this admin muted - see MutedCategoriesPref
}

func (sc Scope) permChannelPairs() []string {
	var out []string
	for perm, chs := range sc.PermChannels {
		for _, ch := range chs {
			out = append(out, perm+"|"+ch.String())
		}
	}
	return out
}

// Whether a notification is visible under this scope - the in-memory mirror of ListNotifications' SQL filter, for the live subscription fan-out, which can't route every push through a DB query
func (sc Scope) CanSee(n Notification) bool {
	// Before the bypass: muting is a preference, not a permission, so it holds for superusers too -
	// and the badge is counted in SQL with the same predicate, or a mute would leave it lying
	if slices.Contains(sc.Muted, n.Category) {
		return false
	}
	if sc.Bypass {
		return true
	}
	permOK := slices.Contains(sc.Perms, n.RequiredPermission)
	if !permOK {
		return false
	}
	if n.ChannelID == nil {
		return true // global / system notice
	}
	if slices.Contains(sc.GlobalPerms, n.RequiredPermission) {
		return true
	}
	return slices.Contains(sc.PermChannels[n.RequiredPermission], *n.ChannelID)
}
