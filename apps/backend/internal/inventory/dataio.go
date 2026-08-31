package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

const (
	colInvSKU       = "sku"
	colInvLocation  = "location"
	colInvQuantity  = "quantity"
	colInvReserved  = "reserved"
	colInvTracked   = "tracked"
	colInvBackorder = "backorderable"
	colInvThreshold = "low_stock_threshold"

	optNote = "note"
)

var levelFields = []dataio.Field{
	{Name: colInvSKU, Label: "SKU", Type: dataio.FieldText, Identifier: true, Required: true, Example: "TEE-BLUE-M"},
	{Name: colInvLocation, Label: "Location", Type: dataio.FieldText, Identifier: true, Description: "Location slug - leave blank only when the store runs in flat (single-stock) mode", Example: "warehouse-1"},
	{Name: colInvQuantity, Label: "Quantity", Type: dataio.FieldNumber, Description: "The absolute count to set, not a delta - the difference is written to the ledger as a manual adjustment", Example: "42"},
	{Name: colInvReserved, Label: "Reserved", Type: dataio.FieldNumber, ReadOnly: true, Description: "Held by open orders - salable is quantity minus this"},
	{Name: colInvTracked, Label: "Tracked", Type: dataio.FieldBool},
	{Name: colInvBackorder, Label: "Backorderable", Type: dataio.FieldBool},
	{Name: colInvThreshold, Label: "Low-stock threshold", Type: dataio.FieldNumber},
}

// A ProductLookup narrower than the service's own, so the resource resolves a SKU without importing catalog
type SKULookup interface {
	ProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, error)
}

// LevelResource is stock on hand, addressed by SKU and location slug. Quantities are absolute: the import writes
// the difference to the ledger as a manual adjustment rather than stacking deltas
func LevelResource(queries *db.Queries, svc *Service, skus SKULookup) dataio.Resource {
	return dataio.Resource{
		Slug:             "inventory_levels",
		Label:            "Stock levels",
		Group:            "Inventory",
		Description:      "On-hand quantity per product per location. Setting a quantity records the difference as a manual adjustment, so the ledger stays the story of how stock moved.",
		ViewPermission:   "inventory.view",
		ImportPermission: "inventory.manage",
		Fields:           levelFields,
		Options: []dataio.OptionDef{
			{Name: optNote, Label: "Adjustment note", Type: dataio.OptString, Description: "Written to every ledger row this run creates", Default: "bulk import", Applies: dataio.DirectionImport},
		},
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return &levelExporter{queries: queries, svc: svc, opts: o}, nil
		},
		NewImporter: func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) {
			im := &levelImporter{svc: svc, skus: skus, opts: o, locations: map[string]uuid.UUID{}, items: map[uuid.UUID]Item{}}
			locs, err := svc.ListLocations(ctx)
			if err != nil {
				return nil, err
			}
			for _, l := range locs {
				im.locations[l.Slug] = l.ID
			}
			im.flat = svc.flatModeEnabled(ctx)
			return im, nil
		},
	}
}

type levelExporter struct {
	queries *db.Queries
	svc     *Service
	opts    dataio.Opts

	levels []db.ExportInventoryLevelsRow
	flat   []db.ExportInventoryItemsFlatRow
}

func (e *levelExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	var err error
	if e.svc.flatModeEnabled(ctx) {
		e.flat, err = e.queries.ExportInventoryItemsFlat(ctx)
		return nil, err
	}
	e.levels, err = e.queries.ExportInventoryLevels(ctx)
	return nil, err
}

func (e *levelExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, src := range e.flat {
		r := dataio.NewRow()
		r.Ref = src.Sku
		r.Set(colInvSKU, src.Sku)
		r.Set(colInvQuantity, src.Quantity)
		r.Set(colInvTracked, src.Tracked)
		r.Set(colInvBackorder, src.Backorderable)
		r.Set(colInvThreshold, src.LowStockThreshold)
		if err := emit(r); err != nil {
			return err
		}
	}
	for _, src := range e.levels {
		r := dataio.NewRow()
		r.Ref = src.Sku
		r.Set(colInvSKU, src.Sku)
		r.Set(colInvLocation, src.LocationSlug)
		r.Set(colInvQuantity, src.Quantity)
		r.Set(colInvReserved, src.ReservedQuantity)
		r.Set(colInvTracked, src.Tracked)
		r.Set(colInvBackorder, src.Backorderable)
		r.Set(colInvThreshold, src.LowStockThreshold)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

type levelImporter struct {
	svc  *Service
	skus SKULookup
	opts dataio.Opts

	flat      bool
	locations map[string]uuid.UUID
	items     map[uuid.UUID]Item // memoised per run - a thousand rows on one product cost one EnsureItem
}

func (im *levelImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	sku := r.Str(colInvSKU)
	r.Ref = sku
	if sku == "" {
		return dataio.OutcomeSkipped, errors.New("missing sku")
	}
	productID, err := im.skus.ProductIDBySKU(ctx, sku)
	if err != nil {
		return dataio.OutcomeSkipped, fmt.Errorf("sku %q: %w", sku, err)
	}
	qty := r.OptFloat(colInvQuantity)
	if qty != nil && *qty < 0 {
		r.Fail("%s cannot be negative", colInvQuantity)
	}
	tracked := r.OptBool(colInvTracked)
	backorder := r.OptBool(colInvBackorder)
	threshold := r.OptFloat(colInvThreshold)
	if err := r.Err(); err != nil {
		return dataio.OutcomeSkipped, err
	}

	var locationID uuid.UUID
	if !im.flat {
		slug := r.Str(colInvLocation)
		if slug == "" {
			return dataio.OutcomeSkipped, errors.New("location is required unless the store runs in flat mode")
		}
		var ok bool
		if locationID, ok = im.locations[slug]; !ok {
			return dataio.OutcomeSkipped, fmt.Errorf("unknown location %q", slug)
		}
	}
	if im.opts.DryRun {
		return dataio.OutcomeUpdated, nil
	}

	item, err := im.item(ctx, productID, tracked)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	if tracked != nil && *tracked != item.Tracked {
		if item, err = im.svc.SetItemTracking(ctx, productID, *tracked); err != nil {
			return dataio.OutcomeSkipped, err
		}
		im.items[productID] = item
	}
	if backorder != nil || threshold != nil {
		wantBackorder, wantThreshold := item.Backorderable, item.LowStockThreshold
		if backorder != nil {
			wantBackorder = *backorder
		}
		if threshold != nil {
			wantThreshold = *threshold
		}
		if item, err = im.svc.SetItemBackorder(ctx, productID, wantBackorder, wantThreshold); err != nil {
			return dataio.OutcomeSkipped, err
		}
		im.items[productID] = item
	}
	if qty == nil {
		return dataio.OutcomeUpdated, nil // a row that only flips tracking is a legitimate row
	}

	if im.flat {
		if _, err := im.svc.SetItemQuantity(ctx, SetItemQuantityInput{ProductID: productID, Quantity: *qty, Source: im.note()}); err != nil {
			return dataio.OutcomeSkipped, err
		}
		return dataio.OutcomeUpdated, nil
	}
	adminID := im.opts.ActorID
	in := AdjustLevelInput{InventoryItemID: item.ID, LocationID: locationID, NewQuantity: *qty, Note: im.note()}
	if adminID != uuid.Nil {
		in.AdminID = &adminID
	}
	if _, err := im.svc.AdjustLevel(ctx, in); err != nil {
		return dataio.OutcomeSkipped, err
	}
	return dataio.OutcomeUpdated, nil
}

func (im *levelImporter) item(ctx context.Context, productID uuid.UUID, tracked *bool) (Item, error) {
	if it, ok := im.items[productID]; ok {
		return it, nil
	}
	// A product with no inventory row yet gets one, defaulting to tracked unless the file says otherwise
	want := true
	if tracked != nil {
		want = *tracked
	}
	it, err := im.svc.EnsureItem(ctx, productID, want)
	if err != nil {
		return Item{}, err
	}
	im.items[productID] = it
	return it, nil
}

func (im *levelImporter) note() string {
	if n := im.opts.Str(optNote); n != "" {
		return n
	}
	return "bulk import"
}
