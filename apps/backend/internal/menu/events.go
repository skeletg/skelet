package menu

import "github.com/google/uuid"

const (
	EventMenuUpdated = "menu.updated"
)

// Fires AFTER any structural mutation to a menu - create, update, item add, remove or reorder, store assignment - Async-only
type MenuUpdated struct {
	MenuID uuid.UUID
	Slug   string
}

func (*MenuUpdated) Type() string { return EventMenuUpdated }
