package tracking

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCatalogueDeliveryCoalescesRetriesAndNeverReceivesShopperEvents(t *testing.T) {
	ctx := context.Background()
	ch, account, eventAccount := uuid.New(), uuid.New(), uuid.New()
	catalogCalls, eventCalls := 0, 0
	registerTestKind(t, Kind{Kind: "test-catalog", Catalog: true, SendCatalog: func(_ context.Context, d Destination) error {
		catalogCalls++
		if catalogCalls == 1 {
			return errors.New("temporary refusal: " + d.Secret)
		}
		return nil
	}})
	registerTestKind(t, Kind{Kind: "test-event", Send: func(context.Context, *http.Client, Destination, Event) error { eventCalls++; return nil }})
	fs := newFakeStore(Destination{ID: account, ChannelID: ch, Kind: "test-catalog", Enabled: true, Secret: "catalog-secret"}, Destination{ID: eventAccount, ChannelID: ch, Kind: "test-event", Enabled: true})
	svc := &Service{store: fs}
	if err := svc.Track(ctx, Event{ID: "order", Name: Purchase, ChannelID: ch, User: User{Email: "private@example.com"}}); err != nil {
		t.Fatal(err)
	}
	if len(fs.byDestination(account)) != 0 {
		t.Fatal("catalogue received a shopper event")
	}
	for range 2 {
		if err := svc.QueueCatalog(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	if len(fs.byDestination(account)) != 1 {
		t.Fatal("pending refreshes did not coalesce")
	}
	NewWorker(svc, http.DefaultClient).RunOnce(ctx)
	if eventCalls != 1 || catalogCalls != 0 {
		t.Fatal("event worker claimed a catalogue")
	}
	worker := NewCatalogWorker(svc)
	worker.RunOnce(ctx)
	row := fs.byDestination(account)[0]
	if row.Status != "pending" || row.Attempts != 1 || strings.Contains(row.LastError, "catalog-secret") {
		t.Fatalf("failed catalogue: %+v", row)
	}
	d := fs.dests[account]
	d.Enabled = false
	fs.dests[account] = d
	_ = fs.set(row.ID, func(d *Delivery) { d.NextAttemptAt = time.Time{} })
	worker.RunOnce(ctx)
	if fs.byDestination(account)[0].Status != "dead" || catalogCalls != 1 {
		t.Fatal("disabled account uploaded its catalogue")
	}
	d.Enabled = true
	fs.dests[account] = d
	if err := svc.Redeliver(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	worker.RunOnce(ctx)
	if got := fs.byDestination(account)[0]; got.Status != "sent" || catalogCalls != 2 {
		t.Fatalf("manual retry: %+v", got)
	}
}

func TestPanickingCatalogueSenderReleasesItsAccountAfterBoundedRetries(t *testing.T) {
	ctx := context.Background()
	account := uuid.New()
	registerTestKind(t, Kind{Kind: "panic-catalog", Catalog: true, SendCatalog: func(context.Context, Destination) error { panic("broken adapter") }})
	fs := newFakeStore(Destination{ID: account, ChannelID: uuid.New(), Kind: "panic-catalog", Enabled: true})
	svc := &Service{store: fs}
	if err := svc.QueueCatalog(ctx, account); err != nil {
		t.Fatal(err)
	}
	w := NewCatalogWorker(svc)
	w.RunOnce(ctx)
	row := fs.byDestination(account)[0]
	if row.Status != "pending" || row.LastError != "destination sender panicked" {
		t.Fatalf("panic bypassed retry handling: %+v", row)
	}
	_ = fs.set(row.ID, func(d *Delivery) { d.Attempts = maxAttempts - 1; d.NextAttemptAt = time.Time{} })
	w.RunOnce(ctx)
	if got := fs.byDestination(account)[0]; got.Status != "dead" || got.Attempts != maxAttempts {
		t.Fatalf("panicking adapter blocked account indefinitely: %+v", got)
	}
	if err := svc.QueueCatalog(ctx, account); err != nil {
		t.Fatal(err)
	}
	if len(fs.byDestination(account)) != 2 {
		t.Fatal("dead delivery still prevents the next snapshot")
	}
}
