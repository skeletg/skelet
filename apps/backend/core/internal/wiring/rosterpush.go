package wiring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

// The 6-hourly sweep is too coarse for what a campaign must not get wrong - an unsubscribe four hours before a send, most of all
// Each of these re-reads the customer rather than trusting the payload, so a handler arriving out of order still lands the current row

type rosterResync interface {
	RunNowByName(ctx context.Context, name string) error
}

func registerRosterPush(bus *events.Bus, q *db.Queries, client *signalsclient.Client, jobs rosterResync) {
	push := func(ctx context.Context, customerID uuid.UUID) error {
		row, err := q.CustomerRosterEntry(ctx, customerID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // deleted between the event and this replay - the delete push is what carries that
		}
		if err != nil {
			return fmt.Errorf("read roster entry %s: %w", customerID, err)
		}
		behind, err := client.UpsertRosterCustomer(ctx, signalsclient.RosterCustomer{
			CustomerID: row.CustomerID, ChannelID: row.ChannelID, Email: row.Email,
			FirstName: row.FirstName, Phone: row.Phone, IsActive: row.IsActive,
			GroupID:          uuid.NullUUID{UUID: row.CustomerGroupID, Valid: row.CustomerGroupID != uuid.Nil},
			GroupSlug:        row.CustomerGroupSlug,
			Consents:         signalsclient.ConsentsFromJSON(row.Consents),
			MarketingOptedIn: row.MarketingOptedIn, SMSOptedIn: row.SmsOptedIn, ReadAt: row.ReadAt,
			OrderCount: row.OrderCount, MonetaryCents: row.MonetaryCents,
			FirstOrderAt: signalsclient.TimeFromAny(row.FirstOrderAt), LastOrderAt: signalsclient.TimeFromAny(row.LastOrderAt),
		})
		if err != nil {
			return err
		}
		if behind && jobs != nil {
			if err := jobs.RunNowByName(ctx, "customer-roster-push"); err != nil {
				log.Printf("[roster] full sync requested but could not be scheduled: %v", err)
			}
		}
		return nil
	}

	bus.Subscribe(customer.EventRegistered, func(ctx context.Context, e events.Event) error {
		return push(ctx, e.(*customer.CustomerRegistered).CustomerID)
	})
	bus.Subscribe(customer.EventUpdated, func(ctx context.Context, e events.Event) error {
		return push(ctx, e.(*customer.CustomerUpdated).CustomerID)
	})
	bus.Subscribe(customer.EventProfileUpdated, func(ctx context.Context, e events.Event) error {
		return push(ctx, e.(*customer.CustomerProfileUpdated).CustomerID)
	})
	bus.Subscribe(consent.EventChanged, func(ctx context.Context, e events.Event) error {
		return push(ctx, e.(*consent.ConsentChanged).CustomerID)
	})
	// A customer's lifetime order totals ride on their entry, so an order that counts or stops counting re-sends it
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error {
		if id := e.(*order.OrderPlaced).CustomerID; id != nil {
			return push(ctx, *id)
		}
		return nil
	})
	bus.Subscribe(order.EventCancelled, func(ctx context.Context, e events.Event) error {
		o, err := q.GetOrderByID(ctx, e.(*order.OrderCancelled).OrderID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !o.CustomerID.Valid) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read cancelled order: %w", err)
		}
		return push(ctx, o.CustomerID.UUID)
	})
	// The row is already gone, so there is nothing to re-read - the suite drops its copy and the
	// traits and memberships that hung off it
	bus.Subscribe(customer.EventDeleted, func(ctx context.Context, e events.Event) error {
		return client.DeleteRosterCustomer(ctx, e.(*customer.CustomerDeleted).CustomerID)
	})
}
