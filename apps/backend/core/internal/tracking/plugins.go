package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// RegisterPluginKinds turns every destination a compiled-in plugin registered into a kind, offered on a channel only
// where that plugin is enabled. Called once at boot
func RegisterPluginKinds(regs []sdk.RegisteredDestination, enabled func(ctx context.Context, slug string, channelID uuid.UUID) bool) {
	for _, reg := range regs {
		d, slug := reg.Destination, reg.PluginSlug
		purposes := make([]Purpose, len(d.Purposes))
		for i, p := range d.Purposes {
			purposes[i] = Purpose(p)
		}
		RegisterKind(Kind{
			Kind: d.Kind, Label: d.Label, Plugin: slug, Fields: d.Fields,
			Purposes: purposes, DefaultEvents: d.DefaultEvents, NecessarySetting: d.NecessarySetting,
			Available: func(ctx context.Context, channelID uuid.UUID) bool { return enabled(ctx, slug, channelID) },
			Send:      pluginSend(d.Send),
		})
	}
}

func pluginSend(send func(context.Context, *http.Client, sdk.DestinationConfig, sdk.ShopEvent) error) func(context.Context, *http.Client, Destination, Event) error {
	return func(ctx context.Context, client *http.Client, d Destination, ev Event) error {
		shop, err := ToShopEvent(ev)
		if err != nil {
			return Permanent(err)
		}
		err = send(ctx, client, sdk.DestinationConfig{ID: d.ID, ChannelID: d.ChannelID, Name: d.Name, Config: d.Config, Secret: d.Secret}, shop)
		if sdk.IsPermanent(err) {
			return Permanent(err)
		}
		return err
	}
}

// ToShopEvent is the event as the SDK spells it - both are the same JSON, which keeps the two types from drifting apart
func ToShopEvent(ev Event) (sdk.ShopEvent, error) {
	var out sdk.ShopEvent
	raw, err := json.Marshal(ev)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if err != nil {
		return sdk.ShopEvent{}, fmt.Errorf("event %s as the sdk's: %w", ev.ID, err)
	}
	return out, nil
}
