package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Long enough for a plugin to walk its own rows for one customer, short enough that a stuck one can't hold an export
const customerDataTimeout = 30 * time.Second

// CustomerDataExport is every plugin's section of a customer's data export, keyed "<plugin>:<key>" - asked of every
// compiled-in plugin, on or off, since what it kept outlives its being on. Any error fails the export
func CustomerDataExport(ctx context.Context, customerID, channelID uuid.UUID, email string) (map[string]any, error) {
	who := sdk.CustomerDataSubject{CustomerID: customerID, ChannelID: channelID, Email: email}
	out := map[string]any{}
	for _, r := range sdk.CustomerDataSets() {
		if r.Data.Export == nil {
			continue
		}
		var v any
		err := customerDataCall(ctx, func(ctx context.Context) (err error) {
			v, err = r.Data.Export(ctx, who)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("plugin %s: export %s: %w", r.PluginSlug, r.Data.Key, err)
		}
		// Marshalled here to find a section that won't go into the bundle while its plugin can still be named, and to
		// leave out one that comes to null - a nil slice in an `any` isn't nil
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: export %s doesn't read as JSON: %w", r.PluginSlug, r.Data.Key, err)
		}
		if string(raw) != "null" {
			out[r.Key()] = v
		}
	}
	return out, nil
}

// EraseCustomerData asks every compiled-in plugin to scrub what it keeps about a customer who's gone. Each is asked
// even when another failed, and the failures come back together
func EraseCustomerData(ctx context.Context, customerID, channelID uuid.UUID, email string) error {
	var errs []error
	for _, ref := range CustomerDataErasers() {
		if err := EraseCustomerDataSet(ctx, ref, customerID, channelID, email); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// One data set of one plugin - what an erasure is owed by
type CustomerDataRef struct{ Plugin, Key string }

// CustomerDataErasers is the data sets that have something to scrub when a customer goes
func CustomerDataErasers() []CustomerDataRef {
	var out []CustomerDataRef
	for _, r := range sdk.CustomerDataSets() {
		if r.Data.Erase != nil {
			out = append(out, CustomerDataRef{Plugin: r.PluginSlug, Key: r.Data.Key})
		}
	}
	return out
}

// ErrNoCustomerData is a data set no compiled-in plugin registers any more
var ErrNoCustomerData = errors.New("plugin: no such customer data set compiled in")

// EraseCustomerDataSet asks one plugin to scrub one of its data sets for a customer
func EraseCustomerDataSet(ctx context.Context, ref CustomerDataRef, customerID, channelID uuid.UUID, email string) error {
	who := sdk.CustomerDataSubject{CustomerID: customerID, ChannelID: channelID, Email: email}
	for _, r := range sdk.CustomerDataSets() {
		if r.PluginSlug != ref.Plugin || r.Data.Key != ref.Key || r.Data.Erase == nil {
			continue
		}
		if err := customerDataCall(ctx, func(ctx context.Context) error { return r.Data.Erase(ctx, who) }); err != nil {
			return fmt.Errorf("plugin %s: erase %s: %w", ref.Plugin, ref.Key, err)
		}
		return nil
	}
	return fmt.Errorf("%w: %s:%s", ErrNoCustomerData, ref.Plugin, ref.Key)
}

// Not Instrument - its kill switch would skip a plugin that's off, and the law doesn't
func customerDataCall(ctx context.Context, fn func(context.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, customerDataTimeout)
	defer cancel()
	defer func() {
		if rec := recover(); rec != nil {
			err = errFromPanic(rec)
			slog.ErrorContext(ctx, "plugin customer data hook panicked", "err", err, "stack", string(debug.Stack()))
		}
	}()
	return fn(ctx)
}
