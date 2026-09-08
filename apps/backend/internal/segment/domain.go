// Package segment is the customer-segments domain - an admin-defined bucket of customers like "VIP", "first-time buyers" or "lapsed for 90 days"
// v2 scope, not built yet: a definition language (the refresh_sql column is reserved for it), an auto-refresh scheduler, and materialised-view backing
package segment

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type MemberSegment struct {
	ID   uuid.UUID
	Name string
	Slug string
}

// One segment a customer belongs to, carrying the two columns that say whether Signals maintains it
type Membership struct {
	Slug       string
	Rules      json.RawMessage
	RefreshSQL string
}

type Member struct {
	ID        uuid.UUID
	Email     string
	FirstName string
	LastName  string
	ChannelID uuid.UUID
	JoinedAt  time.Time
}

type Segment struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	RefreshSQL  string // placeholder - Rules is the live definition
	// The dynamic-segment condition tree (rules.go) - nil for a static or trait segment
	Rules               json.RawMessage
	IsActive            bool
	LifecycleNewDays    *int32
	LifecycleActiveDays *int32
	LifecycleAtRiskDays *int32
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
