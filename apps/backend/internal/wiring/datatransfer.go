package wiring

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/customerio"
	"github.com/skeletg/skelet/apps/backend/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
)

// One registry and one runner behind the whole import/export surface - GraphQL, the HTTP upload/download routes and the CLI all read the same registrations
var DataTransfer = fx.Module("datatransfer",
	fx.Provide(newDataRegistry, newDataJobService),
)

type dataResourceDeps struct {
	fx.In

	Queries     *db.Queries
	Catalog     *catalog.Service
	Channel     *channel.Service
	Customer    *customer.Service
	Inventory   *inventory.Service
	Tax         *tax.Service
	Promotion   *promotion.Service
	Destination *destination.Service
	Segment     *segment.Service
}

func newDataRegistry(d dataResourceDeps) *dataio.Registry { // registered here rather than from each package's init() because every handler closes over a live service, the same reason internal/action registers late
	reg := dataio.NewRegistry()
	reg.Register(catalogio.Resource(d.Queries, d.Catalog, d.Channel))
	reg.Register(catalogio.CategoryResource(d.Catalog))
	reg.Register(customerio.CustomerResource(d.Queries, d.Customer))
	reg.Register(customerio.OrderLineResource(d.Queries))
	reg.Register(inventory.LevelResource(d.Queries, d.Inventory, catalogSKULookup{d.Catalog}))
	reg.Register(tax.RateResource(d.Tax))
	reg.Register(promotion.CodeResource(d.Promotion))
	reg.Register(destination.AudienceResource(d.Destination))

	// Mirrors config.RegisterSource - what keeps entity pickers out of the dashboard entirely
	reg.RegisterSource(promotion.SourceRules, func(ctx context.Context) ([]dataio.Choice, error) {
		rules, _, err := d.Promotion.ListRules(ctx, choiceLimit, 0)
		if err != nil {
			return nil, err
		}
		out := make([]dataio.Choice, len(rules))
		for i, r := range rules {
			out[i] = dataio.Choice{Value: r.ID.String(), Label: r.Name}
		}
		return out, nil
	})
	reg.RegisterSource(destination.SourceSegments, func(ctx context.Context) ([]dataio.Choice, error) {
		segments, _, err := d.Segment.List(ctx, choiceLimit, 0)
		if err != nil {
			return nil, err
		}
		out := make([]dataio.Choice, len(segments))
		for i, s := range segments {
			out[i] = dataio.Choice{Value: s.ID.String(), Label: s.Name}
		}
		return out, nil
	})

	if err := reg.ValidateSources(); err != nil { // after registration, so a typo fails the boot rather than rendering an empty dropdown
		panic(fmt.Sprintf("dataio: %v", err))
	}
	return reg
}

// A picker, not a report - past this many an admin wants search, which is its own feature
const choiceLimit = 200

type catalogSKULookup struct{ cat *catalog.Service } // keeps internal/inventory off internal/catalog - the resource only ever needs a SKU turned into an id

func (l catalogSKULookup) ProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, error) {
	p, err := l.cat.GetProductBySKU(ctx, sku)
	if err != nil {
		return uuid.Nil, err
	}
	return p.ID, nil
}

// A nil Storage is survivable: the hub still lists resources and history, and queueing a run says why it can't
func newDataJobService(q *db.Queries, reg *dataio.Registry, storage *media.Storage, cfg *config.Service) *datajob.Service {
	var blobs datajob.Blobs
	if storage != nil {
		blobs = storage
	} else {
		slog.Warn("object storage is not configured - import and export jobs will refuse to run")
	}
	svc := datajob.New(q, reg, blobs)
	svc.SetConfig(cfg)
	return svc
}
