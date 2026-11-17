package plugin

import (
	"context"
	"errors"
	"fmt"
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
		if v != nil {
			out[r.Key()] = v
		}
	}
	return out, nil
}

// EraseCustomerData asks every compiled-in plugin to scrub what it keeps about an erased customer. Each is asked even
// when another failed, and the failures come back together so the erasure is tried again
func EraseCustomerData(ctx context.Context, customerID, channelID uuid.UUID, email string) error {
	who := sdk.CustomerDataSubject{CustomerID: customerID, ChannelID: channelID, Email: email}
	var errs []error
	for _, r := range sdk.CustomerDataSets() {
		if r.Data.Erase == nil {
			continue
		}
		if err := customerDataCall(ctx, func(ctx context.Context) error { return r.Data.Erase(ctx, who) }); err != nil {
			errs = append(errs, fmt.Errorf("plugin %s: erase %s: %w", r.PluginSlug, r.Data.Key, err))
		}
	}
	return errors.Join(errs...)
}

// Not Instrument - its kill switch would skip a plugin that's off, and the law doesn't
func customerDataCall(ctx context.Context, fn func(context.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, customerDataTimeout)
	defer cancel()
	defer func() {
		if rec := recover(); rec != nil {
			err = errFromPanic(rec)
		}
	}()
	return fn(ctx)
}
