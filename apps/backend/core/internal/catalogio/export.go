package catalogio

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// One page of products per read - the cost is set by page size rather than by catalogue size
const exportPageSize = 500

type exporter struct {
	cat     *catalog.Service
	queries *db.Queries
	opts    dataio.Opts
	locale  string

	// Resolved once in Columns, then reused per page - these are small maps, unlike the product set itself
	typeSlug     map[uuid.UUID]string
	attrSlugByID map[uuid.UUID]string
	childParent  map[uuid.UUID]string
	attrCols     []dataio.Field
}

// The attribute columns come from the attribute registry rather than a scan of every product's values. A superset
// of what is actually used, which costs a few empty columns and buys a header that needs no full read to compute
func (ex *exporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	types, err := ex.cat.ListProductTypes(ctx)
	if err != nil {
		return nil, err
	}
	ex.typeSlug = make(map[uuid.UUID]string, len(types))
	for _, t := range types {
		ex.typeSlug[t.ID] = t.Slug
	}

	attrs, err := ex.cat.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	ex.attrSlugByID = make(map[uuid.UUID]string, len(attrs))
	slugs := make([]string, 0, len(attrs))
	labels := make(map[string]string, len(attrs))
	for _, a := range attrs {
		ex.attrSlugByID[a.ID] = a.Slug
		slugs = append(slugs, a.Slug)
		labels[a.Slug] = a.Name
	}
	sort.Strings(slugs)
	ex.attrCols = make([]dataio.Field, 0, len(slugs))
	for _, s := range slugs {
		ex.attrCols = append(ex.attrCols, dataio.Field{Name: attrPrefix + s, Label: labels[s], Type: dataio.FieldText})
	}

	// The whole child→parent map up front, because paging means a variant can surface before its parent does
	ex.childParent = map[uuid.UUID]string{}
	links, err := ex.queries.ExportProductParentSKUs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load variant parents: %w", err)
	}
	for _, l := range links {
		ex.childParent[l.ChildID] = l.ParentSku
	}
	return ex.attrCols, nil
}

// Rows arrive in SKU order rather than parents-then-their-children: the importer groups rows into passes itself,
// so the file no longer has to carry that ordering, and paging no longer has to preserve it
func (ex *exporter) Stream(ctx context.Context, emit func(*dataio.Row) error) error {
	filter := catalog.ProductPageFilter{Locale: ex.locale, SortKey: "sku", Limit: exportPageSize}
	if ex.opts.ChannelID != uuid.Nil {
		ch := ex.opts.ChannelID
		filter.ChannelID = &ch
	}
	for offset := int32(0); ; offset += exportPageSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		filter.Offset = offset
		page, _, err := ex.cat.ListPageWithGraph(ctx, filter)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		if err := ex.emitPage(ctx, page, emit); err != nil {
			return err
		}
		if len(page) < exportPageSize {
			return nil
		}
	}
}

func (ex *exporter) emitPage(ctx context.Context, page []catalog.Product, emit func(*dataio.Row) error) error {
	ids := make([]uuid.UUID, len(page))
	for i, p := range page {
		ids[i] = p.ID
	}

	primary, err := ex.cat.ListPrimaryCategoryByProductIDs(ctx, ids)
	if err != nil {
		return err
	}
	published := map[uuid.UUID]bool{}
	if ex.opts.ChannelID != uuid.Nil {
		listings, err := ex.cat.ListProductChannelListingsByProductIDs(ctx, ids)
		if err != nil {
			return err
		}
		for pid, rows := range listings {
			for _, l := range rows {
				if l.ChannelID == ex.opts.ChannelID {
					published[pid] = l.IsPublished
				}
			}
		}
	}

	grouped, err := ex.groupedMembers(ctx, page)
	if err != nil {
		return err
	}
	for _, p := range page {
		if err := emit(ex.row(p, primary, published, grouped[p.ID])); err != nil {
			return err
		}
	}
	return nil
}

// "SKU" or "SKU:qty" tokens per grouped parent on the page, so a round trip reproduces the memberships - the :qty suffix is omitted for 1
func (ex *exporter) groupedMembers(ctx context.Context, page []catalog.Product) (map[uuid.UUID]string, error) {
	var parents []uuid.UUID
	for _, p := range page {
		if p.Kind == catalog.KindGrouped {
			parents = append(parents, p.ID)
		}
	}
	if len(parents) == 0 {
		return nil, nil
	}
	rows, err := ex.queries.ExportGroupedMemberSKUs(ctx, parents)
	if err != nil {
		return nil, fmt.Errorf("list grouped members: %w", err)
	}
	toks := map[uuid.UUID][]string{}
	for _, m := range rows {
		tok := m.MemberSku
		if m.DefaultQuantity != 1 {
			tok += ":" + strconv.FormatFloat(m.DefaultQuantity, 'f', -1, 64)
		}
		toks[m.ParentID] = append(toks[m.ParentID], tok)
	}
	out := make(map[uuid.UUID]string, len(toks))
	for id, t := range toks {
		out[id] = strings.Join(t, "|")
	}
	return out, nil
}

func (ex *exporter) row(p catalog.Product, primary map[uuid.UUID]*catalog.Category, published map[uuid.UUID]bool, grouped string) *dataio.Row {
	r := dataio.NewRow()
	r.Ref = p.SKU
	parentSKU := ex.childParent[p.ID]

	name := ""
	if p.Content != nil {
		name = p.Content.Name
	}
	category := ""
	if c, ok := primary[p.ID]; ok && c != nil {
		category = c.Slug
	}
	r.Set(colSKU, p.SKU)
	r.Set(colKind, string(p.Kind))
	r.Set(colParentSKU, parentSKU)
	r.Set(colName, name)
	if !p.URLAuto {
		r.Set(colURLPath, p.URL) // an auto path is derived, and re-importing it would pin the product to manual URLs
	}
	r.Set(colProductType, ex.typeSlug[p.ProductTypeID])
	r.Set(colPrice, p.DefaultPrice)
	r.Set(colCurrency, p.DefaultCurrencyCode)
	r.Set(colCategory, category)
	if ex.opts.ChannelID != uuid.Nil {
		r.Set(colPublished, published[p.ID])
	}
	if p.Kind == catalog.KindConfigurable && parentSKU == "" {
		mode := catalog.VariantModeAttribute
		if p.VariantMode != nil {
			mode = *p.VariantMode
		}
		r.Set(colVariantMode, string(mode))
		opts := make([]string, 0, len(p.ConfigurableOptions))
		for _, o := range p.ConfigurableOptions {
			opts = append(opts, o.Slug)
		}
		r.SetList(colOptions, opts)
	}
	r.Set(colGroupedMembers, grouped)
	r.Set(colWeightGrams, p.WeightGrams)
	r.Set(colLengthMm, p.LengthMm)
	r.Set(colWidthMm, p.WidthMm)
	r.Set(colHeightMm, p.HeightMm)

	// Scalars come off the JSONB blob, selectable values off the hydrated selections
	for slug, v := range p.Attributes {
		if s := stringifyScalar(v); s != "" {
			r.Set(attrPrefix+slug, s)
		}
	}
	selected := map[string][]string{}
	for _, sel := range p.AttributeSelections {
		if slug := ex.attrSlugByID[sel.AttributeID]; slug != "" {
			selected[slug] = append(selected[slug], sel.Value.Slug)
		}
	}
	for slug, vals := range selected {
		r.SetList(attrPrefix+slug, vals)
	}
	return r
}
