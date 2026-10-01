package catalogio

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

type importer struct {
	cat  *catalog.Service
	ch   *channel.Service
	opts dataio.Opts
	rs   *resolver

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

	created, err := im.upsert(ctx, r, isChild, sortOrder)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	im.known[sku] = true
	if !isChild && strings.EqualFold(r.Str(colKind), string(catalog.KindGrouped)) && r.Has(colGroupedMembers) {
		im.deferred = append(im.deferred, r)
	}
	if created {
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

func (im *importer) locale() string {
	if l := im.opts.Locale; l != "" {
		return l
	}
	return "en"
}

// Resolves everything before it writes anything, so a dry run validates a row as fully as a real one
func (im *importer) upsert(ctx context.Context, r *dataio.Row, isChild bool, sortOrder int32) (bool, error) {
	sku := r.Str(colSKU)
	existing, getErr := im.cat.GetProductBySKU(ctx, sku)
	exists := getErr == nil
	if getErr != nil && !errors.Is(getErr, catalog.ErrProductNotFound) {
		return false, fmt.Errorf("lookup by sku: %w", getErr)
	}

	parentSKU := r.Str(colParentSKU)
	pt, err := im.resolveType(ctx, r, exists, existing, isChild, parentSKU)
	if err != nil {
		return false, err
	}
	if pt.ID == uuid.Nil {
		// A dry-run child whose parent is only created later in this file - count it, skip validation we can't do yet
		return !exists, nil
	}

	kind := catalog.KindSimple
	if !isChild && r.Has(colKind) {
		kind = catalog.ProductKind(r.Enum(colKind))
	}
	if !isChild && kind == catalog.KindGrouped {
		if _, err := parseGroupedMembers(r.Str(colGroupedMembers)); err != nil {
			return false, err // validated here so a dry run rejects a malformed cell, applied in Finish
		}
	}

	price := r.Int(colPrice)
	currency := r.Upper(colCurrency)
	name := r.Str(colName)
	slug := r.Str(colSlug)
	weightGrams := r.Int32(colWeightGrams)
	lengthMm := r.Int32(colLengthMm)
	widthMm := r.Int32(colWidthMm)
	heightMm := r.Int32(colHeightMm)
	hasPhysical := weightGrams != nil || lengthMm != nil || widthMm != nil || heightMm != nil

	assignments, err := im.rs.assignmentsForRow(ctx, r, pt, im.opts.Bool(optCreateMissingValues))
	if err != nil {
		return false, err
	}

	var catID uuid.UUID
	if c := r.Str(colCategory); c != "" {
		if catID, err = im.rs.categoryBySlug(ctx, c); err != nil {
			return false, err
		}
	}

	var optionIDs []uuid.UUID
	variantMode := catalog.VariantModeAttribute
	if !isChild && kind == catalog.KindConfigurable {
		if m := r.Enum(colVariantMode); m != "" {
			variantMode = catalog.VariantMode(m)
		}
		for _, oslug := range r.List(colOptions) {
			a, err := im.rs.attributeBySlug(ctx, oslug)
			if err != nil {
				return false, err
			}
			optionIDs = append(optionIDs, a.ID)
		}
	}
	publish := r.OptBool(colPublished)

	// Create-time requirements are checked before the dry-run gate, so a preview surfaces the same errors a real run would
	if !exists && !r.RequireDeclared() {
		return false, nil
	}
	if err := r.Err(); err != nil {
		return false, err
	}
	if im.opts.DryRun {
		return !exists, nil
	}

	locale := im.locale()
	var productID uuid.UUID
	if exists {
		productID = existing.ID
		upd := catalog.UpdateProductInput{ID: productID}
		changed := false
		if slug != "" {
			// An explicit slug switches the product to manual URLs - the service rejects a manual URL while urlPathAuto is on
			manual := false
			upd.URL = &slug
			upd.URLAuto = &manual
			changed = true
		}
		if r.Has(colPrice) {
			upd.DefaultPrice = &price
			changed = true
		}
		if currency != "" {
			upd.DefaultCurrencyCode = &currency
			changed = true
		}
		if hasPhysical {
			upd.WeightGrams, upd.LengthMm, upd.WidthMm, upd.HeightMm = weightGrams, lengthMm, widthMm, heightMm
			changed = true
		}
		if changed {
			if _, err := im.cat.UpdateProduct(ctx, upd); err != nil {
				return false, fmt.Errorf("update product: %w", err)
			}
		}
		if name != "" {
			if err := im.applyName(ctx, productID, locale, name); err != nil {
				return false, err
			}
		}
	} else {
		in := catalog.CreateProductInput{
			ProductTypeID:       pt.ID,
			Kind:                kind,
			SKU:                 sku,
			DefaultPrice:        price,
			DefaultCurrencyCode: currency,
			Locale:              locale,
			Name:                name,
		}
		if slug != "" {
			manual := false
			in.URL = &slug
			in.URLAuto = &manual
		}
		p, err := im.cat.CreateProduct(ctx, in)
		if err != nil {
			return false, fmt.Errorf("create product: %w", err)
		}
		productID = p.ID
		// CreateProductInput carries no physical fields, so created rows need a follow-up update to honour those columns
		if hasPhysical {
			if _, err := im.cat.UpdateProduct(ctx, catalog.UpdateProductInput{
				ID: productID, WeightGrams: weightGrams, LengthMm: lengthMm, WidthMm: widthMm, HeightMm: heightMm,
			}); err != nil {
				return false, fmt.Errorf("set physical attributes: %w", err)
			}
		}
	}

	if len(assignments) > 0 {
		if _, err := im.cat.SetProductAttributes(ctx, productID, assignments); err != nil {
			return false, fmt.Errorf("set attributes: %w", err)
		}
	}
	if catID != uuid.Nil {
		if _, err := im.cat.AddProductToCategory(ctx, productID, catID, 0, true); err != nil {
			return false, fmt.Errorf("assign category: %w", err)
		}
	}

	if !isChild && kind == catalog.KindConfigurable {
		// Read current state first - the service refuses to change options once children are linked, so blindly re-setting them fails a re-import
		current, err := im.cat.GetProductWithAttributes(ctx, productID)
		if err != nil {
			return false, fmt.Errorf("load configurable: %w", err)
		}
		if variantMode == catalog.VariantModeList {
			if current.VariantMode == nil || *current.VariantMode != catalog.VariantModeList {
				if _, err := im.cat.SetProductVariantMode(ctx, productID, catalog.VariantModeList); err != nil {
					return false, fmt.Errorf("set variant mode: %w", err)
				}
			}
		} else if len(optionIDs) > 0 && !sameOptionSet(current.ConfigurableOptions, optionIDs) {
			if _, err := im.cat.SetConfigurableOptions(ctx, productID, optionIDs); err != nil {
				return false, fmt.Errorf("set configurable options: %w", err)
			}
		}
	}

	if im.opts.ChannelID != uuid.Nil && publish != nil {
		if _, err := im.cat.SetProductListingPublished(ctx, productID, im.opts.ChannelID, *publish); err != nil {
			return false, fmt.Errorf("set listing: %w", err)
		}
	}

	if isChild {
		parent, err := im.cat.GetProductBySKU(ctx, parentSKU)
		if err != nil {
			return false, fmt.Errorf("parent %q: %w", parentSKU, err)
		}
		linked, err := im.rs.isLinked(ctx, parent.ID, productID)
		if err != nil {
			return false, fmt.Errorf("parent %q: %w", parentSKU, err)
		}
		if !linked {
			// Re-linking an existing child trips the variant-uniqueness guard, not ErrAlreadyLinked
			if _, err := im.cat.AddProductChild(ctx, parent.ID, productID, sortOrder); err != nil {
				if !errors.Is(err, catalog.ErrAlreadyLinked) {
					return false, fmt.Errorf("link to parent %q: %w", parentSKU, err)
				}
			}
			im.rs.markLinked(parent.ID, productID)
		}
	}
	return !exists, nil
}

// An explicit product_type column wins, otherwise an existing product keeps its type and a variant inherits its parent's
// Returns a zero ProductType only for a dry-run child whose parent is created later in the same file
func (im *importer) resolveType(ctx context.Context, r *dataio.Row, exists bool, existing catalog.Product, isChild bool, parentSKU string) (catalog.ProductType, error) {
	switch {
	case r.Has(colProductType):
		return im.rs.productTypeBySlug(ctx, r.Str(colProductType))
	case exists:
		return im.rs.productTypeByID(ctx, existing.ProductTypeID)
	case isChild:
		parent, err := im.cat.GetProductBySKU(ctx, parentSKU)
		if err != nil {
			if errors.Is(err, catalog.ErrProductNotFound) {
				if im.opts.DryRun && im.known[parentSKU] {
					return catalog.ProductType{}, nil
				}
				return catalog.ProductType{}, fmt.Errorf("parent %q not found (add a product_type column or place the parent row first)", parentSKU)
			}
			return catalog.ProductType{}, fmt.Errorf("parent %q: %w", parentSKU, err)
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
			if _, err := im.cat.GetProductBySKU(ctx, spec.sku); err != nil {
				if errors.Is(err, catalog.ErrProductNotFound) {
					return fmt.Errorf("grouped member %q not found (not in the catalog or this file)", spec.sku)
				}
				return fmt.Errorf("grouped member %q: %w", spec.sku, err)
			}
		}
		return nil
	}

	parent, err := im.cat.GetProductBySKU(ctx, r.Str(colSKU))
	if err != nil {
		return fmt.Errorf("load grouped parent: %w", err)
	}
	current, err := im.cat.ListGroupedMembers(ctx, parent.ID, im.locale())
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
		member, err := im.cat.GetProductBySKU(ctx, spec.sku)
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
func (im *importer) applyName(ctx context.Context, productID uuid.UUID, locale, name string) error {
	full, err := im.cat.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return fmt.Errorf("load content: %w", err)
	}
	in := catalog.ProductContentInput{Locale: locale, Name: name}
	for _, c := range full.AllContent {
		if c.Locale != locale {
			continue
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
	if _, err := im.cat.UpsertProductContent(ctx, productID, in); err != nil {
		return fmt.Errorf("update name: %w", err)
	}
	return nil
}
