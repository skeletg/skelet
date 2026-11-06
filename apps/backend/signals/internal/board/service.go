package board

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type Service struct{ r *repo }

func NewService(conn *sql.DB) *Service { return &Service{r: &repo{conn: conn, q: db.New(conn)}} }

func (s *Service) List(ctx context.Context, channelID uuid.UUID) ([]Summary, error) {
	return s.r.list(ctx, channelID)
}

// Get answers not-found for another shop's board too, so an id alone says nothing about other shops
func (s *Service) Get(ctx context.Context, channelID, id uuid.UUID) (Board, error) {
	return s.r.get(ctx, channelID, id)
}

func (s *Service) Create(ctx context.Context, channelID uuid.UUID, name string, reports []Report) (Board, error) {
	b, err := Board{ChannelID: channelID, Name: name, Reports: reports}.normalized()
	if err != nil {
		return Board{}, err
	}
	return s.r.create(ctx, b)
}

// Save replaces a board's name and reports - readAt is the UpdatedAt the caller loaded, so two admins can't overwrite each other unseen
func (s *Service) Save(ctx context.Context, channelID, id uuid.UUID, name string, reports []Report, readAt time.Time) (Board, error) {
	b, err := Board{ID: id, ChannelID: channelID, Name: name, Reports: reports}.normalized()
	if err != nil {
		return Board{}, err
	}
	if err := s.r.save(ctx, b, readAt); err != nil {
		return Board{}, err
	}
	return s.r.get(ctx, channelID, id)
}

func (s *Service) Delete(ctx context.Context, channelID, id uuid.UUID) error {
	return s.r.delete(ctx, channelID, id)
}

// CreateStarters adds the four starter boards to a shop that has none yet, and does nothing to one that does
func (s *Service) CreateStarters(ctx context.Context, channelID uuid.UUID) ([]Summary, error) {
	// Starters are already tidy and valid - TestEveryStarterBoardIsValid holds them to it
	if err := s.r.createIfNone(ctx, channelID, Starters); err != nil {
		return nil, err
	}
	return s.r.list(ctx, channelID)
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
