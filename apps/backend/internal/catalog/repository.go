package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
	"github.com/sqlc-dev/pqtype"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *tx.Manager
}

// NewRepository returns the concrete repository - It is deliberately a struct, not an interface (Go idiom: return structs, accept narrow interfaces)
func NewRepository(queries *db.Queries, txMgr *tx.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

// ==================== ProductType ====================

func (r *postgresRepository) CreateProductType(ctx context.Context, input CreateProductTypeInput) (ProductType, error) {
	row, err := r.queries.CreateProductType(ctx, db.CreateProductTypeParams{
		Slug:               input.Slug,
		Name:               input.Name,
		IsDigital:          input.IsDigital,
		IsShippingRequired: input.IsShippingRequired,
		UrlTemplate:        input.URLTemplate,
		TracksInventory:    input.TracksInventory,
		Metadata:           metadataToJSON(input.Metadata),
		PrivateMetadata:    metadataToJSON(input.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ProductType{}, ErrSlugTaken
		}
		return ProductType{}, fmt.Errorf("create product type: %w", err)
	}
	return toProductType(row), nil
}

func (r *postgresRepository) GetProductTypeByID(ctx context.Context, id uuid.UUID) (ProductType, error) {
	row, err := r.queries.GetProductTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductType{}, ErrProductTypeNotFound
		}
		return ProductType{}, fmt.Errorf("get product type: %w", err)
	}
	return toProductType(row), nil
}

// GetProductTypesByIDs is the batched form - Returns rows keyed by id; missing ids are absent from the map - No attribute hydration - the storefront DataLoader path only needs the type's identity fields (name, slug) - Callers that need the full attribute graph keep using GetProductTypeWithAttributes for one id
func (r *postgresRepository) GetProductTypesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductType, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]ProductType{}, nil
	}
	rows, err := r.queries.GetProductTypesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get product types by ids: %w", err)
	}
	out := make(map[uuid.UUID]ProductType, len(rows))
	for _, row := range rows {
		out[row.ID] = toProductType(row)
	}
	return out, nil
}

// GetProductTypeWithAttributes loads the product type plus its assigned attributes and each attribute's values (where applicable) - Uses four queries: 1 product_type, 1 junction+attributes join, 1 values-by-ids (only for attributes whose input type accepts values) - No N+1
func (r *postgresRepository) GetProductTypeWithAttributes(ctx context.Context, id uuid.UUID) (ProductType, error) {
	pt, err := r.GetProductTypeByID(ctx, id)
	if err != nil {
		return ProductType{}, err
	}

	rows, err := r.queries.ListProductTypeAttributes(ctx, id)
	if err != nil {
		return ProductType{}, fmt.Errorf("list product type attributes: %w", err)
	}
	if len(rows) == 0 {
		pt.Attributes = []ProductTypeAttribute{}
		return pt, nil
	}

	// Collect attribute IDs whose input types carry values so we can batch-fetch
	attrsByID := make(map[uuid.UUID]*Attribute, len(rows))
	var valueAttrIDs []uuid.UUID
	assignments := make([]ProductTypeAttribute, len(rows))
	for i, row := range rows {
		attr := Attribute{
			ID:                 row.ID,
			Slug:               row.Slug,
			Name:               row.Name,
			InputType:          InputType(row.InputType),
			RelationshipTarget: ReferenceTarget(row.RelationshipTarget.String),
			Unit:               row.Unit,
			ValueRequired:      row.ValueRequired,
			Filterable:         row.Filterable,
			Searchable:         row.Searchable,
			IsPublic:           row.IsPublic,
			FacetCollapsed:     row.FacetCollapsed,
			IsIndexable:        row.IsIndexable,
			H1Position:         H1Position(row.H1Position),
			SortOrder:          row.AttributeSortOrder,
			Metadata:           metadataFromJSON(row.Metadata),
			PrivateMetadata:    metadataFromJSON(row.PrivateMetadata),
			DefaultValue:       rawMessageFromNullable(row.DefaultValue),
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		}
		var groupID *uuid.UUID
		if row.GroupID.Valid {
			id := row.GroupID.UUID
			groupID = &id
		}
		assignments[i] = ProductTypeAttribute{
			Attribute: attr,
			SortOrder: row.AssignmentSortOrder,
			GroupID:   groupID,
		}
		attrsByID[attr.ID] = &assignments[i].Attribute
		if attr.InputType.AcceptsValues() {
			valueAttrIDs = append(valueAttrIDs, attr.ID)
		}
	}

	if len(valueAttrIDs) > 0 {
		vrows, err := r.queries.ListAttributeValuesByAttributeIDs(ctx, valueAttrIDs)
		if err != nil {
			return ProductType{}, fmt.Errorf("list attribute values: %w", err)
		}
		for _, vr := range vrows {
			if a, ok := attrsByID[vr.AttributeID]; ok {
				a.Values = append(a.Values, toAttributeValue(vr))
			}
		}
	}

	// Ensure non-nil slices for attrs that accept values but have none yet
	for _, a := range attrsByID {
		if a.InputType.AcceptsValues() && a.Values == nil {
			a.Values = []AttributeValue{}
		}
	}

	pt.Attributes = assignments

	// Load groups and bucket assignments into them
	groups, err := r.ListAttributeGroupsByProductType(ctx, id)
	if err != nil {
		return ProductType{}, fmt.Errorf("list attribute groups: %w", err)
	}
	groupsByID := make(map[uuid.UUID]*AttributeGroup, len(groups))
	for i := range groups {
		groups[i].Attributes = []ProductTypeAttribute{}
		groupsByID[groups[i].ID] = &groups[i]
	}
	for _, a := range assignments {
		if a.GroupID != nil {
			if g, ok := groupsByID[*a.GroupID]; ok {
				g.Attributes = append(g.Attributes, a)
			}
		}
	}
	pt.Groups = groups

	return pt, nil
}

// Attributes are hydrated in one batched query - returning bare rows would leave every
// caller counting Attributes (the admin list's column) saw zero
// Values and groups are left off: the list surfaces need the assignment, and fetching them would
// cost two more queries per page - GetProductType stays the hydrating read for the detail view
func (r *postgresRepository) ListProductTypes(ctx context.Context) ([]ProductType, error) {
	rows, err := r.queries.ListProductTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list product types: %w", err)
	}
	out := make([]ProductType, len(rows))
	ids := make([]uuid.UUID, len(rows))
	byID := make(map[uuid.UUID]*ProductType, len(rows))
	for i, row := range rows {
		out[i] = toProductType(row)
		out[i].Attributes = []ProductTypeAttribute{}
		ids[i] = out[i].ID
		byID[out[i].ID] = &out[i]
	}
	if len(ids) == 0 {
		return out, nil
	}
	arows, err := r.queries.ListProductTypeAttributesByTypeIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list product type attributes: %w", err)
	}
	for _, row := range arows {
		pt, ok := byID[row.ProductTypeID]
		if !ok {
			continue
		}
		var groupID *uuid.UUID
		if row.GroupID.Valid {
			id := row.GroupID.UUID
			groupID = &id
		}
		pt.Attributes = append(pt.Attributes, ProductTypeAttribute{
			Attribute: Attribute{
				ID:                 row.ID,
				Slug:               row.Slug,
				Name:               row.Name,
				InputType:          InputType(row.InputType),
				RelationshipTarget: ReferenceTarget(row.RelationshipTarget.String),
				Unit:               row.Unit,
				ValueRequired:      row.ValueRequired,
				Filterable:         row.Filterable,
				Searchable:         row.Searchable,
				IsPublic:           row.IsPublic,
				FacetCollapsed:     row.FacetCollapsed,
				IsIndexable:        row.IsIndexable,
				H1Position:         H1Position(row.H1Position),
				SortOrder:          row.AttributeSortOrder,
				Metadata:           metadataFromJSON(row.Metadata),
				PrivateMetadata:    metadataFromJSON(row.PrivateMetadata),
				DefaultValue:       rawMessageFromNullable(row.DefaultValue),
				CreatedAt:          row.CreatedAt,
				UpdatedAt:          row.UpdatedAt,
			},
			SortOrder: row.AssignmentSortOrder,
			GroupID:   groupID,
		})
	}
	return out, nil
}

func (r *postgresRepository) UpdateProductType(ctx context.Context, input UpdateProductTypeInput) (ProductType, error) {
	current, err := r.queries.GetProductTypeByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductType{}, ErrProductTypeNotFound
		}
		return ProductType{}, fmt.Errorf("load product type: %w", err)
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	isDigital := current.IsDigital
	if input.IsDigital != nil {
		isDigital = *input.IsDigital
	}
	isShippingRequired := current.IsShippingRequired
	if input.IsShippingRequired != nil {
		isShippingRequired = *input.IsShippingRequired
	}
	urlTemplate := current.UrlTemplate
	if input.URLTemplate != nil {
		urlTemplate = *input.URLTemplate
	}
	tracksInventory := current.TracksInventory
	if input.TracksInventory != nil {
		tracksInventory = *input.TracksInventory
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = metadataToJSON(*input.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if input.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(*input.PrivateMetadata)
	}

	row, err := r.queries.UpdateProductType(ctx, db.UpdateProductTypeParams{
		ID:                 input.ID,
		Name:               name,
		IsDigital:          isDigital,
		IsShippingRequired: isShippingRequired,
		UrlTemplate:        urlTemplate,
		TracksInventory:    tracksInventory,
		Metadata:           metadata,
		PrivateMetadata:    privateMetadata,
	})
	if err != nil {
		return ProductType{}, fmt.Errorf("update product type: %w", err)
	}
	return toProductType(row), nil
}

func (r *postgresRepository) DeleteProductType(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProductType(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrProductTypeInUse // catalog_products references it ON DELETE RESTRICT
		}
		return fmt.Errorf("delete product type: %w", err)
	}
	return nil
}

// ==================== Attribute ====================

func (r *postgresRepository) CreateAttribute(ctx context.Context, input CreateAttributeInput) (Attribute, error) {
	var created Attribute
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateAttribute(ctx, db.CreateAttributeParams{
			Slug:               input.Slug,
			Name:               input.Name,
			InputType:          string(input.InputType),
			Unit:               input.Unit,
			ValueRequired:      input.ValueRequired,
			Filterable:         input.Filterable,
			Searchable:         input.Searchable,
			IsPublic:           input.IsPublic,
			FacetCollapsed:     input.FacetCollapsed,
			IsIndexable:        input.IsIndexable,
			H1Position:         string(input.H1Position),
			SortOrder:          input.SortOrder,
			Metadata:           metadataToJSON(input.Metadata),
			PrivateMetadata:    metadataToJSON(input.PrivateMetadata),
			DefaultValue:       rawMessageToNullable(input.DefaultValue),
			SynapseTrack:       input.SynapseTrack,
			AutoSegment:        input.AutoSegment,
			RelationshipTarget: nullStringFromRefTarget(input.RelationshipTarget),
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrSlugTaken
			}
			return fmt.Errorf("create attribute: %w", err)
		}
		created = toAttribute(row)

		if len(input.Values) > 0 {
			values := make([]AttributeValue, 0, len(input.Values))
			for _, v := range input.Values {
				vrow, err := q.CreateAttributeValue(ctx, db.CreateAttributeValueParams{
					AttributeID: created.ID,
					Slug:        v.Slug,
					Name:        v.Name,
					Value:       v.Value,
					SortOrder:   v.SortOrder,
					IsIndexable: v.IsIndexable,
				})
				if err != nil {
					if isUniqueViolation(err) {
						return fmt.Errorf("attribute value slug %q: %w", v.Slug, ErrSlugTaken)
					}
					return fmt.Errorf("create attribute value: %w", err)
				}
				values = append(values, toAttributeValue(vrow))
			}
			created.Values = values
		}
		return nil
	})
	if err != nil {
		return Attribute{}, err
	}
	return created, nil
}

func (r *postgresRepository) GetAttributeByID(ctx context.Context, id uuid.UUID) (Attribute, error) {
	row, err := r.queries.GetAttributeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("get attribute: %w", err)
	}
	return toAttribute(row), nil
}

func (r *postgresRepository) GetAttributeWithValues(ctx context.Context, id uuid.UUID) (Attribute, error) {
	attr, err := r.GetAttributeByID(ctx, id)
	if err != nil {
		return Attribute{}, err
	}
	if !attr.InputType.AcceptsValues() {
		attr.Values = []AttributeValue{}
		return attr, nil
	}
	vrows, err := r.queries.ListAttributeValues(ctx, id)
	if err != nil {
		return Attribute{}, fmt.Errorf("list attribute values: %w", err)
	}
	attr.Values = make([]AttributeValue, len(vrows))
	for i, v := range vrows {
		attr.Values[i] = toAttributeValue(v)
	}
	return attr, nil
}

func (r *postgresRepository) ListAttributes(ctx context.Context) ([]Attribute, error) {
	rows, err := r.queries.ListAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list attributes: %w", err)
	}
	if len(rows) == 0 {
		return []Attribute{}, nil
	}
	attrs := make([]Attribute, len(rows))
	valueIDs := make([]uuid.UUID, 0, len(rows))
	attrsByID := make(map[uuid.UUID]*Attribute, len(rows))
	for i, row := range rows {
		attrs[i] = toAttribute(row)
		attrsByID[attrs[i].ID] = &attrs[i]
		if attrs[i].InputType.AcceptsValues() {
			valueIDs = append(valueIDs, attrs[i].ID)
			attrs[i].Values = []AttributeValue{} // ensure non-nil
		}
	}
	if len(valueIDs) > 0 {
		vrows, err := r.queries.ListAttributeValuesByAttributeIDs(ctx, valueIDs)
		if err != nil {
			return nil, fmt.Errorf("list attribute values: %w", err)
		}
		for _, vr := range vrows {
			if a, ok := attrsByID[vr.AttributeID]; ok {
				a.Values = append(a.Values, toAttributeValue(vr))
			}
		}
	}
	return attrs, nil
}

func (r *postgresRepository) UpdateAttribute(ctx context.Context, input UpdateAttributeInput) (Attribute, error) {
	current, err := r.queries.GetAttributeByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("load attribute: %w", err)
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	unit := current.Unit
	if input.Unit != nil {
		unit = *input.Unit
	}
	valueRequired := current.ValueRequired
	if input.ValueRequired != nil {
		valueRequired = *input.ValueRequired
	}
	filterable := current.Filterable
	if input.Filterable != nil {
		filterable = *input.Filterable
	}
	searchable := current.Searchable
	if input.Searchable != nil {
		searchable = *input.Searchable
	}
	isPublic := current.IsPublic
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}
	facetCollapsed := current.FacetCollapsed
	if input.FacetCollapsed != nil {
		facetCollapsed = *input.FacetCollapsed
	}
	isIndexable := current.IsIndexable
	if input.IsIndexable != nil {
		isIndexable = *input.IsIndexable
	}
	h1Position := current.H1Position
	if input.H1Position != nil {
		h1Position = string(*input.H1Position)
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = metadataToJSON(*input.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if input.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(*input.PrivateMetadata)
	}
	defaultValue := current.DefaultValue
	if input.DefaultValue != nil {
		defaultValue = rawMessageToNullable(*input.DefaultValue)
	}
	synapseTrack := current.SynapseTrack
	if input.SynapseTrack != nil {
		synapseTrack = *input.SynapseTrack
	}
	autoSegment := current.AutoSegment
	if input.AutoSegment != nil {
		autoSegment = *input.AutoSegment
	}

	row, err := r.queries.UpdateAttribute(ctx, db.UpdateAttributeParams{
		ID:              input.ID,
		Name:            name,
		Unit:            unit,
		ValueRequired:   valueRequired,
		Filterable:      filterable,
		Searchable:      searchable,
		IsPublic:        isPublic,
		FacetCollapsed:  facetCollapsed,
		IsIndexable:     isIndexable,
		H1Position:      h1Position,
		SortOrder:       sortOrder,
		Metadata:        metadata,
		PrivateMetadata: privateMetadata,
		DefaultValue:    defaultValue,
		SynapseTrack:    synapseTrack,
		AutoSegment:     autoSegment,
		// Immutable after create (like input_type) - preserve unchanged
		RelationshipTarget: current.RelationshipTarget,
	})
	if err != nil {
		return Attribute{}, fmt.Errorf("update attribute: %w", err)
	}
	return toAttribute(row), nil
}

func (r *postgresRepository) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteAttribute(ctx, id); err != nil {
		return fmt.Errorf("delete attribute: %w", err)
	}
	return nil
}

// SetAttributeValuesIndexable flips a hand-picked set of values in one statement - the SEO
// page's bulk opt-in, where looping UpdateAttributeValue would rewrite whole rows
func (r *postgresRepository) SetAttributeValuesIndexable(ctx context.Context, valueIDs []uuid.UUID, isIndexable bool) (int64, error) {
	n, err := r.queries.SetAttributeValuesIndexable(ctx, db.SetAttributeValuesIndexableParams{
		Ids:         valueIDs,
		IsIndexable: isIndexable,
	})
	if err != nil {
		return 0, fmt.Errorf("set attribute values indexable: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) CascadeAttributeIndexable(ctx context.Context, attributeID uuid.UUID, isIndexable bool) (CascadeAttributeIndexableResult, error) {
	var out CascadeAttributeIndexableResult
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)

		// Snapshot the affected-row counts before we update so the number reported in the admin "cascaded to N values" toast matches what was actually touched, even when most rows were already in the target state - AffectedProductsCount is now purely informational - it reports how many scalar slug overrides exist under the attribute, not an indexability cascade (scalar facets follow the attribute flag directly)
		valCount, err := q.CountAttributeValues(ctx, attributeID)
		if err != nil {
			return fmt.Errorf("count values: %w", err)
		}
		prodCount, err := q.CountProductAttributeSlugs(ctx, attributeID)
		if err != nil {
			return fmt.Errorf("count product slug overrides: %w", err)
		}

		row, err := q.SetAttributeIndexableCascade(ctx, db.SetAttributeIndexableCascadeParams{
			ID:          attributeID,
			IsIndexable: isIndexable,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAttributeNotFound
			}
			return fmt.Errorf("update attribute: %w", err)
		}

		if err := q.CascadeAttributeValuesIndexable(ctx, db.CascadeAttributeValuesIndexableParams{
			AttributeID: attributeID,
			IsIndexable: isIndexable,
		}); err != nil {
			return fmt.Errorf("cascade values: %w", err)
		}

		out = CascadeAttributeIndexableResult{
			Attribute:             toAttribute(row),
			AffectedValuesCount:   valCount,
			AffectedProductsCount: prodCount,
		}
		return nil
	})
	if err != nil {
		return CascadeAttributeIndexableResult{}, err
	}
	return out, nil
}

// ==================== AttributeValue ====================

func (r *postgresRepository) AddAttributeValue(ctx context.Context, attributeID uuid.UUID, v AttributeValueInput) (AttributeValue, error) {
	row, err := r.queries.CreateAttributeValue(ctx, db.CreateAttributeValueParams{
		AttributeID: attributeID,
		Slug:        v.Slug,
		Name:        v.Name,
		Value:       v.Value,
		SortOrder:   v.SortOrder,
		IsIndexable: v.IsIndexable,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AttributeValue{}, ErrSlugTaken
		}
		return AttributeValue{}, fmt.Errorf("create attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) GetAttributeValueByID(ctx context.Context, id uuid.UUID) (AttributeValue, error) {
	row, err := r.queries.GetAttributeValueByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeValue{}, ErrAttributeValueNotFound
		}
		return AttributeValue{}, fmt.Errorf("get attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) UpdateAttributeValue(ctx context.Context, id uuid.UUID, v AttributeValueInput) (AttributeValue, error) {
	row, err := r.queries.UpdateAttributeValue(ctx, db.UpdateAttributeValueParams{
		ID:          id,
		Slug:        v.Slug,
		Name:        v.Name,
		Value:       v.Value,
		SortOrder:   v.SortOrder,
		IsIndexable: v.IsIndexable,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeValue{}, ErrAttributeValueNotFound
		}
		if isUniqueViolation(err) {
			return AttributeValue{}, ErrSlugTaken
		}
		return AttributeValue{}, fmt.Errorf("update attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) DeleteAttributeValue(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteAttributeValue(ctx, id); err != nil {
		return fmt.Errorf("delete attribute value: %w", err)
	}
	return nil
}

// ==================== AttributeContent / AttributeValueContent ====================

func (r *postgresRepository) ListAttributeContent(ctx context.Context, attributeID uuid.UUID) ([]AttributeContent, error) {
	rows, err := r.queries.ListAttributeContent(ctx, attributeID)
	if err != nil {
		return nil, fmt.Errorf("list attribute content: %w", err)
	}
	out := make([]AttributeContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAttributeContentByAttributeIDs(ctx context.Context, attributeIDs []uuid.UUID) ([]AttributeContent, error) {
	if len(attributeIDs) == 0 {
		return []AttributeContent{}, nil
	}
	rows, err := r.queries.ListAttributeContentByAttributeIDs(ctx, attributeIDs)
	if err != nil {
		return nil, fmt.Errorf("list attribute content by ids: %w", err)
	}
	out := make([]AttributeContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeContent(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertAttributeContent(ctx context.Context, input UpsertAttributeContentInput) (AttributeContent, error) {
	row, err := r.queries.UpsertAttributeContent(ctx, db.UpsertAttributeContentParams{
		AttributeID: input.AttributeID,
		Locale:      input.Locale,
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		return AttributeContent{}, fmt.Errorf("upsert attribute content: %w", err)
	}
	return toAttributeContent(row), nil
}

func (r *postgresRepository) DeleteAttributeContent(ctx context.Context, attributeID uuid.UUID, locale string) error {
	if err := r.queries.DeleteAttributeContent(ctx, db.DeleteAttributeContentParams{
		AttributeID: attributeID,
		Locale:      locale,
	}); err != nil {
		return fmt.Errorf("delete attribute content: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListAttributeValueContent(ctx context.Context, valueID uuid.UUID) ([]AttributeValueContent, error) {
	rows, err := r.queries.ListAttributeValueContent(ctx, valueID)
	if err != nil {
		return nil, fmt.Errorf("list attribute value content: %w", err)
	}
	out := make([]AttributeValueContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAttributeValueContentByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueContent, error) {
	if len(valueIDs) == 0 {
		return []AttributeValueContent{}, nil
	}
	rows, err := r.queries.ListAttributeValueContentByValueIDs(ctx, valueIDs)
	if err != nil {
		return nil, fmt.Errorf("list attribute value content by ids: %w", err)
	}
	out := make([]AttributeValueContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueContent(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertAttributeValueContent(ctx context.Context, input UpsertAttributeValueContentInput) (AttributeValueContent, error) {
	row, err := r.queries.UpsertAttributeValueContent(ctx, db.UpsertAttributeValueContentParams{
		ValueID:     input.ValueID,
		Locale:      input.Locale,
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		return AttributeValueContent{}, fmt.Errorf("upsert attribute value content: %w", err)
	}
	return toAttributeValueContent(row), nil
}

func (r *postgresRepository) DeleteAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale string) error {
	if err := r.queries.DeleteAttributeValueContent(ctx, db.DeleteAttributeValueContentParams{
		ValueID: valueID,
		Locale:  locale,
	}); err != nil {
		return fmt.Errorf("delete attribute value content: %w", err)
	}
	return nil
}

func toAttributeContent(row db.CatalogAttributeContent) AttributeContent {
	return AttributeContent{
		AttributeID: row.AttributeID,
		Locale:      row.Locale,
		Name:        row.Name,
		Description: row.Description,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toAttributeValueContent(row db.CatalogAttributeValueContent) AttributeValueContent {
	return AttributeValueContent{
		ValueID:     row.ValueID,
		Locale:      row.Locale,
		Name:        row.Name,
		Description: row.Description,
		UpdatedAt:   row.UpdatedAt,
	}
}

// ==================== ProductAttributeSlug ====================

func (r *postgresRepository) ListProductAttributeSlugs(ctx context.Context, productID uuid.UUID) ([]ProductAttributeSlug, error) {
	rows, err := r.queries.ListProductAttributeSlugs(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product attribute slugs: %w", err)
	}
	out := make([]ProductAttributeSlug, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeSlug(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductAttributeSlugsByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeSlug, error) {
	if len(productIDs) == 0 {
		return []ProductAttributeSlug{}, nil
	}
	rows, err := r.queries.ListProductAttributeSlugsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list product attribute slugs by product ids: %w", err)
	}
	out := make([]ProductAttributeSlug, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeSlug(row)
	}
	return out, nil
}

func (r *postgresRepository) GetProductAttributeSlug(ctx context.Context, productID, attributeID uuid.UUID) (ProductAttributeSlug, error) {
	row, err := r.queries.GetProductAttributeSlug(ctx, db.GetProductAttributeSlugParams{
		ProductID:   productID,
		AttributeID: attributeID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductAttributeSlug{}, ErrProductAttributeSlugNotFound
		}
		return ProductAttributeSlug{}, fmt.Errorf("get product attribute slug: %w", err)
	}
	return toProductAttributeSlug(row), nil
}

func (r *postgresRepository) UpsertProductAttributeSlug(ctx context.Context, input UpsertProductAttributeSlugInput) (ProductAttributeSlug, error) {
	// Global and channel rows live under different partial unique indexes, so each needs its own ON CONFLICT target
	if input.ChannelID == nil {
		row, err := r.queries.UpsertProductAttributeSlug(ctx, db.UpsertProductAttributeSlugParams{
			ProductID:   input.ProductID,
			AttributeID: input.AttributeID,
			Slug:        input.Slug,
		})
		if err != nil {
			return ProductAttributeSlug{}, fmt.Errorf("upsert product attribute slug: %w", err)
		}
		return toProductAttributeSlug(row), nil
	}
	row, err := r.queries.UpsertProductAttributeSlugForChannel(ctx, db.UpsertProductAttributeSlugForChannelParams{
		ProductID:   input.ProductID,
		AttributeID: input.AttributeID,
		ChannelID:   uuid.NullUUID{UUID: *input.ChannelID, Valid: true},
		Slug:        input.Slug,
	})
	if err != nil {
		return ProductAttributeSlug{}, fmt.Errorf("upsert product attribute channel slug: %w", err)
	}
	return toProductAttributeSlug(row), nil
}

func (r *postgresRepository) DeleteProductAttributeSlug(ctx context.Context, productID, attributeID uuid.UUID, channelID *uuid.UUID) error {
	var ch uuid.NullUUID
	if channelID != nil {
		ch = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	if err := r.queries.DeleteProductAttributeSlugByProductAttribute(ctx, db.DeleteProductAttributeSlugByProductAttributeParams{
		ProductID:   productID,
		AttributeID: attributeID,
		ChannelID:   ch,
	}); err != nil {
		return fmt.Errorf("delete product attribute slug: %w", err)
	}
	return nil
}

func toProductAttributeSlug(row db.CatalogProductAttributeSlug) ProductAttributeSlug {
	var channelID *uuid.UUID
	if row.ChannelID.Valid {
		v := row.ChannelID.UUID
		channelID = &v
	}
	return ProductAttributeSlug{
		ID:          row.ID,
		ProductID:   row.ProductID,
		AttributeID: row.AttributeID,
		ChannelID:   channelID,
		Slug:        row.Slug,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// ==================== ProductAttributeLocaleValue ====================

func (r *postgresRepository) ListProductAttributeLocaleValues(ctx context.Context, productID uuid.UUID) ([]ProductAttributeLocaleValue, error) {
	rows, err := r.queries.ListProductAttributeLocaleValues(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product attribute locale values: %w", err)
	}
	out := make([]ProductAttributeLocaleValue, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeLocaleValue(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductAttributeLocaleValuesByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeLocaleValue, error) {
	if len(productIDs) == 0 {
		return []ProductAttributeLocaleValue{}, nil
	}
	rows, err := r.queries.ListProductAttributeLocaleValuesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list product attribute locale values by product ids: %w", err)
	}
	out := make([]ProductAttributeLocaleValue, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeLocaleValue(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertProductAttributeLocaleValue(ctx context.Context, input ProductAttributeLocaleValue) (ProductAttributeLocaleValue, error) {
	row, err := r.queries.UpsertProductAttributeLocaleValue(ctx, db.UpsertProductAttributeLocaleValueParams{
		ProductID:   input.ProductID,
		AttributeID: input.AttributeID,
		Locale:      input.Locale,
		Value:       input.Value,
	})
	if err != nil {
		return ProductAttributeLocaleValue{}, fmt.Errorf("upsert product attribute locale value: %w", err)
	}
	return toProductAttributeLocaleValue(row), nil
}

func (r *postgresRepository) DeleteProductAttributeLocaleValue(ctx context.Context, productID, attributeID uuid.UUID, locale string) error {
	if err := r.queries.DeleteProductAttributeLocaleValue(ctx, db.DeleteProductAttributeLocaleValueParams{
		ProductID:   productID,
		AttributeID: attributeID,
		Locale:      locale,
	}); err != nil {
		return fmt.Errorf("delete product attribute locale value: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteProductAttributeLocaleValuesForAttribute(ctx context.Context, productID, attributeID uuid.UUID) error {
	if err := r.queries.DeleteProductAttributeLocaleValuesForAttribute(ctx, db.DeleteProductAttributeLocaleValuesForAttributeParams{
		ProductID:   productID,
		AttributeID: attributeID,
	}); err != nil {
		return fmt.Errorf("delete product attribute locale values for attribute: %w", err)
	}
	return nil
}

func toProductAttributeLocaleValue(row db.CatalogProductAttributeLocaleValue) ProductAttributeLocaleValue {
	return ProductAttributeLocaleValue{
		ProductID:   row.ProductID,
		AttributeID: row.AttributeID,
		Locale:      row.Locale,
		Value:       row.Value,
		UpdatedAt:   row.UpdatedAt,
	}
}

// ==================== AttributeValueChannelSlug ====================

func (r *postgresRepository) ListAttributeValueChannelSlugsByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueChannelSlug, error) {
	if len(valueIDs) == 0 {
		return []AttributeValueChannelSlug{}, nil
	}
	rows, err := r.queries.ListAttributeValueChannelSlugsByValueIDs(ctx, valueIDs)
	if err != nil {
		return nil, fmt.Errorf("list attribute value channel slugs: %w", err)
	}
	out := make([]AttributeValueChannelSlug, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueChannelSlug(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAttributeValueChannelSlugsForChannel(ctx context.Context, channelID uuid.UUID) ([]AttributeValueChannelSlug, error) {
	rows, err := r.queries.ListAttributeValueChannelSlugsForChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list attribute value channel slugs for channel: %w", err)
	}
	out := make([]AttributeValueChannelSlug, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueChannelSlug(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertAttributeValueChannelSlug(ctx context.Context, valueID, attributeID, channelID uuid.UUID, slug string) (AttributeValueChannelSlug, error) {
	row, err := r.queries.UpsertAttributeValueChannelSlug(ctx, db.UpsertAttributeValueChannelSlugParams{
		ValueID:     valueID,
		AttributeID: attributeID,
		ChannelID:   channelID,
		Slug:        slug,
	})
	if err != nil {
		return AttributeValueChannelSlug{}, fmt.Errorf("upsert attribute value channel slug: %w", err)
	}
	return toAttributeValueChannelSlug(row), nil
}

func (r *postgresRepository) DeleteAttributeValueChannelSlug(ctx context.Context, valueID, channelID uuid.UUID) error {
	if err := r.queries.DeleteAttributeValueChannelSlug(ctx, db.DeleteAttributeValueChannelSlugParams{
		ValueID:   valueID,
		ChannelID: channelID,
	}); err != nil {
		return fmt.Errorf("delete attribute value channel slug: %w", err)
	}
	return nil
}

func toAttributeValueChannelSlug(row db.CatalogAttributeValueChannelSlug) AttributeValueChannelSlug {
	return AttributeValueChannelSlug{
		ID:          row.ID,
		ValueID:     row.ValueID,
		AttributeID: row.AttributeID,
		ChannelID:   row.ChannelID,
		Slug:        row.Slug,
		UpdatedAt:   row.UpdatedAt,
	}
}

// ==================== Assignment ====================

func (r *postgresRepository) AssignAttribute(ctx context.Context, input AssignAttributeInput) error {
	var groupID uuid.NullUUID
	if input.GroupID != nil {
		groupID = uuid.NullUUID{UUID: *input.GroupID, Valid: true}
	}
	if err := r.queries.AssignAttributeToProductType(ctx, db.AssignAttributeToProductTypeParams{
		ProductTypeID: input.ProductTypeID,
		AttributeID:   input.AttributeID,
		SortOrder:     input.SortOrder,
		GroupID:       groupID,
	}); err != nil {
		return fmt.Errorf("assign attribute: %w", err)
	}
	return nil
}

func (r *postgresRepository) MoveAttributeToGroup(ctx context.Context, productTypeID, attributeID uuid.UUID, groupID *uuid.UUID) error {
	var gid uuid.NullUUID
	if groupID != nil {
		gid = uuid.NullUUID{UUID: *groupID, Valid: true}
	}
	if err := r.queries.MoveAttributeToGroup(ctx, db.MoveAttributeToGroupParams{
		ProductTypeID: productTypeID,
		AttributeID:   attributeID,
		GroupID:       gid,
	}); err != nil {
		return fmt.Errorf("move attribute to group: %w", err)
	}
	return nil
}

func (r *postgresRepository) CreateAttributeGroup(ctx context.Context, input CreateAttributeGroupInput) (AttributeGroup, error) {
	row, err := r.queries.CreateAttributeGroup(ctx, db.CreateAttributeGroupParams{
		ProductTypeID: input.ProductTypeID,
		Name:          input.Name,
		SortOrder:     input.SortOrder,
	})
	if err != nil {
		return AttributeGroup{}, fmt.Errorf("create attribute group: %w", err)
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) GetAttributeGroupByID(ctx context.Context, id uuid.UUID) (AttributeGroup, error) {
	row, err := r.queries.GetAttributeGroupByID(ctx, id)
	if err != nil {
		return AttributeGroup{}, ErrAttributeGroupNotFound
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) ListAttributeGroupsByProductType(ctx context.Context, productTypeID uuid.UUID) ([]AttributeGroup, error) {
	rows, err := r.queries.ListAttributeGroupsByProductType(ctx, productTypeID)
	if err != nil {
		return nil, fmt.Errorf("list attribute groups: %w", err)
	}
	out := make([]AttributeGroup, len(rows))
	for i, row := range rows {
		out[i] = toAttributeGroup(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateAttributeGroup(ctx context.Context, input UpdateAttributeGroupInput) (AttributeGroup, error) {
	current, err := r.GetAttributeGroupByID(ctx, input.ID)
	if err != nil {
		return AttributeGroup{}, err
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	row, err := r.queries.UpdateAttributeGroup(ctx, db.UpdateAttributeGroupParams{
		ID:        input.ID,
		Name:      name,
		SortOrder: sortOrder,
	})
	if err != nil {
		return AttributeGroup{}, fmt.Errorf("update attribute group: %w", err)
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteAttributeGroup(ctx, id); err != nil {
		return fmt.Errorf("delete attribute group: %w", err)
	}
	return nil
}

func toAttributeGroup(row db.AttributeGroup) AttributeGroup {
	return AttributeGroup{
		ID:            row.ID,
		ProductTypeID: row.ProductTypeID,
		Name:          row.Name,
		SortOrder:     row.SortOrder,
		Attributes:    []ProductTypeAttribute{},
		CreatedAt:     row.CreatedAt,
	}
}

func (r *postgresRepository) UnassignAttribute(ctx context.Context, productTypeID, attributeID uuid.UUID) error {
	n, err := r.queries.UnassignAttributeFromProductType(ctx, db.UnassignAttributeFromProductTypeParams{
		ProductTypeID: productTypeID,
		AttributeID:   attributeID,
	})
	if err != nil {
		return fmt.Errorf("unassign attribute: %w", err)
	}
	if n == 0 {
		return ErrAssignmentNotFound
	}
	return nil
}

// ==================== Product Children ====================

func (r *postgresRepository) AddProductChild(ctx context.Context, parentID, childID uuid.UUID, sortOrder int32) error {
	if _, err := r.queries.AddProductChild(ctx, db.AddProductChildParams{
		ParentID:  parentID,
		ChildID:   childID,
		SortOrder: sortOrder,
	}); err != nil {
		return fmt.Errorf("add product child: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemoveProductChild(ctx context.Context, parentID, childID uuid.UUID) error {
	n, err := r.queries.RemoveProductChild(ctx, db.RemoveProductChildParams{
		ParentID: parentID,
		ChildID:  childID,
	})
	if err != nil {
		return fmt.Errorf("remove product child: %w", err)
	}
	if n == 0 {
		return ErrProductChildNotFound
	}
	return nil
}

func (r *postgresRepository) ListProductChildren(ctx context.Context, parentID uuid.UUID) ([]Product, error) {
	rows, err := r.queries.ListProductChildren(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("list product children: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductChildrenByParentIDs(ctx context.Context, parentIDs []uuid.UUID) (map[uuid.UUID][]Product, error) {
	rows, err := r.queries.ListProductChildrenByParentIDs(ctx, parentIDs)
	if err != nil {
		return nil, fmt.Errorf("list product children by parent ids: %w", err)
	}
	out := make(map[uuid.UUID][]Product)
	for _, row := range rows {
		out[row.ParentID] = append(out[row.ParentID], toProductFromChildRow(row))
	}
	return out, nil
}

func (r *postgresRepository) ListProductParents(ctx context.Context, childID uuid.UUID) ([]Product, error) {
	rows, err := r.queries.ListProductParents(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("list product parents: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) ListConfigurableParentByChildIDs(ctx context.Context, childIDs []uuid.UUID) (map[uuid.UUID]ConfigurableParentLink, error) {
	out := make(map[uuid.UUID]ConfigurableParentLink, len(childIDs))
	if len(childIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductChildLinksByChildIDs(ctx, childIDs)
	if err != nil {
		return nil, fmt.Errorf("list configurable parent links: %w", err)
	}
	for _, row := range rows {
		out[row.ChildID] = ConfigurableParentLink{ParentID: row.ParentID, VariantSortOrder: row.SortOrder}
	}
	return out, nil
}

// ==================== mappers ====================

func toProductType(row db.ProductType) ProductType {
	return ProductType{
		ID:                 row.ID,
		Slug:               row.Slug,
		Name:               row.Name,
		IsDigital:          row.IsDigital,
		IsShippingRequired: row.IsShippingRequired,
		URLTemplate:        row.UrlTemplate,
		TracksInventory:    row.TracksInventory,
		Metadata:           metadataFromJSON(row.Metadata),
		PrivateMetadata:    metadataFromJSON(row.PrivateMetadata),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func toAttribute(row db.Attribute) Attribute {
	return Attribute{
		ID:              row.ID,
		Slug:            row.Slug,
		Name:            row.Name,
		InputType:       InputType(row.InputType),
		Unit:            row.Unit,
		ValueRequired:   row.ValueRequired,
		Filterable:      row.Filterable,
		Searchable:      row.Searchable,
		IsPublic:        row.IsPublic,
		FacetCollapsed:  row.FacetCollapsed,
		IsIndexable:     row.IsIndexable,
		H1Position:      H1Position(row.H1Position),
		SynapseTrack:    row.SynapseTrack,
		AutoSegment:     row.AutoSegment,
		SortOrder:       row.SortOrder,
		Metadata:        metadataFromJSON(row.Metadata),
		PrivateMetadata: metadataFromJSON(row.PrivateMetadata),
		DefaultValue:    rawMessageFromNullable(row.DefaultValue),
		// "" when NULL (non-reference attributes)
		RelationshipTarget: ReferenceTarget(row.RelationshipTarget.String),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

// nullStringFromRefTarget maps a reference target to a nullable text column - empty target ("") stores NULL (every non-reference attribute)
func nullStringFromRefTarget(t ReferenceTarget) sql.NullString {
	if t == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(t), Valid: true}
}

func toAttributeValue(row db.AttributeValue) AttributeValue {
	return AttributeValue{
		ID:          row.ID,
		AttributeID: row.AttributeID,
		Slug:        row.Slug,
		Name:        row.Name,
		Value:       row.Value,
		SortOrder:   row.SortOrder,
		IsIndexable: row.IsIndexable,
		CreatedAt:   row.CreatedAt,
	}
}

// metadataToJSON serializes a Metadata map to JSONB - Nil/empty maps marshal to '{}' so the NOT NULL DEFAULT invariant stays true
func metadataToJSON(m Metadata) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// rawMessageToNullable converts a nullable JSON value coming from the domain layer (nil = no value) into the pqtype.NullRawMessage shape sqlc expects for a nullable JSONB column
func rawMessageToNullable(raw json.RawMessage) pqtype.NullRawMessage {
	if len(raw) == 0 {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

// rawMessageFromNullable converts a nullable JSONB row value back into the domain layer's nil-or-RawMessage convention
func rawMessageFromNullable(n pqtype.NullRawMessage) json.RawMessage {
	if !n.Valid {
		return nil
	}
	return n.RawMessage
}

func metadataFromJSON(raw json.RawMessage) Metadata {
	if len(raw) == 0 {
		return Metadata{}
	}
	var m Metadata
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return Metadata{}
	}
	return m
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
