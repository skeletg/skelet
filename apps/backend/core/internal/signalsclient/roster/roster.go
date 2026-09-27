// Package roster keeps the suite's copy of core's customers and channels current - the pushes between the six-hourly sweeps, and the channel list the sweep sends whole
package roster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The 6-hourly sweep is too coarse for what a campaign must not get wrong - an unsubscribe four hours before a send, most of all
// Each of these re-reads the customer rather than trusting the payload, so a handler arriving out of order still lands the current row

type Resync interface {
	RunNowByName(ctx context.Context, name string) error
}

func Register(bus *events.Bus, q *db.Queries, client *signalsclient.Client, jobs Resync) {
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

// PushChannels hands the suite every channel core has, flagged active or not - one left out reads as deleted there, and takes its Pulse content with it
// The identity fields come from the config registry, so a message sent from the suite still looks like it came from the shop
func PushChannels(ctx context.Context, chans *channel.Service, cfg *config.Service, client *signalsclient.Client) error {
	all, err := chans.List(ctx)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return nil // a shop always has its default channel, so an empty read is a fault, and pushing it would wipe every channel's content
	}
	entries := make([]signalsclient.ChannelEntry, 0, len(all))
	for _, ch := range all {
		id := ch.ID
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
		entry := signalsclient.ChannelEntry{ID: ch.ID, IsDefault: ch.IsDefault, IsActive: ch.IsActive, Name: ch.Name, PublicURL: ch.PublicURL}
		if cfg != nil {
			// general/branding/store_name is the customer-facing name a mail prints; channels.name is the
			// operator's internal label ("US Wholesale B2B"), which is what the setting exists to keep out of one
			if name, _ := cfg.GetString(ctx, "general/branding/store_name", scope); strings.TrimSpace(name) != "" {
				entry.Name = strings.TrimSpace(name)
			}
			entry.FromAddress, _ = cfg.GetString(ctx, "general/contact/email_from_address", scope)
			entry.FromName, _ = cfg.GetString(ctx, "general/contact/email_from_name", scope)
			entry.LogoURL, _ = cfg.GetString(ctx, "general/branding/logo_url", scope)
		}
		entries = append(entries, entry)
	}
	if _, _, err := client.PushChannels(ctx, entries); err != nil {
		return err
	}
	return nil
}

// A roster migration on the suite's side leaves every row short a column until a full sweep lands - asked once at boot, so a deploy fixes itself instead of waiting for the six-hourly job
func ResyncIfBehind(ctx context.Context, client *signalsclient.Client, jobs Resync) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	behind, err := client.RosterNeedsFullSync(ctx)
	if err != nil {
		log.Printf("[roster] status: %v", err)
		return
	}
	if !behind {
		return
	}
	if err := jobs.RunNowByName(ctx, "customer-roster-push"); err != nil {
		log.Printf("[roster] full sync requested but could not be scheduled: %v", err)
	}
}
