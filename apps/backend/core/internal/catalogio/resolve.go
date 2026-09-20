package catalogio

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

// resolver turns the human-friendly references in a CSV (product-type slugs, attribute slugs, attribute value slugs/names, category slugs) into the IDs the catalog service needs - Every lookup is memoised for the lifetime of one import so a thousand rows sharing a product type cost one query
type resolver struct {
	cat *catalog.Service
	ch  *channel.Service

	typesLoaded  bool
	typeIDBySlug map[string]uuid.UUID
	typeByID     map[uuid.UUID]catalog.ProductType // with Attributes loaded

	attrsLoaded bool
	attrBySlug  map[string]catalog.Attribute       // InputType populated
	valuesByID  map[uuid.UUID]map[string]uuid.UUID // attrID → (value slug & lower(name)) → valueID

	catBySlug map[string]uuid.UUID

	// parentChildren caches a configurable parent's linked child IDs so a re-import doesn't re-link (and trip the
	// variant-uniqueness guard) and large parents aren't re-read once per child
	parentChildren map[uuid.UUID]map[uuid.UUID]bool
}

func newResolver(cat *catalog.Service, ch *channel.Service) *resolver {
	return &resolver{
		cat:            cat,
		ch:             ch,
		typeIDBySlug:   map[string]uuid.UUID{},
		typeByID:       map[uuid.UUID]catalog.ProductType{},
		attrBySlug:     map[string]catalog.Attribute{},
		valuesByID:     map[uuid.UUID]map[string]uuid.UUID{},
		catBySlug:      map[string]uuid.UUID{},
		parentChildren: map[uuid.UUID]map[uuid.UUID]bool{},
	}
}

// isLinked reports whether childID is already a child of parentID, loading and
// caching the parent's child set on first use
func (rs *resolver) isLinked(ctx context.Context, parentID, childID uuid.UUID) (bool, error) {
	set, ok := rs.parentChildren[parentID]
	if !ok {
		full, err := rs.cat.GetProductWithAttributes(ctx, parentID)
		if err != nil {
			return false, err
		}
		set = make(map[uuid.UUID]bool, len(full.Children))
		for _, c := range full.Children {
			set[c.ID] = true
		}
		rs.parentChildren[parentID] = set
	}
	return set[childID], nil
}

// markLinked records a freshly-linked child in the cache
func (rs *resolver) markLinked(parentID, childID uuid.UUID) {
	if set := rs.parentChildren[parentID]; set != nil {
		set[childID] = true
	}
}

func (rs *resolver) ensureTypes(ctx context.Context) error {
	if rs.typesLoaded {
		return nil
	}
	list, err := rs.cat.ListProductTypes(ctx)
	if err != nil {
		return fmt.Errorf("list product types: %w", err)
	}
	for _, t := range list {
		rs.typeIDBySlug[t.Slug] = t.ID
	}
	rs.typesLoaded = true
	return nil
}

func (rs *resolver) productTypeByID(ctx context.Context, id uuid.UUID) (catalog.ProductType, error) {
	if t, ok := rs.typeByID[id]; ok {
		return t, nil
	}
	t, err := rs.cat.GetProductType(ctx, id) // loads assigned attributes
	if err != nil {
		return catalog.ProductType{}, err
	}
	rs.typeByID[id] = t
	return t, nil
}

func (rs *resolver) productTypeBySlug(ctx context.Context, slug string) (catalog.ProductType, error) {
	if err := rs.ensureTypes(ctx); err != nil {
		return catalog.ProductType{}, err
	}
	id, ok := rs.typeIDBySlug[slug]
	if !ok {
		return catalog.ProductType{}, fmt.Errorf("unknown product_type %q", slug)
	}
	return rs.productTypeByID(ctx, id)
}

func (rs *resolver) ensureAttrs(ctx context.Context) error {
	if rs.attrsLoaded {
		return nil
	}
	list, err := rs.cat.ListAttributes(ctx)
	if err != nil {
		return fmt.Errorf("list attributes: %w", err)
	}
	for _, a := range list {
		rs.attrBySlug[a.Slug] = a
	}
	rs.attrsLoaded = true
	return nil
}

func (rs *resolver) attributeBySlug(ctx context.Context, slug string) (catalog.Attribute, error) {
	if err := rs.ensureAttrs(ctx); err != nil {
		return catalog.Attribute{}, err
	}
	a, ok := rs.attrBySlug[slug]
	if !ok {
		return catalog.Attribute{}, fmt.Errorf("unknown attribute %q", slug)
	}
	return a, nil
}

// valueID resolves one selectable-attribute value cell (a value slug or its display name) to an AttributeValue ID -
// When createMissing is set, an unmatched cell is created as a new value rather than rejected
func (rs *resolver) valueID(ctx context.Context, attr catalog.Attribute, cell string, createMissing bool) (uuid.UUID, error) {
	m, ok := rs.valuesByID[attr.ID]
	if !ok {
		full, err := rs.cat.GetAttribute(ctx, attr.ID) // hydrates Values
		if err != nil {
			return uuid.Nil, err
		}
		m = make(map[string]uuid.UUID, len(full.Values)*2)
		for _, v := range full.Values {
			m[v.Slug] = v.ID
			m[strings.ToLower(v.Name)] = v.ID
		}
		rs.valuesByID[attr.ID] = m
	}
	if id, ok := m[cell]; ok {
		return id, nil
	}
	if id, ok := m[strings.ToLower(cell)]; ok {
		return id, nil
	}
	if createMissing {
		v, err := rs.cat.AddAttributeValue(ctx, attr.ID, catalog.AttributeValueInput{
			Slug: slugify(cell),
			Name: cell,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("create value %q on attribute %q: %w", cell, attr.Slug, err)
		}
		m[v.Slug] = v.ID
		m[strings.ToLower(v.Name)] = v.ID
		return v.ID, nil
	}
	return uuid.Nil, fmt.Errorf("attribute %q has no value %q (enable create-missing-values to add it)", attr.Slug, cell)
}

func (rs *resolver) categoryBySlug(ctx context.Context, slug string) (uuid.UUID, error) {
	if id, ok := rs.catBySlug[slug]; ok {
		return id, nil
	}
	c, err := rs.cat.GetCategoryBySlug(ctx, slug)
	if err != nil {
		return uuid.Nil, fmt.Errorf("category %q: %w", slug, err)
	}
	rs.catBySlug[slug] = c.ID
	return c.ID, nil
}

// Builds the attribute writes from a row's non-empty attr:<slug> columns, validated against the product type's assigned attributes
// An empty cell leaves the value unchanged - a non-empty cell for an attribute not on the type is an error
func (rs *resolver) assignmentsForRow(ctx context.Context, row *dataio.Row, pt catalog.ProductType, createMissing bool) ([]catalog.AttributeAssignment, error) {
	ptAttrs := make(map[string]catalog.Attribute, len(pt.Attributes))
	for _, a := range pt.Attributes {
		ptAttrs[a.Attribute.Slug] = a.Attribute
	}

	var out []catalog.AttributeAssignment
	for _, h := range row.WithPrefix(attrPrefix) {
		cell := row.Str(h)
		if cell == "" {
			continue
		}
		slug := strings.TrimPrefix(h, attrPrefix)
		attr, ok := ptAttrs[slug]
		if !ok {
			return nil, fmt.Errorf("attribute %q is not assigned to product type %q", slug, pt.Slug)
		}
		if attr.InputType.AcceptsValues() {
			var ids []uuid.UUID
			for _, token := range splitMulti(cell) {
				id, err := rs.valueID(ctx, attr, token, createMissing)
				if err != nil {
					return nil, err
				}
				ids = append(ids, id)
			}
			out = append(out, catalog.AttributeAssignment{AttributeID: attr.ID, ValueIDs: ids})
		} else {
			v, err := coerceScalar(attr.InputType, cell)
			if err != nil {
				return nil, fmt.Errorf("attribute %q: %w", slug, err)
			}
			out = append(out, catalog.AttributeAssignment{AttributeID: attr.ID, ScalarValue: v, HasScalar: true})
		}
	}
	return out, nil
}
