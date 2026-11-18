// Package pluginerase is what plugins still owe a customer who was erased or deleted: one row per plugin data set,
// asked again until that plugin's Erase answers nil. The event that announces the customer going only fills the
// queue - riding the event's own redelivery, a plugin failing for longer than the outbox keeps trying was never asked
// again, and every other subscriber of the event was replayed each time it failed
package pluginerase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
)

const (
	// How many erasures one pass asks for
	batch = 200
	// The wait after a first failure, doubling to the longest - a plugin's system down for a day is asked four times,
	// not a hundred and forty
	firstRetry   = 10 * time.Minute
	longestRetry = 6 * time.Hour
	// The most of a plugin's error the row keeps
	errorLimit = 500
)

// Enqueue records what every plugin has to scrub about the customer. Safe to repeat: a data set already queued for
// them stays as it is
func Enqueue(ctx context.Context, q *db.Queries, customerID, channelID uuid.UUID, email string) error {
	for _, ref := range pluginhost.CustomerDataErasers() {
		if err := q.EnqueuePluginCustomerErasure(ctx, db.EnqueuePluginCustomerErasureParams{
			CustomerID: customerID, ChannelID: channelID, Email: email, PluginSlug: ref.Plugin, DataKey: ref.Key,
		}); err != nil {
			return fmt.Errorf("queue %s:%s's erasure of customer %s: %w", ref.Plugin, ref.Key, customerID, err)
		}
	}
	return nil
}

// DrainCustomer asks for one customer's erasures now, due or not - most are gone on this first go, and what isn't
// stays queued for Drain
func DrainCustomer(ctx context.Context, q *db.Queries, customerID uuid.UUID, now time.Time) {
	rows, err := q.ListPluginCustomerErasuresForCustomer(ctx, customerID)
	if err != nil {
		slog.ErrorContext(ctx, "plugin erasures: reading a customer's queue", "customer", customerID, "err", err)
		return
	}
	for _, row := range rows {
		attempt(ctx, q, row, now)
	}
}

// Drain asks again for every erasure that's due. Nothing is ever dropped: the error is what's still owed, so the job
// calling this keeps failing, and alerting, for as long as a plugin holds data it was told to erase
func Drain(ctx context.Context, q *db.Queries, now time.Time) error {
	rows, err := q.ListDuePluginCustomerErasures(ctx, db.ListDuePluginCustomerErasuresParams{Now: now, Lim: batch})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		attempt(ctx, q, row, now)
	}
	owed, err := q.PendingPluginCustomerErasures(ctx)
	if err != nil {
		return err
	}
	if owed.Pending > 0 {
		return fmt.Errorf("%d plugin erasures still owed, the oldest since %s", owed.Pending, owed.Oldest.UTC().Format(time.RFC3339))
	}
	return nil
}

func attempt(ctx context.Context, q *db.Queries, row db.PluginCustomerErasure, now time.Time) {
	ref := pluginhost.CustomerDataRef{Plugin: row.PluginSlug, Key: row.DataKey}
	err := pluginhost.EraseCustomerDataSet(ctx, ref, row.CustomerID, row.ChannelID, row.Email)
	if err == nil {
		if err := q.DeletePluginCustomerErasure(ctx, row.ID); err != nil {
			slog.ErrorContext(ctx, "plugin erasures: clearing a finished one", "id", row.ID, "err", err)
		}
		return
	}
	level := slog.LevelWarn
	if errors.Is(err, pluginhost.ErrNoCustomerData) {
		level = slog.LevelError // the plugin left the build with data still owed - nothing here can erase it now
	}
	slog.Log(ctx, level, "plugin erasures: still owed", "plugin", row.PluginSlug, "key", row.DataKey, "customer", row.CustomerID, "attempts", row.Attempts+1, "err", err)
	reason := err.Error()
	if len(reason) > errorLimit {
		reason = strings.ToValidUTF8(reason[:errorLimit], "") // the cut can land inside a character
	}
	if err := q.FailPluginCustomerErasure(ctx, db.FailPluginCustomerErasureParams{
		ID: row.ID, LastError: reason, NextAttemptAt: now.Add(retryAfter(row.Attempts)),
	}); err != nil {
		slog.ErrorContext(ctx, "plugin erasures: recording a failure", "id", row.ID, "err", err)
	}
}

// How long to wait after the attempt that just failed, given how many failed before it
func retryAfter(failedBefore int32) time.Duration {
	wait := firstRetry
	for range failedBefore {
		if wait >= longestRetry {
			break
		}
		wait *= 2
	}
	return min(wait, longestRetry)
}
