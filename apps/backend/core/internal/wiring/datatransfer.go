package wiring

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/audienceio"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/customerio"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// One registry and one runner behind the whole import/export surface - GraphQL, the HTTP upload/download routes and the CLI all read the same registrations
var DataTransfer = fx.Module("datatransfer",
	fx.Provide(newDataRegistry, newDataJobService),
)

type dataResourceDeps struct {
	fx.In

	Queries   *db.Queries
	Catalog   *catalog.Service
	Channel   *channel.Service
	Customer  *customer.Service
	Inventory *inventory.Service
	Tax       *tax.Service
	Promotion *promotion.Service
	Signals   *signalsclient.Client
	Admin     *admin.Service
}

// The suite answers who is in a segment - core never holds the membership
// An export runs detached, so it acts as the admin who queued it rather than as core itself
func (d dataResourceDeps) segmentMembers(ctx context.Context, actorID, segmentID uuid.UUID) ([]uuid.UUID, error) {
	authed, err := d.Signals.AsAdmin(ctx, actorID)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for token := ""; ; {
		res, err := d.Signals.Segments.ListMembers(authed, &signalsv1.ListMembersRequest{
			SegmentId: segmentID.String(), PageSize: 1000, PageToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, m := range res.GetMembers() {
			if id, perr := uuid.Parse(m.GetCustomerId()); perr == nil {
				out = append(out, id)
			}
		}
		if token = res.GetNextPageToken(); token == "" {
			return out, nil
		}
	}
}

func (d dataResourceDeps) customerEmails(ctx context.Context, ids []uuid.UUID) ([]string, error) {
	return d.Queries.CustomerEmailsByIDs(ctx, ids)
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
	// Membership is the suite's and addresses are core's, so the export asks each for its own half
	reg.Register(audienceio.Resource(d.segmentMembers, d.customerEmails))

	// Mirrors config.RegisterSource - what keeps entity pickers out of the dashboard entirely
	reg.RegisterSource(promotion.SourceRules, func(ctx context.Context) ([]dataio.Choice, error) {
		rules, _, err := d.Promotion.ListRules(ctx, choiceLimit, 0)
		if err != nil {
			return nil, err
		}
		return ruleChoices(ctx, d.Admin, rules)
	})
	reg.RegisterSource(audienceio.SourceSegments, func(ctx context.Context) ([]dataio.Choice, error) {
		authed, err := d.Signals.Authed(ctx) // the picker renders for the admin on the data page
		if err != nil {
			return nil, err
		}
		res, err := d.Signals.Segments.ListSegments(authed, &signalsv1.ListSegmentsRequest{PageSize: choiceLimit})
		if err != nil {
			return nil, err
		}
		segments := res.GetSegments()
		out := make([]dataio.Choice, len(segments))
		for i, s := range segments {
			out[i] = dataio.Choice{Value: s.GetId(), Label: s.GetName()}
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
func newDataJobService(q *db.Queries, reg *dataio.Registry, storage *objstore.Storage, cfg *config.Service) *datajob.Service {
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

func ruleChoices(ctx context.Context, svc auth.ChannelScopeProvider, rules []promotion.Rule) ([]dataio.Choice, error) {
	permitted, err := auth.LoadPermittedChannels(ctx, svc, "promotions.manage")
	if err != nil {
		return nil, err
	}
	out := make([]dataio.Choice, 0, len(rules))
	for _, r := range rules {
		if permitted.AllowsOptional(r.ChannelID) {
			out = append(out, dataio.Choice{Value: r.ID.String(), Label: r.Name})
		}
	}
	return out, nil
}
