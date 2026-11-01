package wiring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func newProductFeedExporter(q *db.Queries, storage objstore.Storage, inv *inventory.Service, cat *catalog.Service) *catalogio.ProductFeedExporter {
	return catalogio.NewProductFeedExporter(q, storage, inv, cat)
}

func catalogSender(channels *channel.Service, exporter *catalogio.ProductFeedExporter, client sdk.ProductFeedClient) func(context.Context, tracking.Destination, sdk.ProductFeed) error {
	return func(ctx context.Context, d tracking.Destination, feed sdk.ProductFeed) error {
		ch, err := channels.GetByID(ctx, d.ChannelID)
		if err != nil {
			return err
		}
		if !ch.IsActive || ch.PublicURL == "" {
			return sdk.Permanent(fmt.Errorf("catalogue channel is inactive or has no public URL"))
		}
		cfg := sdk.DestinationConfig{ID: d.ID, ChannelID: d.ChannelID, Name: d.Name, Config: d.Config, Secret: d.Secret}
		result, err := exporter.Export(ctx, feed, catalogio.MerchantFeedOptions{
			ChannelID: ch.ID, BaseURL: ch.PublicURL, CurrencyCode: ch.CurrencyCode, Locale: ch.DefaultLocale, Title: ch.Name,
		}, cfg)
		if err != nil {
			return err
		}
		slog.Info("catalogue rendered", "destination", d.ID, "exported", result.Exported, "skipped", result.Skipped)
		return feed.Send(ctx, client, cfg, result.File)
	}
}

// Only scheduling lives here. Each account is an independent durable delivery, so a failed
// upload neither holds up another account nor makes the scheduler retry completed uploads.
func refreshProductFeed(ctx context.Context, reg sdk.RegisteredProductFeed, channels *channel.Service, svc *tracking.Service) error {
	chs, err := channels.List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, ch := range chs {
		if !ch.IsActive || ch.PublicURL == "" || !pluginhost.Enabled(ctx, reg.PluginSlug, ch.ID) {
			continue
		}
		dests, err := svc.List(ctx, ch.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, d := range dests {
			if d.Enabled && d.Kind == tracking.CatalogKind(reg.Feed.Kind) {
				if err := svc.QueueCatalog(ctx, d.ID); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func productFeedHandler(reg sdk.RegisteredProductFeed, svc *tracking.Service, exporter *catalogio.ProductFeedExporter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ch, ok := channel.FromContext(r.Context())
		if !ok || ch == nil || !ch.IsActive || ch.PublicURL == "" || !pluginhost.Enabled(r.Context(), reg.PluginSlug, ch.ID) {
			http.NotFound(w, r)
			return
		}
		wanted := uuid.Nil
		raw := r.URL.Query().Get("destination")
		if raw != "" {
			var err error
			wanted, err = uuid.Parse(raw)
			if err != nil {
				http.Error(w, "invalid destination", http.StatusBadRequest)
				return
			}
		}
		dests, err := svc.List(r.Context(), ch.ID)
		if err != nil {
			http.Error(w, "feed unavailable", http.StatusServiceUnavailable)
			return
		}
		var dest *tracking.Destination
		for _, d := range dests {
			if d.Enabled && d.Kind == tracking.CatalogKind(reg.Feed.Kind) && (raw == "" || wanted == d.ID) {
				if dest != nil {
					http.Error(w, "choose a destination using ?destination=<id>", http.StatusBadRequest)
					return
				}
				selected := d
				dest = &selected
			}
		}
		if dest == nil {
			http.NotFound(w, r)
			return
		}
		body, found, err := exporter.Open(r.Context(), reg.Feed, ch.ID, dest.ID)
		if err != nil {
			http.Error(w, "feed unavailable", http.StatusServiceUnavailable)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		defer body.Close()
		w.Header().Set("Content-Type", reg.Feed.ContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+reg.Feed.Kind+"-"+dest.ID.String()+"."+reg.Feed.Extension+`"`)
		if r.Method != http.MethodHead {
			if _, err := io.Copy(w, body); err != nil {
				slog.Warn("download product feed", "destination", dest.ID, "err", err)
			}
		}
	}
}
