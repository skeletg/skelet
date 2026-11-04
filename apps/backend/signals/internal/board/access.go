package board

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Access is what an admin may do with one board, each level including the ones below it
type Access int

const (
	NoAccess Access = iota
	View            // open it, star it, duplicate its reports
	Edit            // change its name, reports and subject
	Own             // share it, make it a shop board, delete it
)

func (a Access) String() string {
	return [...]string{"none", "view", "edit", "own"}[a]
}

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	ShareView       = "view"
	ShareEdit       = "edit"
	maxShares       = 100
	maxDraft        = 256 << 10
	// DraftKeep is how long unsaved edits wait for their admin to come back
	DraftKeep = 30 * 24 * time.Hour
)

// ErrForbidden is a board the caller can see but not change this way - one they can't see answers ErrNotFound
var ErrForbidden = errors.New("you can't do that on this board")

// Actor is the admin asking, as core's service token describes them. CanManage is boards.manage on the board's shop
type Actor struct {
	AdminID   uuid.UUID
	RoleIDs   []uuid.UUID
	Superuser bool
	CanManage bool
}

// Share gives one admin, or everyone holding a role, a level on a board - exactly one of AdminID and RoleID is set
type Share struct {
	AdminID uuid.UUID
	RoleID  uuid.UUID
	Level   string
}

// Draft is an admin's unsaved edits to a board, as the dashboard staged them
type Draft struct {
	Payload       []byte
	BaseUpdatedAt time.Time
	UpdatedAt     time.Time
}

// access is the one answer to "what may this admin do with this board" - every check in the service goes through it
//
//   - a draft is its owner's alone
//   - the owner owns it; a superuser can do anything
//   - a shop board (no owner) is everyone's on the shop to view, and theirs to manage with boards.manage
//   - otherwise the best of the shares to the admin or to a role they hold
func access(b Board, shares []Share, a Actor) Access {
	mine := b.OwnerID != uuid.Nil && b.OwnerID == a.AdminID
	if b.Status == StatusDraft {
		if mine || a.Superuser {
			return Own
		}
		return NoAccess
	}
	if mine || a.Superuser {
		return Own
	}
	if b.OwnerID == uuid.Nil {
		if a.CanManage {
			return Own
		}
		return View
	}
	best := NoAccess
	for _, s := range shares {
		if (s.AdminID != uuid.Nil && s.AdminID == a.AdminID) || (s.RoleID != uuid.Nil && slices.Contains(a.RoleIDs, s.RoleID)) {
			lvl := View
			if s.Level == ShareEdit {
				lvl = Edit
			}
			best = max(best, lvl)
		}
	}
	return best
}

// asSelf is the actor without their superuser bypass - what they'd see as themselves, which is what their own lists show
func (a Actor) asSelf() Actor {
	a.Superuser = false
	return a
}
