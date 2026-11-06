package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A channel's tracking is its own: an admin of one channel can't read, change, test or retry another's, and no answer
// ever carries a destination's secret
func TestTrackingDestinationsStayInTheirChannel(t *testing.T) {
	conn := testdb.Require(t)
	q := db.New(conn)
	ctx := context.Background()
	chans := channel.NewService(channel.NewRepository(q, txrunner.New(conn)))
	suffix := uuid.NewString()[:8]
	mk := func(slug string) uuid.UUID {
		ch, err := chans.Create(ctx, channel.CreateChannelInput{Slug: slug + "-" + suffix, Name: slug, CurrencyCode: "EUR", PublicURL: "https://" + slug + "-" + suffix + ".test"})
		if err != nil {
			t.Fatal(err)
		}
		return ch.ID
	}
	chA, chB := mk("trk-a"), mk("trk-b")

	kind := "t-authz-" + suffix
	dest := sdk.Destination{Kind: kind, Label: "Test", Send: func(context.Context, *http.Client, sdk.DestinationConfig, tracking.Event) error { return nil },
		Fields: []sdk.DestinationField{{Key: "account", Label: "Account", Required: true}, {Key: "api_secret", Label: "Secret", Required: true, Secret: true}}}
	tracking.RegisterKind(tracking.Kind{Destination: dest})
	svc := tracking.NewService(q, nil, nil)
	const secret = "super-secret-value-123"
	inB, err := svc.Create(ctx, tracking.Destination{ChannelID: chB, Kind: kind, Name: "B", Config: map[string]string{"account": "b"}, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.EnqueueTrackingEvent(ctx, db.EnqueueTrackingEventParams{DestinationIds: []uuid.UUID{inB.ID}, ChannelID: chB, EventID: "e1", EventName: "purchase", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	logB, err := svc.Deliveries(ctx, inB.ID, "", 10)
	if err != nil || len(logB) != 1 {
		t.Fatal(err)
	}
	if err := q.MarkTrackingDeliveryDead(ctx, db.MarkTrackingDeliveryDeadParams{ID: logB[0].ID, LastError: "gave up"}); err != nil {
		t.Fatal(err)
	}

	base := &Resolver{TrackingSvc: svc, TrackingClient: http.DefaultClient}
	qr, mr := &queryResolver{base}, &mutationResolver{base}
	viewA := adminWithCachedGrants(auth.Grant{Slug: "tracking.view", ChannelID: &chA})
	manageA := adminWithCachedGrants(auth.Grant{Slug: "tracking.manage", ChannelID: &chA})
	manageB := adminWithCachedGrants(auth.Grant{Slug: "tracking.manage", ChannelID: &chB})
	enabled := true
	input := model.TrackingDestinationInput{Name: "A", Enabled: &enabled, Config: []*model.TrackingConfigValueInput{{Key: "account", Value: "a"}, {Key: "api_secret", Value: secret}, {Key: "smuggled", Value: "x"}}}

	if _, err := qr.TrackingDestinations(viewA, chB.String()); err == nil {
		t.Error("a channel-A admin listed channel B's destinations")
	}
	if _, err := qr.TrackingDestination(viewA, inB.ID.String()); err == nil {
		t.Error("a channel-A admin read a channel-B destination by id")
	}
	if _, err := qr.TrackingDeliveries(viewA, inB.ID.String(), nil, nil); err == nil {
		t.Error("a channel-A admin read channel B's delivery log")
	}
	if _, err := mr.UpdateTrackingDestination(manageA, inB.ID.String(), input); err == nil {
		t.Error("a channel-A admin changed a channel-B destination")
	}
	if _, err := mr.SendTrackingTest(manageA, inB.ID.String()); err == nil {
		t.Error("a channel-A admin test-sent through a channel-B destination")
	}
	if _, err := mr.RedeliverTrackingDelivery(manageA, logB[0].ID.String()); err == nil {
		t.Error("a channel-A admin retried a channel-B delivery")
	}
	if _, err := mr.DeleteTrackingDestination(manageA, inB.ID.String()); err == nil {
		t.Error("a channel-A admin deleted a channel-B destination")
	}
	if _, err := mr.CreateTrackingDestination(viewA, chA.String(), kind, input); err == nil {
		t.Error("view permission created a destination")
	}

	created, err := mr.CreateTrackingDestination(manageA, chA.String(), kind, input)
	if err != nil || created.ChannelID != chA.String() || !created.SecretSet {
		t.Fatalf("a channel-A admin setting up their own: %+v %v", created, err)
	}
	for _, c := range created.Config {
		if c.Key == "smuggled" || c.Key == "api_secret" {
			t.Errorf("config carried %q", c.Key)
		}
	}
	listed, err := qr.TrackingDestinations(viewA, chA.String())
	if err != nil || len(listed) != 1 {
		t.Fatalf("listing channel A: %v %v", listed, err)
	}
	got, _ := qr.TrackingDestination(manageB, inB.ID.String())
	retried, err := mr.RedeliverTrackingDelivery(manageB, logB[0].ID.String())
	if err != nil || retried.Status != model.TrackingDeliveryStatusPending {
		t.Errorf("channel B's own admin retrying its dead delivery: %+v %v", retried, err)
	}
	if _, err := mr.RedeliverTrackingDelivery(manageB, logB[0].ID.String()); err == nil {
		t.Error("retrying a delivery that is no longer dead should say so")
	}
	for _, v := range []any{created, listed, got} {
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), secret) {
			t.Errorf("a secret came back: %s", raw)
		}
	}
}
