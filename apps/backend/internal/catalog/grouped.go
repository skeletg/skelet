package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type GroupedMember struct { // one SIMPLE product attached to a GROUPED parent - members render side-by-side and go into the cart individually as their own SKU (the parent is never purchasable), and a product can appear in multiple groups since UNIQUE is on the (parent, member) pair
	ID                     uuid.UUID
	ParentID               uuid.UUID
	MemberID               uuid.UUID
	DefaultQuantity        float64
	SortOrder              int32
	MemberURL              string
	MemberSKU              string
	MemberName             string
	MemberDefaultPrice     int64
	MemberCurrency         string
	MemberMinOrderQty      *float64 // a grouped add is one cart line per member, so each row is bounded by its own member's limits - nil = unbounded
	MemberMaxOrderQty      *float64
	MemberQuantityStep     float64
	MemberQuantityDecimals int16
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type AddGroupedMemberInput struct {
	ParentID        uuid.UUID
	MemberID        uuid.UUID
	DefaultQuantity float64
	SortOrder       int32
}

type UpdateGroupedMemberInput struct {
	ID              uuid.UUID
	DefaultQuantity *float64
	SortOrder       *int32
}

var (
	ErrGroupedParentNotGrouped = errors.New("grouped members can only be attached to products of kind GROUPED")
	ErrGroupedMemberNotSimple  = errors.New("grouped members must reference SIMPLE products")
	ErrGroupedMemberIsParent   = errors.New("a grouped product cannot include itself as a member")
	ErrGroupedMemberNotFound   = errors.New("grouped member not found")
	ErrGroupedMemberDuplicate  = errors.New("member is already attached to this group")
)
