package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/sqlc-dev/pqtype"
)

// ==================== Products ====================

func (r *postgresRepository) CreateProduct(ctx context.Context, input CreateProductInput) (Product, error) {
	urlPathAuto := false
	if input.URLAuto != nil {
		urlPathAuto = *input.URLAuto
	}
	tracksInventoryOverride := sql.NullBool{}
	if input.TracksInventoryOverride != nil {
		tracksInventoryOverride = sql.NullBool{Bool: *input.TracksInventoryOverride, Valid: true}
	}
	// Decimal quantity defaults: 0 decimals + step 1 (whole units) - The
	// service layer normalises any out-of-range values before we get here
	step := input.QuantityStep
	if step <= 0 {
		step = 1
	}
	// url is NOT NULL - The service guarantees a sensible initial value (the explicit url, else slugified name) before we
	// get here; regenerateProductURL refines it (template + uniqueness) right after create
	initialURL := ""
	if input.URL != nil {
		initialURL = *input.URL
	}
	row, err := r.queries.CreateProduct(ctx, db.CreateProductParams{
		ProductTypeID:           input.ProductTypeID,
		DefaultPrice:            input.DefaultPrice,
		DefaultCurrencyCode:     input.DefaultCurrencyCode,
		Sku:                     input.SKU,
		SeoNoindex:              input.SEONoindex,
		CanonicalUrl:            sqlnull.StringValueNonEmpty(input.CanonicalURL),
		Url:                     initialURL,
		UrlAuto:                 urlPathAuto,
		UrlTemplateOverride:     input.URLTemplateOverride,
		TracksInventoryOverride: tracksInventoryOverride,
		QuantityDecimals:        input.QuantityDecimals,
		QuantityStep:            step,
		Attributes:              json.RawMessage(`{}`), // scalars set via SetProductAttributes
		Metadata:                metadataToJSON(input.Metadata),
		PrivateMetadata:         metadataToJSON(input.PrivateMetadata),
		Kind:                    db.CatalogProductKind(input.Kind),
	})
	if err != nil {
		if isUniqueViolation(err) {
			// SKU is the only unique constraint on catalog_products now (slug was
			// dropped); url uniqueness is enforced downstream by url_rewrites
			return Product{}, ErrSKUTaken
		}
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	return toProduct(row), nil
}

func (r *postgresRepository) GetProductByID(ctx context.Context, id uuid.UUID) (Product, error) {
	row, err := r.queries.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get product: %w", err)
	}
	return toProduct(row), nil
}

func (r *postgresRepository) GetProductBySKU(ctx context.Context, sku string) (Product, error) {
	row, err := r.queries.GetProductBySKU(ctx, sku)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get product by sku: %w", err)
	}
	return toProduct(row), nil
}

func (r *postgresRepository) GetProductsBySKUs(ctx context.Context, skus []string) (map[string]Product, error) {
	out := make(map[string]Product, len(skus))
	if len(skus) == 0 {
		return out, nil
	}
	rows, err := r.queries.GetProductsBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("get products by skus: %w", err)
	}
	for _, row := range rows {
		p := toProduct(row)
		out[p.SKU] = p
	}
	return out, nil
}

// ListProductSelectableValuesByIDs loads selectable attribute-value IDs for a set of products restricted to a set of attribute IDs -
// Returns a map of productID → attributeID → []attributeValueID for the duplicate-combination check in AddProductChild
func (r *postgresRepository) ListProductSelectableValuesByIDs(ctx context.Context, productIDs, attributeIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID][]uuid.UUID, error) {
	if len(productIDs) == 0 || len(attributeIDs) == 0 {
		return map[uuid.UUID]map[uuid.UUID][]uuid.UUID{}, nil
	}
	rows, err := r.queries.ListProductAttributeValuesByProductIDs(ctx, db.ListProductAttributeValuesByProductIDsParams{
		ProductIds:   productIDs,
		AttributeIds: attributeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("list product selectable values: %w", err)
	}
	out := make(map[uuid.UUID]map[uuid.UUID][]uuid.UUID)
	for _, row := range rows {
		if out[row.ProductID] == nil {
			out[row.ProductID] = make(map[uuid.UUID][]uuid.UUID)
		}
		out[row.ProductID][row.AttributeID] = append(out[row.ProductID][row.AttributeID], row.AttributeValueID)
	}
	return out, nil
}

// ==================== Configurable Options ====================

func (r *postgresRepository) SetConfigurableOptions(ctx context.Context, productID uuid.UUID, options []AssignAttributeInput) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.SetConfigurableOptions(ctx, productID); err != nil {
			return fmt.Errorf("clear configurable options: %w", err)
		}
		for _, opt := range options {
			if err := q.InsertConfigurableOption(ctx, db.InsertConfigurableOptionParams{
				ProductID:   productID,
				AttributeID: opt.AttributeID,
				SortOrder:   opt.SortOrder,
			}); err != nil {
				return fmt.Errorf("insert configurable option: %w", err)
			}
		}
		return nil
	})
}

func (r *postgresRepository) CountProductsUsingConfigurableOption(ctx context.Context, attributeID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductsUsingConfigurableOption(ctx, attributeID)
	if err != nil {
		return 0, fmt.Errorf("count products using configurable option: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) CountProductsUsingConfigurableOptionForType(ctx context.Context, attributeID, productTypeID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductsUsingConfigurableOptionForType(ctx, db.CountProductsUsingConfigurableOptionForTypeParams{
		AttributeID:   attributeID,
		ProductTypeID: productTypeID,
	})
	if err != nil {
		return 0, fmt.Errorf("count products using configurable option for type: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListConfigurableOptions(ctx context.Context, productID uuid.UUID) ([]Attribute, error) {
	rows, err := r.queries.ListConfigurableOptions(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list configurable options: %w", err)
	}
	return configurableOptionRowsToAttrs(rows), nil
}

func (r *postgresRepository) ListConfigurableOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Attribute, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID][]Attribute{}, nil
	}
	rows, err := r.queries.ListConfigurableOptionsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list configurable options by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]Attribute)
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], toAttributeFromOptionRow(row))
	}
	return out, nil
}

// buildAttributeFromOptionFields constructs an Attribute from the common column set shared by both configurable-option
// query row types - Centralising the mapping eliminates the copy-paste drift risk between the two callers
func buildAttributeFromOptionFields(
	id uuid.UUID, slug, name, inputType, unit string,
	valueRequired, filterable, searchable, isPublic, facetCollapsed, isIndexable bool,
	h1Position string,
	sortOrder int32,
	metadata, privateMetadata json.RawMessage,
	createdAt, updatedAt time.Time,
) Attribute {
	return Attribute{
		ID:              id,
		Slug:            slug,
		Name:            name,
		InputType:       InputType(inputType),
		Unit:            unit,
		ValueRequired:   valueRequired,
		Filterable:      filterable,
		Searchable:      searchable,
		IsPublic:        isPublic,
		FacetCollapsed:  facetCollapsed,
		IsIndexable:     isIndexable,
		H1Position:      H1Position(h1Position),
		SortOrder:       sortOrder,
		Metadata:        metadataFromJSON(metadata),
		PrivateMetadata: metadataFromJSON(privateMetadata),
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

func configurableOptionRowsToAttrs(rows []db.ListConfigurableOptionsRow) []Attribute {
	out := make([]Attribute, len(rows))
	for i, row := range rows {
		out[i] = buildAttributeFromOptionFields(
			row.ID, row.Slug, row.Name, row.InputType, row.Unit,
			row.ValueRequired, row.Filterable, row.Searchable, row.IsPublic, row.FacetCollapsed, row.IsIndexable,
			row.H1Position,
			row.AttributeSortOrder, row.Metadata, row.PrivateMetadata,
			row.CreatedAt, row.UpdatedAt,
		)
	}
	return out
}

func toAttributeFromOptionRow(row db.ListConfigurableOptionsByProductIDsRow) Attribute {
	return buildAttributeFromOptionFields(
		row.ID, row.Slug, row.Name, row.InputType, row.Unit,
		row.ValueRequired, row.Filterable, row.Searchable, row.IsPublic, row.FacetCollapsed, row.IsIndexable,
		row.H1Position,
		row.AttributeSortOrder, row.Metadata, row.PrivateMetadata,
		row.CreatedAt, row.UpdatedAt,
	)
}

// FacetCounts is a thin pass-through, like the two below it
func (r *postgresRepository) FacetCounts(ctx context.Context, params db.BrowseFacetCountsFallbackParams) ([]db.BrowseFacetCountsFallbackRow, error) {
	return r.queries.BrowseFacetCountsFallback(ctx, params)
}

// SearchFallback is a thin pass-through, like BrowseFallback
func (r *postgresRepository) SearchFallback(ctx context.Context, params db.SearchProductsFallbackParams) ([]db.SearchProductsFallbackRow, error) {
	return r.queries.SearchProductsFallback(ctx, params)
}

// BrowseFallback is a thin pass-through - the query does the filtering so the service stays free of SQL
func (r *postgresRepository) BrowseFallback(ctx context.Context, params db.BrowseProductsFallbackParams) ([]db.BrowseProductsFallbackRow, error) {
	return r.queries.BrowseProductsFallback(ctx, params)
}

// ListProductsWithAttributesByIDs is the batched sibling of GetProductWithAttributes for storefront list hydration: five queries total (products, selections, children, configurable options, media roles) regardless of page size, instead of five PER product AllContent is deliberately NOT loaded - the storefront renders the locale-resolved Content the service attaches afterwards; only the admin content editor needs every translation row - Result preserves the input id order; missing ids are skipped
func (r *postgresRepository) ListProductsWithAttributesByIDs(ctx context.Context, ids []uuid.UUID) ([]Product, error) {
	if len(ids) == 0 {
		return []Product{}, nil
	}
	rows, err := r.queries.GetProductsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get products by ids: %w", err)
	}
	byID := make(map[uuid.UUID]*Product, len(rows))
	for _, row := range rows {
		p := toProduct(row)
		byID[p.ID] = &p
	}

	selRows, err := r.queries.ListProductAttributeSelectionsByProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list selections by ids: %w", err)
	}
	for _, row := range selRows {
		p, ok := byID[row.ProductID]
		if !ok {
			continue
		}
		p.AttributeSelections = append(p.AttributeSelections, ProductAttributeSelection{
			AttributeID: row.AttributeID,
			Value: AttributeValue{
				ID:          row.AttributeValueID,
				AttributeID: row.AttributeID,
				Slug:        row.ValueSlug,
				Name:        row.ValueName,
				Value:       row.ValueHex,
				SortOrder:   row.SortOrder,
			},
			SortOrder: row.SortOrder,
		})
	}

	children, err := r.ListProductChildrenByParentIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	opts, err := r.ListConfigurableOptionsByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	roles, err := r.GetMediaRolesByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, p := range byID {
		p.Children = children[id]
		p.ConfigurableOptions = opts[id]
		p.MediaRoles = roles[id]
	}

	out := make([]Product, 0, len(byID))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *postgresRepository) GetProductWithAttributes(ctx context.Context, id uuid.UUID) (Product, error) {
	p, err := r.GetProductByID(ctx, id)
	if err != nil {
		return Product{}, err
	}
	selections, err := r.ListProductAttributeSelections(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p.AttributeSelections = selections
	refs, err := r.LoadProductsAttributeReferences(ctx, []uuid.UUID{id})
	if err != nil {
		return Product{}, err
	}
	p.AttributeReferences = refs[id]
	children, err := r.ListProductChildren(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p.Children = children
	opts, err := r.ListConfigurableOptions(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p.ConfigurableOptions = opts
	allContent, err := r.ListProductContent(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p.AllContent = allContent
	roles, err := r.GetMediaRoles(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p.MediaRoles = roles
	return p, nil
}

// ProductPageFilter is the admin list's server-side page. Zero value means
// "everything, newest first" - the same set the unpaginated list returned.
type ProductPageFilter struct {
	Locale string
	// nil = every channel the caller can see; set = only products listed there
	ChannelID *uuid.UUID
	Search    string
	Kinds     []string
	TypeSlugs []string
	// nil leaves publish state alone; set narrows to published / unpublished
	Published *bool
	// Attribute value ids to require, ANDed across their attributes
	AttributeValueIDs []uuid.UUID
	// One of the keys the list can sort by; anything else falls back to updatedAt
	SortKey  string
	SortDesc bool
	Limit    int32
	Offset   int32
}

// The list sorts on a fixed set of columns, so the sort key is whitelisted rather than interpolated - it reaches SQL as
// a value compared inside a CASE, never as text spliced into the ORDER BY
var productSortKeys = map[string]struct{}{
	"name": {}, "sku": {}, "url": {}, "kind": {}, "productType": {},
	"defaultPrice": {}, "createdAt": {}, "updatedAt": {},
}

func (r *postgresRepository) ListProductIDsPaged(ctx context.Context, f ProductPageFilter) ([]uuid.UUID, int64, error) {
	sortKey := f.SortKey
	if _, ok := productSortKeys[sortKey]; !ok {
		sortKey = "updatedAt"
	}
	params := db.ListProductsPagedAdminParams{
		Locale:       f.Locale,
		AllChannels:  f.ChannelID == nil,
		Search:       f.Search,
		Kinds:        f.Kinds,
		TypeSlugs:    f.TypeSlugs,
		AttrValueIds: f.AttributeValueIDs,
		SortKey:      sortKey,
		SortDesc:     f.SortDesc,
		Lim:          f.Limit,
		Off:          f.Offset,
	}
	if f.ChannelID != nil {
		params.ChannelID = *f.ChannelID
	}
	if f.Published != nil {
		params.FilterPublished = true
		params.IsPublished = *f.Published
	}
	if params.Kinds == nil {
		params.Kinds = []string{}
	}
	if params.TypeSlugs == nil {
		params.TypeSlugs = []string{}
	}
	if params.AttrValueIds == nil {
		params.AttrValueIds = []uuid.UUID{}
	}
	rows, err := r.queries.ListProductsPagedAdmin(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list products paged: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if f.Offset > 0 {
		// A page past the end returns no rows and so no window count - re-ask for the first page's count rather than
		// reporting an empty catalogue, which would collapse the pager and strand the user
		first := params
		first.Off = 0
		first.Lim = 1
		if head, herr := r.queries.ListProductsPagedAdmin(ctx, first); herr == nil && len(head) > 0 {
			total = head[0].TotalCount
		}
	}
	return ids, total, nil
}

func (r *postgresRepository) ListProducts(ctx context.Context, rowLimit int32) ([]Product, error) {
	rows, err := r.queries.ListProducts(ctx, rowLimit)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedProductForChannel(ctx context.Context, id, channelID uuid.UUID) (Product, error) {
	row, err := r.queries.GetPublishedProductForChannel(ctx, db.GetPublishedProductForChannelParams{
		ID:        id,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get published product for store: %w", err)
	}
	return toProduct(row), nil
}

func (r *postgresRepository) FilterPublishedProductIDsForChannel(ctx context.Context, candidateIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	if len(candidateIDs) == 0 {
		return map[uuid.UUID]struct{}{}, nil
	}
	rows, err := r.queries.ListPublishedProductIDsForStoreFiltered(ctx, db.ListPublishedProductIDsForStoreFilteredParams{
		ProductIds: candidateIDs,
		ChannelID:  channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("filter published product ids: %w", err)
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, id := range rows {
		out[id] = struct{}{}
	}
	return out, nil
}

func (r *postgresRepository) UpdateProduct(ctx context.Context, input UpdateProductInput) (Product, error) {
	current, err := r.queries.GetProductByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("load product: %w", err)
	}

	defaultPrice := current.DefaultPrice
	if input.DefaultPrice != nil {
		defaultPrice = *input.DefaultPrice
	}
	defaultCurrencyCode := current.DefaultCurrencyCode
	if input.DefaultCurrencyCode != nil {
		defaultCurrencyCode = *input.DefaultCurrencyCode
	}
	sku := current.Sku
	if input.SKU != nil {
		sku = *input.SKU
	}
	seoNoindex := current.SeoNoindex
	if input.SEONoindex != nil {
		seoNoindex = *input.SEONoindex
	}
	canonicalURL := current.CanonicalUrl
	if input.CanonicalURL != nil {
		canonicalURL = sqlnull.StringValueNonEmpty(*input.CanonicalURL)
	}
	url := current.Url
	if input.URL != nil {
		url = *input.URL
	}
	urlAuto := current.UrlAuto
	if input.URLAuto != nil {
		urlAuto = *input.URLAuto
	}
	urlTemplateOverride := current.UrlTemplateOverride
	if input.URLTemplateOverride != nil {
		urlTemplateOverride = *input.URLTemplateOverride
	}
	tracksInventoryOverride := current.TracksInventoryOverride
	if input.ClearTracksInventoryOverride {
		tracksInventoryOverride = sql.NullBool{}
	} else if input.TracksInventoryOverride != nil {
		tracksInventoryOverride = sql.NullBool{Bool: *input.TracksInventoryOverride, Valid: true}
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = metadataToJSON(*input.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if input.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(*input.PrivateMetadata)
	}
	quantityDecimals := current.QuantityDecimals
	if input.QuantityDecimals != nil {
		quantityDecimals = *input.QuantityDecimals
	}
	quantityStep := current.QuantityStep
	if input.QuantityStep != nil {
		quantityStep = *input.QuantityStep
	}
	if quantityStep <= 0 {
		quantityStep = 1
	}
	compareAt := current.CompareAtPrice
	if input.ClearCompareAtPrice {
		compareAt = sql.NullInt64{}
	} else if input.CompareAtPrice != nil {
		compareAt = sql.NullInt64{Int64: *input.CompareAtPrice, Valid: true}
	}
	specialPrice := current.SpecialPrice
	specialFrom := current.SpecialFrom
	specialTo := current.SpecialTo
	if input.ClearSpecialPrice {
		specialPrice, specialFrom, specialTo = sql.NullInt64{}, sql.NullTime{}, sql.NullTime{}
	} else if input.SpecialPrice != nil {
		specialPrice = sql.NullInt64{Int64: *input.SpecialPrice, Valid: true}
		specialFrom, specialTo = sql.NullTime{}, sql.NullTime{}
		if input.SpecialFrom != nil {
			specialFrom = sql.NullTime{Time: *input.SpecialFrom, Valid: true}
		}
		if input.SpecialTo != nil {
			specialTo = sql.NullTime{Time: *input.SpecialTo, Valid: true}
		}
	}

	dimCol := func(cur sql.NullInt32, in *int32) sql.NullInt32 {
		if in == nil {
			return cur
		}
		if *in <= 0 {
			return sql.NullInt32{}
		}
		return sql.NullInt32{Int32: *in, Valid: true}
	}
	qtyCol := func(cur sql.NullFloat64, in *float64) sql.NullFloat64 {
		if in == nil {
			return cur
		}
		if *in <= 0 {
			return sql.NullFloat64{}
		}
		return sql.NullFloat64{Float64: *in, Valid: true}
	}
	costCol := func(cur sql.NullInt64, in *int64) sql.NullInt64 {
		if in == nil {
			return cur
		}
		if *in <= 0 {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: *in, Valid: true}
	}

	row, err := r.queries.UpdateProduct(ctx, db.UpdateProductParams{
		ID:                      input.ID,
		DefaultPrice:            defaultPrice,
		DefaultCurrencyCode:     defaultCurrencyCode,
		Sku:                     sku,
		SeoNoindex:              seoNoindex,
		CanonicalUrl:            canonicalURL,
		Url:                     url,
		UrlAuto:                 urlAuto,
		UrlTemplateOverride:     urlTemplateOverride,
		TracksInventoryOverride: tracksInventoryOverride,
		QuantityDecimals:        quantityDecimals,
		QuantityStep:            quantityStep,
		Metadata:                metadata,
		PrivateMetadata:         privateMetadata,
		CompareAtPrice:          compareAt,
		SpecialPrice:            specialPrice,
		SpecialFrom:             specialFrom,
		SpecialTo:               specialTo,
		WeightGrams:             dimCol(current.WeightGrams, input.WeightGrams),
		LengthMm:                dimCol(current.LengthMm, input.LengthMm),
		WidthMm:                 dimCol(current.WidthMm, input.WidthMm),
		HeightMm:                dimCol(current.HeightMm, input.HeightMm),
		MinOrderQty:             qtyCol(current.MinOrderQty, input.MinOrderQty),
		MaxOrderQty:             qtyCol(current.MaxOrderQty, input.MaxOrderQty),
		CostCents:               costCol(current.CostCents, input.CostCents),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Product{}, ErrSKUTaken // the only unique column an update can collide on
		}
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	return toProduct(row), nil
}

// UpdateProductURL replaces only the url column - Used by the service layer's
// auto-template recompute path
func (r *postgresRepository) UpdateProductURL(ctx context.Context, id uuid.UUID, url string) error {
	if _, err := r.queries.UpdateProductURL(ctx, db.UpdateProductURLParams{
		ID:  id,
		Url: url,
	}); err != nil {
		return fmt.Errorf("update product url: %w", err)
	}
	return nil
}

// ListProductsByType returns every product of a given type, ordered by created_at desc - Used by the bulk URL-path
// recompute when a product type's template changes
func (r *postgresRepository) ListProductsByType(ctx context.Context, productTypeID uuid.UUID) ([]Product, error) {
	rows, err := r.queries.ListProductsByType(ctx, productTypeID)
	if err != nil {
		return nil, fmt.Errorf("list products by type: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProduct(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrProductInUse // order lines, bundle parts and kit components hold it ON DELETE RESTRICT
		}
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListProductIDsByAttributeValue(ctx context.Context, valueID uuid.UUID, tokenPattern string) ([]uuid.UUID, error) {
	ids, err := r.queries.ListProductIDsByAttributeValue(ctx, db.ListProductIDsByAttributeValueParams{
		AttributeValueID: valueID,
		TokenPattern:     tokenPattern,
	})
	if err != nil {
		return nil, fmt.Errorf("list product ids by attribute value: %w", err)
	}
	return ids, nil
}

// SetProductAttributes atomically replaces the products.attributes JSONB blob with the fully-resolved scalars and re-writes the junction rows for each touched attribute
// Delete-then-insert, so the write is idempotent whatever was there before - the service hands it pre-validated input
func (r *postgresRepository) SetProductAttributes(ctx context.Context, productID uuid.UUID, scalars ProductScalarAttributes, selections []ProductAttributeSelection) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)

		// Rewrite scalars blob
		scalarsJSON, err := json.Marshal(scalars)
		if err != nil {
			return fmt.Errorf("marshal scalars: %w", err)
		}
		if len(scalars) == 0 {
			scalarsJSON = json.RawMessage(`{}`)
		}
		if _, err := q.UpdateProductAttributes(ctx, db.UpdateProductAttributesParams{
			ID:         productID,
			Attributes: scalarsJSON,
		}); err != nil {
			return fmt.Errorf("update product attributes: %w", err)
		}

		// Delete all existing selections and rewrite from the fully merged set - The merged set already carries untouched attributes' values (preserved by resolveAttributeWrites), so a full replace is semantically identical to a selective replace - but correctly handles cleared attributes that have no entries in selections
		if err := q.DeleteAllProductAttributeSelections(ctx, productID); err != nil {
			return fmt.Errorf("clear selections: %w", err)
		}
		for _, sel := range selections {
			if err := q.InsertProductAttributeValue(ctx, db.InsertProductAttributeValueParams{
				ProductID:        productID,
				AttributeID:      sel.AttributeID,
				AttributeValueID: sel.Value.ID,
				SortOrder:        sel.SortOrder,
			}); err != nil {
				return fmt.Errorf("insert selection: %w", err)
			}
		}
		return nil
	})
}

// ListProductAttributeSelections reads the junction + joined attribute_values data and returns one
// ProductAttributeSelection per row - Caller groups by AttributeID if they need a per-attribute view
func (r *postgresRepository) ListProductAttributeSelections(ctx context.Context, productID uuid.UUID) ([]ProductAttributeSelection, error) {
	rows, err := r.queries.ListProductAttributeValues(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product selections: %w", err)
	}
	out := make([]ProductAttributeSelection, len(rows))
	for i, row := range rows {
		out[i] = ProductAttributeSelection{
			AttributeID: row.AttributeID,
			Value: AttributeValue{
				ID:          row.AttributeValueID,
				AttributeID: row.AttributeID,
				Slug:        row.ValueSlug,
				Name:        row.ValueName,
				Value:       row.ValueHex,
				SortOrder:   row.SortOrder,
			},
			SortOrder: row.SortOrder,
		}
	}
	return out, nil
}

// LoadProductsAttributeSelections is the batched form of ListProductAttributeSelections: it resolves the selected attribute values for many products in a single query, grouped by product id The per-product query above orders by (attribute_id, sort_order); the batched query adds product_id as the leading sort key, so each product's slice keeps the same order it would have on its own
func (r *postgresRepository) LoadProductsAttributeSelections(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductAttributeSelection, error) {
	out := make(map[uuid.UUID][]ProductAttributeSelection, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductAttributeSelectionsByProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list product selections by ids: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], ProductAttributeSelection{
			AttributeID: row.AttributeID,
			Value: AttributeValue{
				ID:          row.AttributeValueID,
				AttributeID: row.AttributeID,
				Slug:        row.ValueSlug,
				Name:        row.ValueName,
				Value:       row.ValueHex,
				SortOrder:   row.SortOrder,
			},
			SortOrder: row.SortOrder,
		})
	}
	return out, nil
}

// SetProductAttributeReferences replaces every reference value for one (product, attribute) pair with refs, atomically - the same
// full-replace contract as SetProductAttributes' selections - Label/Path on the input are ignored (read-path only)
func (r *postgresRepository) SetProductAttributeReferences(ctx context.Context, productID, attributeID uuid.UUID, refs []ProductAttributeReference) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteProductAttributeReferences(ctx, db.DeleteProductAttributeReferencesParams{
			ProductID:   productID,
			AttributeID: attributeID,
		}); err != nil {
			return fmt.Errorf("clear references: %w", err)
		}
		for _, ref := range refs {
			if err := q.InsertProductAttributeReference(ctx, db.InsertProductAttributeReferenceParams{
				ProductID:   productID,
				AttributeID: attributeID,
				TargetType:  string(ref.TargetType),
				TargetID:    ref.TargetID,
				Position:    ref.Position,
			}); err != nil {
				return fmt.Errorf("insert reference: %w", err)
			}
		}
		return nil
	})
}

// LoadProductsAttributeReferences batch-loads reference values for many products, grouped by product id and ordered (attribute_id,
// position) - Label and Path are left empty here - the service hydrates them against the target entities (channel-aware) at read time
func (r *postgresRepository) LoadProductsAttributeReferences(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductAttributeReference, error) {
	out := make(map[uuid.UUID][]ProductAttributeReference, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductAttributeReferencesByProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list product references by ids: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], ProductAttributeReference{
			AttributeID: row.AttributeID,
			TargetType:  ReferenceTarget(row.TargetType),
			TargetID:    row.TargetID,
			Position:    row.Position,
		})
	}
	return out, nil
}

// ==================== mapper ====================

func (r *postgresRepository) SetProductTaxClass(ctx context.Context, id uuid.UUID, taxClass string) (Product, error) {
	row, err := r.queries.UpdateProductTaxClass(ctx, db.UpdateProductTaxClassParams{
		ID:       id,
		TaxClass: taxClass,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("update product tax class: %w", err)
	}
	return toProduct(row), nil
}

func (r *postgresRepository) ProductTaxClassesBySKUs(ctx context.Context, skus []string) (map[string]string, error) {
	if len(skus) == 0 {
		return map[string]string{}, nil
	}
	rows, err := r.queries.GetProductTaxClassesBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("get product tax classes: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Sku] = row.TaxClass
	}
	return out, nil
}

func (r *postgresRepository) ProductDimensionsBySKUs(ctx context.Context, skus []string) (map[string]Dimensions, error) {
	if len(skus) == 0 {
		return map[string]Dimensions{}, nil
	}
	rows, err := r.queries.GetProductDimensionsBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("get product dimensions: %w", err)
	}
	out := make(map[string]Dimensions, len(rows))
	for _, row := range rows {
		out[row.Sku] = Dimensions{
			WeightGrams: row.WeightGrams.Int32,
			LengthMm:    row.LengthMm.Int32,
			WidthMm:     row.WidthMm.Int32,
			HeightMm:    row.HeightMm.Int32,
		}
	}
	return out, nil
}

func (r *postgresRepository) ProductCostsBySKUs(ctx context.Context, skus []string) (map[string]int64, error) {
	if len(skus) == 0 {
		return map[string]int64{}, nil
	}
	rows, err := r.queries.GetProductCostsBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("get product costs: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.CostCents.Valid {
			out[row.Sku] = row.CostCents.Int64
		}
	}
	return out, nil
}

// applySpecialPricing maps the nullable special-pricing columns onto a
// Product - Shared by every row-to-domain converter that carries them
func applySpecialPricing(p *Product, compareAt, special sql.NullInt64, from, to sql.NullTime) {
	if compareAt.Valid {
		v := compareAt.Int64
		p.CompareAtPrice = &v
	}
	if special.Valid {
		v := special.Int64
		p.SpecialPrice = &v
	}
	if from.Valid {
		v := from.Time
		p.SpecialFrom = &v
	}
	if to.Valid {
		v := to.Time
		p.SpecialTo = &v
	}
}

func toProduct(row db.Product) Product {
	p := Product{
		ID:                  row.ID,
		ProductTypeID:       row.ProductTypeID,
		Kind:                ProductKind(row.Kind),
		VariantMode:         variantModeFromRow(row.VariantMode),
		URL:                 row.Url,
		DefaultPrice:        row.DefaultPrice,
		DefaultCurrencyCode: row.DefaultCurrencyCode,
		SKU:                 row.Sku,
		TaxClass:            row.TaxClass,
		SEONoindex:          row.SeoNoindex,
		URLAuto:             row.UrlAuto,
		URLTemplateOverride: row.UrlTemplateOverride,
		QuantityDecimals:    row.QuantityDecimals,
		QuantityStep:        row.QuantityStep,
		Attributes:          scalarAttributesFromJSON(row.Attributes),
		Metadata:            metadataFromJSON(row.Metadata),
		PrivateMetadata:     metadataFromJSON(row.PrivateMetadata),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
	if row.CanonicalUrl.Valid {
		p.CanonicalURL = &row.CanonicalUrl.String
	}
	if row.TracksInventoryOverride.Valid {
		v := row.TracksInventoryOverride.Bool
		p.TracksInventoryOverride = &v
	}
	applySpecialPricing(&p, row.CompareAtPrice, row.SpecialPrice, row.SpecialFrom, row.SpecialTo)
	applyPhysical(&p, row.WeightGrams, row.LengthMm, row.WidthMm, row.HeightMm)
	applyQtyBounds(&p, row.MinOrderQty, row.MaxOrderQty)
	if row.CostCents.Valid {
		v := row.CostCents.Int64
		p.CostCents = &v
	}
	return p
}

// applyQtyBounds maps nullable order-quantity bounds onto a Product
func applyQtyBounds(p *Product, minQ, maxQ sql.NullFloat64) {
	if minQ.Valid {
		v := minQ.Float64
		p.MinOrderQty = &v
	}
	if maxQ.Valid {
		v := maxQ.Float64
		p.MaxOrderQty = &v
	}
}

// applyPhysical maps nullable physical columns onto a Product
func applyPhysical(p *Product, w, l, wd, h sql.NullInt32) {
	set := func(dst **int32, n sql.NullInt32) {
		if n.Valid {
			v := n.Int32
			*dst = &v
		}
	}
	set(&p.WeightGrams, w)
	set(&p.LengthMm, l)
	set(&p.WidthMm, wd)
	set(&p.HeightMm, h)
}

func toProductContent(row db.ProductContent) ProductContent {
	c := ProductContent{
		Locale:    row.Locale,
		Name:      row.Name,
		UpdatedAt: row.UpdatedAt,
	}
	if row.ShortDescription.Valid {
		c.ShortDescription = row.ShortDescription.RawMessage
	}
	if row.Description.Valid {
		c.Description = row.Description.RawMessage
	}
	if row.MetaTitle.Valid {
		c.MetaTitle = &row.MetaTitle.String
	}
	if row.MetaDescription.Valid {
		c.MetaDescription = &row.MetaDescription.String
	}
	if row.MetaKeywords.Valid {
		c.MetaKeywords = &row.MetaKeywords.String
	}
	return c
}

// nullableString converts an empty string to a null NullString and a
// non-empty string to a valid NullString - Used for optional text columns
func (r *postgresRepository) SetProductVariantMode(ctx context.Context, id uuid.UUID, mode VariantMode) (Product, error) {
	row, err := r.queries.SetProductVariantMode(ctx, db.SetProductVariantModeParams{
		ID:          id,
		VariantMode: sql.NullString{String: string(mode), Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("set product variant mode: %w", err)
	}
	return toProduct(row), nil
}

// variantModeFromRow maps the nullable variant_mode column to the domain
// pointer (nil for non-configurables / unset configurables)
func variantModeFromRow(s sql.NullString) *VariantMode {
	if !s.Valid || s.String == "" {
		return nil
	}
	m := VariantMode(s.String)
	return &m
}

func toProductFromChildRow(row db.ListProductChildrenByParentIDsRow) Product {
	p := Product{
		ID:                  row.ID,
		ProductTypeID:       row.ProductTypeID,
		Kind:                ProductKind(row.Kind),
		URL:                 row.Url,
		DefaultPrice:        row.DefaultPrice,
		DefaultCurrencyCode: row.DefaultCurrencyCode,
		SKU:                 row.Sku,
		TaxClass:            row.TaxClass,
		SEONoindex:          row.SeoNoindex,
		URLAuto:             row.UrlAuto,
		URLTemplateOverride: row.UrlTemplateOverride,
		QuantityDecimals:    row.QuantityDecimals,
		QuantityStep:        row.QuantityStep,
		Attributes:          scalarAttributesFromJSON(row.Attributes),
		Metadata:            metadataFromJSON(row.Metadata),
		PrivateMetadata:     metadataFromJSON(row.PrivateMetadata),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
	if row.TracksInventoryOverride.Valid {
		v := row.TracksInventoryOverride.Bool
		p.TracksInventoryOverride = &v
	}
	applySpecialPricing(&p, row.CompareAtPrice, row.SpecialPrice, row.SpecialFrom, row.SpecialTo)
	applyPhysical(&p, row.WeightGrams, row.LengthMm, row.WidthMm, row.HeightMm)
	applyQtyBounds(&p, row.MinOrderQty, row.MaxOrderQty)
	if row.CostCents.Valid {
		v := row.CostCents.Int64
		p.CostCents = &v
	}
	return p
}

// ==================== Product Content ====================

func (r *postgresRepository) UpsertProductContent(ctx context.Context, productID uuid.UUID, input ProductContentInput) (ProductContent, error) {
	row, err := r.queries.UpsertProductContent(ctx, db.UpsertProductContentParams{
		ProductID:        productID,
		Locale:           input.Locale,
		Name:             input.Name,
		ShortDescription: pqtype.NullRawMessage{RawMessage: input.ShortDescription, Valid: len(input.ShortDescription) > 0},
		Description:      pqtype.NullRawMessage{RawMessage: input.Description, Valid: len(input.Description) > 0},
		MetaTitle:        sqlnull.StringValueNonEmpty(input.MetaTitle),
		MetaDescription:  sqlnull.StringValueNonEmpty(input.MetaDescription),
		MetaKeywords:     sqlnull.StringValueNonEmpty(input.MetaKeywords),
	})
	if err != nil {
		return ProductContent{}, fmt.Errorf("upsert product content: %w", err)
	}
	return toProductContent(row), nil
}

func (r *postgresRepository) GetProductContent(ctx context.Context, productID uuid.UUID, locale string) (ProductContent, error) {
	row, err := r.queries.GetProductContent(ctx, db.GetProductContentParams{
		ProductID: productID,
		Locale:    locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductContent{}, ErrProductContentNotFound
		}
		return ProductContent{}, fmt.Errorf("get product content: %w", err)
	}
	return toProductContent(row), nil
}

func (r *postgresRepository) ListProductContent(ctx context.Context, productID uuid.UUID) ([]ProductContent, error) {
	rows, err := r.queries.ListProductContent(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product content: %w", err)
	}
	out := make([]ProductContent, len(rows))
	for i, row := range rows {
		out[i] = toProductContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductContentByScope(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID]ProductContent, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID]ProductContent{}, nil
	}
	rows, err := r.queries.ListProductContentByScope(ctx, db.ListProductContentByScopeParams{
		ProductIds: productIDs,
		Locale:     locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list product content by scope: %w", err)
	}
	out := make(map[uuid.UUID]ProductContent, len(rows))
	for _, row := range rows {
		out[row.ProductID] = toProductContent(row)
	}
	return out, nil
}

// Every locale's row per product, keyed by product - a product with no rows is simply absent
func (r *postgresRepository) ListProductContentForProducts(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductContent, error) {
	out := make(map[uuid.UUID][]ProductContent, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductContentForProducts(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list product content for products: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], toProductContent(row))
	}
	return out, nil
}

func (r *postgresRepository) DeleteProductContent(ctx context.Context, productID uuid.UUID, locale string) error {
	n, err := r.queries.DeleteProductContent(ctx, db.DeleteProductContentParams{
		ProductID: productID,
		Locale:    locale,
	})
	if err != nil {
		return fmt.Errorf("delete product content: %w", err)
	}
	if n == 0 {
		return ErrProductContentNotFound
	}
	return nil
}

func scalarAttributesFromJSON(raw json.RawMessage) ProductScalarAttributes {
	if len(raw) == 0 {
		return ProductScalarAttributes{}
	}
	var m ProductScalarAttributes
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return ProductScalarAttributes{}
	}
	return m
}
