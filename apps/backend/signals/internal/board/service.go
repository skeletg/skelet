package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type Service struct{ r *repo }

func NewService(conn *sql.DB) *Service { return &Service{r: &repo{conn: conn, q: db.New(conn)}} }

// List is the shop's boards the actor can open, each with their access to it, their star and their draft. all widens
// a superuser's list to every board on the shop - without it they see what they'd see as themselves
func (s *Service) List(ctx context.Context, channelID uuid.UUID, a Actor, all bool) ([]Summary, error) {
	bs, err := s.r.list(ctx, channelID)
	if err != nil {
		return nil, err
	}
	shares, err := s.r.channelShares(ctx, channelID)
	if err != nil {
		return nil, err
	}
	favs, err := s.r.favourites(ctx, a.AdminID)
	if err != nil {
		return nil, err
	}
	drafts, err := s.r.draftTimes(ctx, a.AdminID)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(bs))
	for _, b := range bs {
		listedAs := a.asSelf()
		if all && a.Superuser {
			listedAs = a
		}
		if access(b.Ownership, shares[b.ID], listedAs) == NoAccess {
			continue
		}
		b.Access = access(b.Ownership, shares[b.ID], a)
		b.Favourite, b.DraftAt = favs[b.ID], drafts[b.ID]
		out = append(out, b)
	}
	return out, nil
}

// Get answers not-found for another shop's board, and for one the actor can't open, so an id alone says nothing
func (s *Service) Get(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID) (Board, error) {
	b, shares, err := s.load(ctx, channelID, a, id, View)
	if err != nil {
		return Board{}, err
	}
	if b.Access == Own {
		b.Shares = shares
	}
	favs, err := s.r.favourites(ctx, a.AdminID)
	if err != nil {
		return Board{}, err
	}
	b.Favourite = favs[b.ID]
	if b.Access >= Edit {
		if b.Draft, err = s.r.draft(ctx, b.ID, a.AdminID); err != nil {
			return Board{}, err
		}
		if b.Draft != nil {
			b.DraftAt = b.Draft.UpdatedAt
		}
	}
	return b, nil
}

// load reads a board with its shares and the actor's access, refusing below need - not found when they can't open it
func (s *Service) load(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID, need Access) (Board, []Share, error) {
	b, err := s.r.get(ctx, channelID, id)
	if err != nil {
		return Board{}, nil, err
	}
	shares, err := s.r.shares(ctx, id)
	if err != nil {
		return Board{}, nil, err
	}
	b.Access = access(b.Ownership, shares, a)
	if b.Access == NoAccess {
		return Board{}, nil, ErrNotFound
	}
	if b.Access < need {
		return Board{}, nil, ErrForbidden
	}
	return b, shares, nil
}

// Create starts a board as the actor's own, private until they share it
func (s *Service) Create(ctx context.Context, channelID uuid.UUID, a Actor, name string, reports []Report) (Board, error) {
	b, err := Board{ChannelID: channelID, Name: name, Reports: reports, OwnerID: a.AdminID}.normalized()
	if err != nil {
		return Board{}, err
	}
	b, err = s.r.create(ctx, b)
	b.Access = Own
	return b, err
}

// Save replaces a board's name and reports - readAt is the UpdatedAt the caller loaded, so two admins can't overwrite
// each other unseen. It clears the saver's draft
func (s *Service) Save(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID, name string, reports []Report, readAt time.Time) (Board, error) {
	if _, _, err := s.load(ctx, channelID, a, id, Edit); err != nil {
		return Board{}, err
	}
	b, err := Board{ID: id, ChannelID: channelID, Name: name, Reports: reports}.normalized()
	if err != nil {
		return Board{}, err
	}
	if err := s.r.save(ctx, b, readAt, a.AdminID); err != nil {
		return Board{}, err
	}
	return s.Get(ctx, channelID, a, id)
}

func (s *Service) Delete(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID) error {
	if _, _, err := s.load(ctx, channelID, a, id, Own); err != nil {
		return err
	}
	return s.r.delete(ctx, channelID, id)
}

// SetShares replaces who the board is shared with. A shop board is everyone's already, so it takes none
func (s *Service) SetShares(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID, shares []Share) (Board, error) {
	b, _, err := s.load(ctx, channelID, a, id, Own)
	if err != nil {
		return Board{}, err
	}
	switch {
	case b.OwnerID == uuid.Nil:
		return Board{}, fmt.Errorf("%w: a shop board is shared with everyone on the shop already", ErrInvalid)
	case len(shares) > maxShares:
		return Board{}, fmt.Errorf("%w: a board is shared with %d admins and roles at most", ErrInvalid, maxShares)
	}
	seen := map[[2]uuid.UUID]bool{}
	for _, sh := range shares {
		if (sh.AdminID == uuid.Nil) == (sh.RoleID == uuid.Nil) {
			return Board{}, fmt.Errorf("%w: a share names one admin or one role", ErrInvalid)
		}
		if sh.Level != ShareView && sh.Level != ShareEdit {
			return Board{}, fmt.Errorf("%w: a share is view or edit", ErrInvalid)
		}
		if sh.AdminID == b.OwnerID {
			return Board{}, fmt.Errorf("%w: the owner can't be shared their own board", ErrInvalid)
		}
		k := [2]uuid.UUID{sh.AdminID, sh.RoleID}
		if seen[k] {
			return Board{}, fmt.Errorf("%w: each admin or role is shared once", ErrInvalid)
		}
		seen[k] = true
	}
	if err := s.r.setShares(ctx, id, a.AdminID, shares); err != nil {
		return Board{}, err
	}
	return s.Get(ctx, channelID, a, id)
}

// Transfer hands the board to another admin, or with uuid.Nil makes it a shop board everyone on the shop sees and its
// managers run - which takes boards.manage. Only whoever owns it may: its owner, a superuser (an orphaned board is
// handed on this way), or a manager on a shop board. Core checks the new owner is an active admin
func (s *Service) Transfer(ctx context.Context, channelID uuid.UUID, a Actor, id, to uuid.UUID) error {
	if _, _, err := s.load(ctx, channelID, a, id, Own); err != nil {
		return err
	}
	if to == uuid.Nil && !a.CanManage && !a.Superuser {
		return ErrForbidden
	}
	return s.r.setOwner(ctx, channelID, id, to)
}

func (s *Service) SetFavourite(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID, on bool) error {
	if _, _, err := s.load(ctx, channelID, a, id, View); err != nil {
		return err
	}
	return s.r.setFavourite(ctx, a.AdminID, id, on)
}

// SaveDraft keeps the actor's staged edits between visits. The payload is the dashboard's own - it comes back on the
// next read and goes through Save, which checks it, so here it's only held to a size
func (s *Service) SaveDraft(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID, payload []byte, base time.Time) (time.Time, error) {
	if _, _, err := s.load(ctx, channelID, a, id, Edit); err != nil {
		return time.Time{}, err
	}
	if len(payload) > maxDraft || !json.Valid(payload) {
		return time.Time{}, fmt.Errorf("%w: a draft is a JSON document of %d bytes at most", ErrInvalid, maxDraft)
	}
	return s.r.putDraft(ctx, id, a.AdminID, payload, base)
}

func (s *Service) DiscardDraft(ctx context.Context, channelID uuid.UUID, a Actor, id uuid.UUID) error {
	if _, _, err := s.load(ctx, channelID, a, id, Edit); err != nil {
		return err
	}
	return s.r.discardDraft(ctx, id, a.AdminID)
}

// SweepDrafts drops unsaved edits nobody came back to within DraftKeep
func (s *Service) SweepDrafts(ctx context.Context, now time.Time) (int64, error) {
	return s.r.sweepDrafts(ctx, now.Add(-DraftKeep))
}

// CreateStarters adds the four starter boards to a shop that has none yet, and does nothing to one that does. They
// are shop boards: no owner, seen by everyone on the shop
func (s *Service) CreateStarters(ctx context.Context, channelID uuid.UUID, a Actor) ([]Summary, error) {
	// Starters are already tidy and valid - TestEveryStarterBoardIsValid holds them to it
	if err := s.r.createIfNone(ctx, channelID, Starters); err != nil {
		return nil, err
	}
	return s.List(ctx, channelID, a, false)
}

// Starters are the boards a new shop can begin from, one per area the seed fills
var Starters = []Board{
	{Name: "Shop health", Reports: []Report{
		{Title: "Page views", Definition: Definition{Kind: "insights", Event: "page_view", Measure: "total", Unit: "day"}},
		{Title: "Visitors", Definition: Definition{Kind: "insights", Event: "page_view", Measure: "unique", Unit: "day"}},
		{Title: "Visit to order", Definition: Definition{Kind: "funnel", Steps: []string{"page_view", "view_item", "cart_add", "begin_checkout", "order_placed"}}},
		{Title: "Returning shoppers", Definition: Definition{Kind: "stickiness", Event: "page_view"}},
	}},
	{Name: "Revenue & repeat", Reports: []Report{
		{Title: "Orders", Definition: Definition{Kind: "insights", Event: "order_placed", Measure: "total", Unit: "day"}},
		{Title: "Buyers", Definition: Definition{Kind: "insights", Event: "order_placed", Measure: "unique", Unit: "week"}},
		{Title: "Repeat orders", Definition: Definition{Kind: "retention", StartEvent: "order_placed", ReturnEvent: "order_placed", Unit: "week", Periods: 8}},
		{Title: "After an order", Definition: Definition{Kind: "flow", Event: "order_placed", Depth: 3}},
	}},
	{Name: "Product usage", Reports: []Report{
		{Title: "Product views", Definition: Definition{Kind: "insights", Event: "view_item", Measure: "total", Unit: "day"}},
		{Title: "Cart adds", Definition: Definition{Kind: "insights", Event: "cart_add", Measure: "total", Unit: "day"}},
		{Title: "View to cart", Definition: Definition{Kind: "funnel", Steps: []string{"view_item", "cart_add"}, WindowHours: 24}},
		{Title: "After a product view", Definition: Definition{Kind: "flow", Event: "view_item", Depth: 3}},
	}},
	{Name: "Email impact", Reports: []Report{
		{Title: "Emails opened", Definition: Definition{Kind: "insights", Event: "email_opened", Measure: "total", Unit: "week"}},
		{Title: "Email clicks", Definition: Definition{Kind: "insights", Event: "email_clicked", Measure: "total", Unit: "week"}},
		{Title: "Send to conversion", Definition: Definition{Kind: "funnel", Steps: []string{"email_sent", "email_opened", "email_clicked", "conversion"}, WindowHours: 24 * 7}},
		{Title: "Attributed orders", Definition: Definition{Kind: "insights", Event: "conversion", Measure: "total", Unit: "week"}},
	}},
}
