package tracking

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestCatalogueQueueUsesEncryptedAccountsAndIndependentDatabaseClaims(t *testing.T) {
	conn := testdb.Require(t)
	q := db.New(conn)
	ctx := context.Background()
	ch := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels(id,slug,name,currency_code) VALUES($1,$2,'Catalogue queue','USD')`, ch, "catalog-"+ch.String()); err != nil {
		t.Fatal(err)
	}
	registerTestKind(t, Kind{Kind: "catalog-db", Catalog: true})
	registerTestKind(t, Kind{Kind: "event-db"})
	cipher, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(q, cipher, nil)
	d, err := svc.Create(ctx, Destination{ChannelID: ch, Kind: "catalog-db", Name: "Merchant", Enabled: true, Secret: "private-credential"})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := q.GetTrackingDestination(ctx, d.ID)
	if err != nil || !secrets.IsEncrypted(stored.Secret) {
		t.Fatalf("credential not encrypted: %v", err)
	}
	if _, err := svc.Create(ctx, Destination{ChannelID: ch, Kind: "event-db", Name: "Analytics", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Track(ctx, Event{ID: "order", Name: Purchase, ChannelID: ch, User: User{Email: "private@example.com"}}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.QueueCatalog(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.Deliveries(ctx, d.ID, 20)
	if err != nil || len(rows) != 1 {
		t.Fatalf("coalesced deliveries: %d, %v", len(rows), err)
	}
	if string(rows[0].Payload) != "{}" {
		t.Fatalf("catalogue job stored account or shopper data: %s", rows[0].Payload)
	}
	events, err := q.ClaimDueTrackingDeliveries(ctx, db.ClaimDueTrackingDeliveriesParams{BatchSize: 8, LeaseSeconds: 60, Catalog: false})
	if err != nil || len(events) != 1 || events[0].EventName != Purchase {
		t.Fatalf("event claims: %+v, %v", events, err)
	}
	catalogs, err := q.ClaimDueTrackingDeliveries(ctx, db.ClaimDueTrackingDeliveriesParams{BatchSize: 2, LeaseSeconds: 1860, Catalog: true})
	if err != nil || len(catalogs) != 1 || catalogs[0].EventName != CatalogSnapshot {
		t.Fatalf("catalogue claims: %+v, %v", catalogs, err)
	}
	if err := svc.QueueCatalog(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := svc.Deliveries(ctx, d.ID, 20); err != nil || len(rows) != 1 {
		t.Fatal("an active lease did not coalesce refreshes")
	}
	if err := q.MarkTrackingDeliverySent(ctx, catalogs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.QueueCatalog(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := svc.Deliveries(ctx, d.ID, 20); err != nil || len(rows) != 2 {
		t.Fatal("a successful upload prevented the next refresh")
	}
}
