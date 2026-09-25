// Package segment is the customer-segments domain - an admin-defined bucket of customers like "VIP", "first-time buyers" or "lapsed for 90 days"
// Membership is materialised: static segments are hand-picked, rule segments recompute from rules.go on the refresh sweep, and behavioural segments keep their definition in the definition column
package segment

import (
	"encoding/json"
	"strings"
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
	Definition string
}

type Member struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	JoinedAt  time.Time
}

type Segment struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	Definition  string // a behavioural segment's JSON definition (behavioralsegment) - empty for static and rule segments
	// The dynamic-segment condition tree (rules.go) - nil for a static or trait segment
	Rules               json.RawMessage
	IsActive            bool
	LifecycleNewDays    *int32
	LifecycleActiveDays *int32
	LifecycleAtRiskDays *int32
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ChannelScope struct {
	All      bool
	Channels []uuid.UUID
}

// OwnerChannel is the channel a behavioral definition names, which owns the segment - false for a global one
func (s Segment) OwnerChannel() (uuid.UUID, bool) {
	if strings.TrimSpace(s.Definition) == "" {
		return uuid.Nil, false
	}
	var d struct {
		ChannelID string `json:"channelId"`
	}
	if json.Unmarshal([]byte(s.Definition), &d) != nil || d.ChannelID == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(d.ChannelID)
	if err != nil {
		return uuid.Nil, true // a channel no one can hold, so only a global grant reaches it
	}
	return id, true
}
