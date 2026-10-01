package catalogio

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

type importer struct {
	cat    *catalog.Service
	opts   dataio.Opts
	locale string
	rs     *resolver

	known    map[string]bool // SKUs this run has landed, so a dry run can resolve a parent created earlier in the same file
	childSeq map[string]int32
	deferred []*dataio.Row // grouped parents, applied in Finish once every member row exists
}

const (
	passTopLevel = 0
	passChildren = 1
)

// Top-level products go first so a configurable parent and its options exist before any variant links to them
func (im *importer) Pass(r *dataio.Row) int {
	if r.Has(colParentSKU) {
		return passChildren
	}
	return passTopLevel
}

func (im *importer) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	sku := r.Str(colSKU)
	r.Ref = sku
	if sku == "" {
		return dataio.OutcomeSkipped, errors.New("missing sku")
	}
	isChild := r.Has(colParentSKU)
	var sortOrder int32
	if isChild {
		parent := r.Str(colParentSKU)
		sortOrder = im.childSeq[parent]
		im.childSeq[parent] = sortOrder + 1
	}

	res, err := im.upsert(ctx, r, isChild, sortOrder)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	if !res.applied {
		return dataio.OutcomeSkipped, nil
	}
	im.known[sku] = true
	if !isChild && res.kind == catalog.KindGrouped && r.Has(colGroupedMembers) {
		im.deferred = append(im.deferred, r)
	}
	if res.created {
		return dataio.OutcomeCreated, nil
	}
	return dataio.OutcomeUpdated, nil
}

// Grouped memberships wait for every product row to have landed, so a member SKU can appear anywhere in the file
func (im *importer) Finish(ctx context.Context) ([]dataio.RowError, error) {
	var errs []dataio.RowError
	for _, r := range im.deferred {
		if err := im.applyGroupedMembers(ctx, r); err != nil {
			errs = append(errs, dataio.RowError{Line: r.Line, Ref: r.Str(colSKU), Message: err.Error()})
		}
	}
	return errs, nil
}

type upsertResult struct {
	applied bool
	created bool
	kind    catalog.ProductKind
}

// One row's resolved inputs, gathered before anything is written so a dry run validates a row as fully as a real one
type rowPlan struct {
	sku, parentSKU, name, urlPath, currency string
	isChild                                 bool
	sortOrder                               int32
	existing                                *catalog.Product
	pt                                      catalog.ProductType
	kind                                    catalog.ProductKind
	price                                   int64
	hasPrice                                bool
	weightGrams, lengthMm, widthMm          *int32
	heightMm                                *int32
	assignments                             []catalog.AttributeAssignment
	catID                                   uuid.UUID
	variantMode                             catalog.VariantMode
	optionIDs                               []uuid.UUID
	publish                                 *bool
}

func (im *importer) upsert(ctx context.Context, r *dataio.Row, isChild bool, sortOrder int32) (upsertResult, error) {
	pl := rowPlan{sku: r.Str(colSKU), parentSKU: r.Str(colParentSKU), isChild: isChild, sortOrder: sortOrder}
	found, err := im.cat.GetProductBySKU(ctx, pl.sku)
	switch {
	case err == nil:
		pl.existing = &found
	case !errors.Is(err, catalog.ErrProductNotFound):
		return upsertResult{}, fmt.Errorf("lookup by sku: %w", err)
	}
	exists := pl.existing != nil

	if pl.pt, err = im.resolveType(ctx, r, pl); err != nil {
		return upsertResult{}, err
	}
	if pl.pt.ID == uuid.Nil {
		// A dry-run child whose parent is only created later in this file - count it, skip validation we can't do yet
		if !exists && !r.RequireDeclared() {
			return upsertResult{}, nil
		}
		return upsertResult{applied: true, created: !exists, kind: catalog.KindSimple}, nil
	}

	if pl.kind, err = rowKind(r, pl); err != nil {
		return upsertResult{}, err
	}
	topLevel := !isChild
	if topLevel && pl.kind == catalog.KindGrouped {
		if _, err := parseGroupedMembers(r.Str(colGroupedMembers)); err != nil {
			return upsertResult{}, err // validated here so a dry run rejects a malformed cell, applied in Finish
		}
	}

	pl.price, pl.hasPrice = r.Int(colPrice), r.Has(colPrice)
	pl.currency = r.Upper(colCurrency)
	pl.name = r.Str(colName)
	pl.urlPath = r.Str(colURLPath)
	pl.weightGrams, pl.lengthMm = r.Int32(colWeightGrams), r.Int32(colLengthMm)
	pl.widthMm, pl.heightMm = r.Int32(colWidthMm), r.Int32(colHeightMm)

	if pl.assignments, err = im.rs.assignmentsForRow(ctx, r, pl.pt, im.opts.Bool(optCreateMissingValues)); err != nil {
		return upsertResult{}, err
	}
	if c := r.Str(colCategory); c != "" {
		if pl.catID, err = im.rs.categoryBySlug(ctx, c); err != nil {
			return upsertResult{}, err
		}
	}
	pl.variantMode = catalog.VariantModeAttribute
	if topLevel && pl.kind == catalog.KindConfigurable {
		if m := r.Enum(colVariantMode); m != "" {
			pl.variantMode = catalog.VariantMode(m)
		}
		for _, oslug := range r.List(colOptions) {
			a, err := im.rs.attributeBySlug(ctx, oslug)
			if err != nil {
				return upsertResult{}, err
			}
			pl.optionIDs = append(pl.optionIDs, a.ID)
		}
	}
	pl.publish = r.OptBool(colPublished)

	// Create-time requirements are checked before the dry-run gate, so a preview surfaces the same errors a real run would
	if !exists && !r.RequireDeclared() {
		return upsertResult{}, nil
	}
	if err := r.Err(); err != nil {
		return upsertResult{}, err
	}
	out := upsertResult{applied: true, created: !exists, kind: pl.kind}
	if im.opts.DryRun {
		return out, nil
	}
	if exists {
		return out, im.apply(ctx, pl, pl.existing.ID)
	}

	p, err := im.cat.CreateProduct(ctx, im.createInput(pl))
	if err != nil {
		return upsertResult{}, fmt.Errorf("create product: %w", err)
	}
	// The catalog's writes each commit alone, so a row failing after its create takes the product back out
	if err := im.apply(ctx, pl, p.ID); err != nil {
		if delErr := im.cat.DeleteProduct(context.WithoutCancel(ctx), p.ID); delErr != nil {
			return upsertResult{}, errors.Join(err, fmt.Errorf("remove partly created product: %w", delErr))
		}
		return upsertResult{}, err
	}
	return out, nil
}

// An existing product keeps its kind - a kind cell that disagrees fails the row rather than steering the wrong steps
func rowKind(r *dataio.Row, pl rowPlan) (catalog.ProductKind, error) {
	cell := catalog.ProductKind(r.Enum(colKind))
	if pl.existing != nil {
		if cell != "" && cell != pl.existing.Kind {
			return "", fmt.Errorf("kind %q does not match the existing product's kind %q - a product's kind can't be changed by import", cell, pl.existing.Kind)
		}
		return pl.existing.Kind, nil
	}
	if pl.isChild || cell == "" {
		return catalog.KindSimple, nil
	}
	return cell, nil
}

func (im *importer) createInput(pl rowPlan) catalog.CreateProductInput {
	in := catalog.CreateProductInput{
		ProductTypeID:       pl.pt.ID,
		Kind:                pl.kind,
		SKU:                 pl.sku,
		DefaultPrice:        pl.price,
		DefaultCurrencyCode: pl.currency,
		Locale:              im.locale,
		Name:                pl.name,
	}
	if pl.urlPath != "" {
		manual := false
		in.URL = &pl.urlPath
		in.URLAuto = &manual
	}
	return in
}

// Everything after the product row itself - a fresh product's update only carries the physical columns CreateProductInput lacks
func (im *importer) apply(ctx context.Context, pl rowPlan, productID uuid.UUID) error {
	var full *catalog.Product
	if pl.existing != nil && (pl.name != "" || (!pl.isChild && pl.kind == catalog.KindConfigurable)) {
		f, err := im.cat.GetProductWithAttributes(ctx, productID)
		if err != nil {
			return fmt.Errorf("load product: %w", err)
		}
		full = &f
	}
	if upd, changed := productUpdate(pl, productID); changed {
		if _, err := im.cat.UpdateProduct(ctx, upd); err != nil {
			return fmt.Errorf("update product: %w", err)
		}
	}
	if full != nil && pl.name != "" {
		if err := im.applyName(ctx, *full, pl.name); err != nil {
			return err
		}
	}
	if len(pl.assignments) > 0 {
		if _, err := im.cat.SetProductAttributes(ctx, productID, pl.assignments); err != nil {
			return fmt.Errorf("set attributes: %w", err)
		}
	}
	if pl.catID != uuid.Nil {
		if _, err := im.cat.AddProductToCategory(ctx, productID, pl.catID, 0, true); err != nil {
			return fmt.Errorf("assign category: %w", err)
		}
	}
	if !pl.isChild && pl.kind == catalog.KindConfigurable {
		if err := im.applyConfigurable(ctx, pl, productID, full); err != nil {
			return err
		}
	}
	if im.opts.ChannelID != uuid.Nil && pl.publish != nil {
		if _, err := im.cat.SetProductListingPublished(ctx, productID, im.opts.ChannelID, *pl.publish); err != nil {
			return fmt.Errorf("set listing: %w", err)
		}
	}
	if pl.isChild {
		return im.linkToParent(ctx, pl, productID)
	}
	return nil
}

// Only the fields whose value differs from what's stored, so a re-import of an unchanged file writes nothing
func productUpdate(pl rowPlan, productID uuid.UUID) (catalog.UpdateProductInput, bool) {
	upd := catalog.UpdateProductInput{ID: productID}
	var cur catalog.Product
	if pl.existing != nil {
		cur = *pl.existing
	}
	changed := false
	if pl.existing != nil {
		if pl.urlPath != "" && (cur.URLAuto || cur.URL != pl.urlPath) {
			// An explicit path switches the product to manual URLs - the service rejects a manual URL while urlPathAuto is on
			manual := false
			upd.URL, upd.URLAuto = &pl.urlPath, &manual
			changed = true
		}
		if pl.hasPrice && pl.price != cur.DefaultPrice {
			upd.DefaultPrice = &pl.price
			changed = true
		}
		if pl.currency != "" && pl.currency != cur.DefaultCurrencyCode {
			upd.DefaultCurrencyCode = &pl.currency
			changed = true
		}
	}
	for _, d := range []struct {
		next, cur *int32
		dst       **int32
	}{
		{pl.weightGrams, cur.WeightGrams, &upd.WeightGrams},
		{pl.lengthMm, cur.LengthMm, &upd.LengthMm},
		{pl.widthMm, cur.WidthMm, &upd.WidthMm},
		{pl.heightMm, cur.HeightMm, &upd.HeightMm},
	} {
		if d.next != nil && (d.cur == nil || *d.cur != *d.next) {
			*d.dst = d.next
			changed = true
		}
	}
	return upd, changed
}

// Read off current state first - the service refuses to change options once children are linked, so blindly re-setting them fails a re-import
func (im *importer) applyConfigurable(ctx context.Context, pl rowPlan, productID uuid.UUID, full *catalog.Product) error {
	var current catalog.Product
	if full != nil {
		current = *full
	}
	if pl.variantMode == catalog.VariantModeList {
		if current.VariantMode == nil || *current.VariantMode != catalog.VariantModeList {
			if _, err := im.cat.SetProductVariantMode(ctx, productID, catalog.VariantModeList); err != nil {
				return fmt.Errorf("set variant mode: %w", err)
			}
		}
		return nil
	}
	if len(pl.optionIDs) > 0 && !sameOptionSet(current.ConfigurableOptions, pl.optionIDs) {
		if _, err := im.cat.SetConfigurableOptions(ctx, productID, pl.optionIDs); err != nil {
			return fmt.Errorf("set configurable options: %w", err)
		}
	}
	return nil
}

func (im *importer) linkToParent(ctx context.Context, pl rowPlan, productID uuid.UUID) error {
	parent, err := im.rs.productBySKU(ctx, pl.parentSKU)
	if err != nil {
		return fmt.Errorf("parent %q: %w", pl.parentSKU, err)
	}
	linked, err := im.rs.isLinked(ctx, parent.ID, productID)
	if err != nil {
		return fmt.Errorf("parent %q: %w", pl.parentSKU, err)
	}
	if linked {
		return nil
	}
	// Re-linking an existing child trips the variant-uniqueness guard, not ErrAlreadyLinked
	if _, err := im.cat.AddProductChild(ctx, parent.ID, productID, pl.sortOrder); err != nil && !errors.Is(err, catalog.ErrAlreadyLinked) {
		return fmt.Errorf("link to parent %q: %w", pl.parentSKU, err)
	}
	im.rs.markLinked(parent.ID, productID)
	return nil
}

// An explicit product_type column wins, otherwise an existing product keeps its type and a variant inherits its parent's
// Returns a zero ProductType only for a dry-run child whose parent is created later in the same file
func (im *importer) resolveType(ctx context.Context, r *dataio.Row, pl rowPlan) (catalog.ProductType, error) {
	switch {
	case r.Has(colProductType):
		return im.rs.productTypeBySlug(ctx, r.Str(colProductType))
	case pl.existing != nil:
		return im.rs.productTypeByID(ctx, pl.existing.ProductTypeID)
	case pl.isChild:
		parent, err := im.rs.productBySKU(ctx, pl.parentSKU)
		if err != nil {
			if errors.Is(err, catalog.ErrProductNotFound) {
				if im.opts.DryRun && im.known[pl.parentSKU] {
					return catalog.ProductType{}, nil
				}
				return catalog.ProductType{}, fmt.Errorf("parent %q not found (add a product_type column or place the parent row first)", pl.parentSKU)
			}
			return catalog.ProductType{}, fmt.Errorf("parent %q: %w", pl.parentSKU, err)
		}
		return im.rs.productTypeByID(ctx, parent.ProductTypeID)
	default:
		return catalog.ProductType{}, errors.New("product_type is required to create a product")
	}
}

type groupedMemberSpec struct {
	sku string
	qty float64
}

// Pipe-separated "SKU" or "SKU:qty" tokens - the first colon splits, so a SKU containing one isn't representable
func parseGroupedMembers(cell string) ([]groupedMemberSpec, error) {
	var specs []groupedMemberSpec
	seen := map[string]bool{}
	for _, tok := range splitMulti(cell) {
		skuPart, qtyPart, hasQty := strings.Cut(tok, ":")
		skuPart = strings.TrimSpace(skuPart)
		if skuPart == "" {
			return nil, fmt.Errorf("grouped_members entry %q is missing a sku", tok)
		}
		if seen[skuPart] {
			return nil, fmt.Errorf("grouped_members lists %q twice", skuPart)
		}
		seen[skuPart] = true
		qty := 1.0
		if hasQty {
			q, err := strconv.ParseFloat(strings.TrimSpace(qtyPart), 64)
			if err != nil || q <= 0 {
				return nil, fmt.Errorf("invalid quantity %q for grouped member %q", qtyPart, skuPart)
			}
			qty = q
		}
		specs = append(specs, groupedMemberSpec{sku: skuPart, qty: qty})
	}
	return specs, nil
}

// Additive like the attribute and category columns: missing members are added and quantity drifts updated, but members already on the product are left alone
func (im *importer) applyGroupedMembers(ctx context.Context, r *dataio.Row) error {
	specs, err := parseGroupedMembers(r.Str(colGroupedMembers))
	if err != nil || len(specs) == 0 {
		return err
	}
	if im.opts.DryRun {
		for _, spec := range specs {
			if im.known[spec.sku] {
				continue
			}
			if _, err := im.rs.productBySKU(ctx, spec.sku); err != nil {
				if errors.Is(err, catalog.ErrProductNotFound) {
					return fmt.Errorf("grouped member %q not found (not in the catalog or this file)", spec.sku)
				}
				return fmt.Errorf("grouped member %q: %w", spec.sku, err)
			}
		}
		return nil
	}

	parent, err := im.rs.productBySKU(ctx, r.Str(colSKU))
	if err != nil {
		return fmt.Errorf("load grouped parent: %w", err)
	}
	current, err := im.cat.ListGroupedMembers(ctx, parent.ID, im.locale)
	if err != nil {
		return fmt.Errorf("list grouped members: %w", err)
	}
	bySKU := make(map[string]catalog.GroupedMember, len(current))
	for _, m := range current {
		bySKU[m.MemberSKU] = m
	}
	for i, spec := range specs {
		if cur, ok := bySKU[spec.sku]; ok {
			if cur.DefaultQuantity != spec.qty {
				q := spec.qty
				if _, err := im.cat.UpdateGroupedMember(ctx, catalog.UpdateGroupedMemberInput{ID: cur.ID, DefaultQuantity: &q}); err != nil {
					return fmt.Errorf("update grouped member %q: %w", spec.sku, err)
				}
			}
			continue
		}
		member, err := im.rs.productBySKU(ctx, spec.sku)
		if err != nil {
			return fmt.Errorf("grouped member %q: %w", spec.sku, err)
		}
		if _, err := im.cat.AddGroupedMember(ctx, catalog.AddGroupedMemberInput{
			ParentID: parent.ID, MemberID: member.ID, DefaultQuantity: spec.qty, SortOrder: int32(i),
		}); err != nil {
			return fmt.Errorf("add grouped member %q: %w", spec.sku, err)
		}
	}
	return nil
}

// Preserves the locale's existing descriptions and SEO copy - the file carries no rich text, so a naive upsert would wipe them
func (im *importer) applyName(ctx context.Context, full catalog.Product, name string) error {
	in := catalog.ProductContentInput{Locale: im.locale, Name: name}
	for _, c := range full.AllContent {
		if c.Locale != im.locale {
			continue
		}
		if c.Name == name {
			return nil
		}
		in.ShortDescription = c.ShortDescription
		in.Description = c.Description
		if c.MetaTitle != nil {
			in.MetaTitle = *c.MetaTitle
		}
		if c.MetaDescription != nil {
			in.MetaDescription = *c.MetaDescription
		}
		if c.MetaKeywords != nil {
			in.MetaKeywords = *c.MetaKeywords
		}
		break
	}
	if _, err := im.cat.UpsertProductContent(ctx, full.ID, in); err != nil {
		return fmt.Errorf("update name: %w", err)
	}
	return nil
}
