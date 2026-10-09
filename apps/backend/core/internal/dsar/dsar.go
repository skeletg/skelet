// Package dsar assembles a customer's data-subject access bundle - one builder behind both the storefront's self-serve
// export and the admin's, so the two can't drift apart
package dsar

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type Customers interface {
	ExportData(ctx context.Context, customerID uuid.UUID) (customer.DataExport, error)
}

type Orders interface {
	ListOrdersByCustomer(ctx context.Context, customerID uuid.UUID) ([]order.Order, error)
}

type Consents interface {
	ListForCustomer(ctx context.Context, customerID uuid.UUID) ([]consent.Consent, error)
	History(ctx context.Context, customerID uuid.UUID, limit int32) ([]consent.Event, error)
}

type Channels interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

type StoreCredit interface {
	Balance(ctx context.Context, customerID uuid.UUID, currency string) (int64, error)
}

type Wishlist interface {
	ListIDs(ctx context.Context, customerID uuid.UUID) ([]uuid.UUID, error)
}

type Builder struct {
	customers Customers
	orders    Orders
	consents  Consents
	channels  Channels
	credit    StoreCredit
	wishlist  Wishlist
	q         *db.Queries
}

func New(customers Customers, orders Orders, consents Consents, channels Channels, credit StoreCredit, wishlist Wishlist, q *db.Queries) *Builder {
	return &Builder{customers: customers, orders: orders, consents: consents, channels: channels, credit: credit, wishlist: wishlist, q: q}
}

// The admin's export - the admin vouches for the requester, so rows a guest left under the customer's address come too
// Every section is the whole set and any read failure fails the export - a bundle missing a section, or cut off at
// some row count, answers the request wrongly while looking complete
func (b *Builder) Build(ctx context.Context, customerID uuid.UUID) (customer.DataExport, error) {
	return b.build(ctx, customerID, true)
}

// The storefront's export - address-matched rows only once the customer proved the address is theirs, since
// signing in doesn't require it and anyone could otherwise register a stranger's address and export their guest rows
func (b *Builder) BuildSelfServe(ctx context.Context, customerID uuid.UUID) (customer.DataExport, error) {
	return b.build(ctx, customerID, false)
}

func (b *Builder) build(ctx context.Context, customerID uuid.UUID, vouched bool) (customer.DataExport, error) {
	bundle, err := b.customers.ExportData(ctx, customerID)
	if err != nil {
		return customer.DataExport{}, err
	}
	c := bundle.Profile
	ch, err := b.channels.GetByID(ctx, c.ChannelID)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export channel: %w", err)
	}
	orders, err := b.orders.ListOrdersByCustomer(ctx, customerID)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export orders: %w", err)
	}
	if bundle.Orders, err = json.Marshal(orders); err != nil {
		return customer.DataExport{}, fmt.Errorf("export orders: %w", err)
	}
	consents, err := b.consents.ListForCustomer(ctx, customerID)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export consents: %w", err)
	}
	if bundle.Consents, err = json.Marshal(consents); err != nil {
		return customer.DataExport{}, fmt.Errorf("export consents: %w", err)
	}
	history, err := b.consents.History(ctx, customerID, 0)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export consent history: %w", err)
	}
	balance, err := b.credit.Balance(ctx, customerID, ch.CurrencyCode)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export store credit balance: %w", err)
	}
	wished, err := b.wishlist.ListIDs(ctx, customerID)
	if err != nil {
		return customer.DataExport{}, fmt.Errorf("export wishlist: %w", err)
	}
	extra := map[string]any{
		"consentHistory":     history,
		"storeCreditBalance": map[string]any{"amountCents": balance, "currency": ch.CurrencyCode},
		"wishlistProductIds": wished,
	}
	addr := ""
	if vouched || c.IsEmailVerified {
		addr = customer.NormalizeEmail(c.Email)
	}
	if err := b.sections(ctx, c.ID, c.ChannelID, addr, extra); err != nil {
		return customer.DataExport{}, err
	}
	bundle.Extra = extra
	return bundle, nil
}

// The rows outside the customer domain, each read as one JSON array
func (b *Builder) sections(ctx context.Context, id, ch uuid.UUID, addr string, extra map[string]any) error {
	nid := uuid.NullUUID{UUID: id, Valid: true}
	for _, s := range []struct {
		key  string
		read func() (json.RawMessage, error)
	}{
		{"storeCredit", func() (json.RawMessage, error) { return b.q.DSARStoreCreditEntries(ctx, id) }},
		{"loyaltyPoints", func() (json.RawMessage, error) { return b.q.DSARLoyaltyEntries(ctx, id) }},
		{"reviews", func() (json.RawMessage, error) {
			return b.q.DSARReviews(ctx, db.DSARReviewsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
		{"productQuestions", func() (json.RawMessage, error) {
			return b.q.DSARProductQuestions(ctx, db.DSARProductQuestionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
		{"subscriptions", func() (json.RawMessage, error) { return b.q.DSARSubscriptions(ctx, nid) }},
		{"giftCards", func() (json.RawMessage, error) {
			return b.q.DSARGiftCards(ctx, db.DSARGiftCardsParams{CustomerID: id, ChannelID: uuid.NullUUID{UUID: ch, Valid: true}, Email: addr})
		}},
		{"savedPaymentMethods", func() (json.RawMessage, error) { return b.q.DSARPaymentMethods(ctx, id) }},
		{"carts", func() (json.RawMessage, error) { return b.q.DSARCarts(ctx, nid) }},
		{"downloadGrants", func() (json.RawMessage, error) {
			return b.q.DSARDownloadGrants(ctx, db.DSARDownloadGrantsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
		{"backInStockRequests", func() (json.RawMessage, error) {
			return b.q.DSARBackInStockRequests(ctx, db.DSARBackInStockRequestsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
		{"signupFormSubmissions", func() (json.RawMessage, error) {
			return b.q.DSARSignupSubmissions(ctx, db.DSARSignupSubmissionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
		{"webFormSubmissions", func() (json.RawMessage, error) {
			return b.q.DSARWebFormSubmissions(ctx, db.DSARWebFormSubmissionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
		}},
	} {
		doc, err := s.read()
		if err != nil {
			return fmt.Errorf("export %s: %w", s.key, err)
		}
		extra[s.key] = doc
	}
	return nil
}
