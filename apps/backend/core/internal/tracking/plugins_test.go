package tracking

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestAPluginDestinationIsOfferedOnlyWhereItsPluginIsOn(t *testing.T) {
	ch, off := uuid.New(), uuid.New()
	var sent []string
	reg := []sdk.RegisteredDestination{{PluginSlug: "t-plugin", Destination: sdk.Destination{
		Kind: "t-plugin-kind", Label: "Test", Purposes: []sdk.DestinationPurpose{sdk.PurposeAnalytics},
		Send: func(_ context.Context, _ *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
			sent = append(sent, d.Secret+"/"+ev.ID)
			if ev.ID == "refused" {
				return sdk.Permanent(errors.New("400"))
			}
			return nil
		},
	}}}
	RegisterPluginKinds(reg, func(_ context.Context, slug string, channelID uuid.UUID) bool {
		return slug == "t-plugin" && channelID != off
	})
	t.Cleanup(func() { kindsMu.Lock(); delete(kinds, "t-plugin-kind"); kindsMu.Unlock() })

	k, ok := KindFor("t-plugin-kind")
	if !ok || k.Plugin != "t-plugin" || len(k.Purposes) != 1 || k.Purposes[0] != PurposeAnalytics {
		t.Fatalf("kind %+v", k)
	}
	on, offDest := uuid.New(), uuid.New()
	fs := newFakeStore(
		Destination{ID: on, ChannelID: ch, Kind: "t-plugin-kind", Enabled: true},
		Destination{ID: offDest, ChannelID: off, Kind: "t-plugin-kind", Enabled: true},
	)
	svc := &Service{store: fs}
	granted := consentOf("v2:a1.d0.p0")
	_ = svc.Track(context.Background(), Event{ID: "e1", Name: ViewItem, ChannelID: ch, Consent: granted})
	_ = svc.Track(context.Background(), Event{ID: "e2", Name: ViewItem, ChannelID: off, Consent: granted})
	if len(fs.byDestination(on)) != 1 || len(fs.byDestination(offDest)) != 0 {
		t.Error("an event reached a channel where the plugin is switched off")
	}

	err := k.Send(context.Background(), http.DefaultClient, Destination{Secret: "s"}, Event{ID: "refused"})
	if !isPermanent(err) || len(sent) != 1 || sent[0] != "s/refused" {
		t.Errorf("a plugin's permanent refusal must stay permanent: %v, %v", err, sent)
	}
}
