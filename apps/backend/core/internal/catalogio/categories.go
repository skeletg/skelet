package catalogio

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

const (
	colCatSlug       = "slug"
	colCatName       = "name"
	colCatParent     = "parent_slug"
	colCatPosition   = "position"
	colCatPublished  = "published"
	colCatNoindex    = "seo_noindex"
	colCatMetaTitle  = "meta_title"
	colCatMetaDesc   = "meta_description"
	colCatURLPath    = "url_path"
	colCatProductCnt = "product_count"
)

var categoryFields = []dataio.Field{
	{Name: colCatSlug, Label: "Slug", Type: dataio.FieldText, Identifier: true, Required: true, Description: "The upsert key", Example: "running-shoes"},
	{Name: colCatName, Label: "Name", Type: dataio.FieldText, Required: true, Description: "Written under the run's locale", Example: "Running shoes"},
	{Name: colCatParent, Label: "Parent slug", Type: dataio.FieldText, Description: "Blank means a root category - the parent may appear anywhere in the file", Example: "shoes"},
	{Name: colCatPosition, Label: "Position", Type: dataio.FieldInteger, Description: "Sort order within the parent"},
	{Name: colCatPublished, Label: "Published", Type: dataio.FieldBool, Example: "true"},
	{Name: colCatNoindex, Label: "SEO noindex", Type: dataio.FieldBool},
	{Name: colCatMetaTitle, Label: "Meta title", Type: dataio.FieldText},
	{Name: colCatMetaDesc, Label: "Meta description", Type: dataio.FieldText},
	{Name: colCatURLPath, Label: "URL path", Type: dataio.FieldText, ReadOnly: true, Description: "Derived from the tree - move a category by changing its parent, not this"},
	{Name: colCatProductCnt, Label: "Products", Type: dataio.FieldInteger, ReadOnly: true},
}

// CategoryResource is the category tree, upserted by slug. Parents are resolved by slug too, so a file can
// describe a whole hierarchy in any row order
func CategoryResource(cat *catalog.Service) dataio.Resource {
	return dataio.Resource{
		Slug:                resourceCategories,
		Label:               "Categories",
		Group:               "Catalog",
		Description:         "The category tree. Rows link to their parent by slug, so the file can arrive in any order.",
		ViewPermission:      permView,
		ImportPermission:    permManage,
		RequireGlobalImport: true,
		Fields:              categoryFields,
		NewExporter: func(ctx context.Context, o dataio.Opts) (dataio.Exporter, error) {
			locale, err := runLocale(ctx, cat, o)
			if err != nil {
				return nil, err
			}
			return &categoryExporter{cat: cat, locale: locale}, nil
		},
		NewImporter: func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) {
			locale, err := runLocale(ctx, cat, o)
			if err != nil {
				return nil, err
			}
			return &categoryImporter{cat: cat, opts: o, locale: locale, bySlug: map[string]uuid.UUID{}}, nil
		},
	}
}

type categoryExporter struct {
	cat    *catalog.Service
	locale string
	flat   []catalog.Category
	counts map[uuid.UUID]int64
}

// One read of the whole tree plus one grouped count, walked depth-first so a parent always precedes its children
func (e *categoryExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	roots, err := e.cat.ListRootCategories(ctx, e.locale)
	if err != nil {
		return nil, err
	}
	var walk func(nodes []catalog.Category)
	walk = func(nodes []catalog.Category) {
		for _, n := range nodes {
			kids := n.Children
			n.Children = nil
			e.flat = append(e.flat, n)
			walk(kids)
		}
	}
	walk(roots)
	ids := make([]uuid.UUID, len(e.flat))
	for i, c := range e.flat {
		ids[i] = c.ID
	}
	if e.counts, err = e.cat.CountProductsInCategories(ctx, ids); err != nil {
		return nil, fmt.Errorf("count category products: %w", err)
	}
	return nil, nil
}

func (e *categoryExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	slugByID := make(map[uuid.UUID]string, len(e.flat))
	for _, c := range e.flat {
		slugByID[c.ID] = c.Slug
	}
	for _, c := range e.flat {
		r := dataio.NewRow()
		r.Ref = c.Slug
		r.Set(colCatSlug, c.Slug)
		if c.Content != nil {
			r.Set(colCatName, c.Content.Name)
			if c.Content.MetaTitle != nil {
				r.Set(colCatMetaTitle, *c.Content.MetaTitle)
			}
			if c.Content.MetaDescription != nil {
				r.Set(colCatMetaDesc, *c.Content.MetaDescription)
			}
		}
		if c.ParentID != nil {
			r.Set(colCatParent, slugByID[*c.ParentID])
		}
		r.Set(colCatPosition, c.Position)
		r.Set(colCatPublished, c.IsPublished)
		r.Set(colCatNoindex, c.SEONoindex)
		r.Set(colCatURLPath, c.URLPath)
		r.Set(colCatProductCnt, e.counts[c.ID])
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

type categoryImporter struct {
	cat    *catalog.Service
	opts   dataio.Opts
	locale string
	bySlug map[string]uuid.UUID // resolved this run, so a child finds a parent created moments earlier
}

const (
	passCatRoots    = 0
	passCatChildren = 1
)

// Roots first - a child whose parent row hasn't landed yet defers until it has, so any depth resolves in any row order
func (im *categoryImporter) Pass(r *dataio.Row) int {
	if r.Has(colCatParent) {
		return passCatChildren
	}
	return passCatRoots
}

func (im *categoryImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	slug := r.Str(colCatSlug)
	r.Ref = slug
	if slug == "" {
		return dataio.OutcomeSkipped, errors.New("missing slug")
	}

	var parentID *uuid.UUID
	if p := r.Str(colCatParent); p != "" {
		if p == slug {
			return dataio.OutcomeSkipped, errors.New("a category cannot be its own parent")
		}
		id, err := im.resolve(ctx, p)
		if errors.Is(err, catalog.ErrCategoryNotFound) {
			return dataio.OutcomeSkipped, dataio.Defer(fmt.Errorf("parent %q not found (not in the catalog or this file)", p))
		}
		if err != nil {
			return dataio.OutcomeSkipped, fmt.Errorf("parent %q: %w", p, err)
		}
		parentID = &id
	}

	position := r.Int32(colCatPosition)
	published := r.OptBool(colCatPublished)
	noindex := r.OptBool(colCatNoindex)
	if err := r.Err(); err != nil {
		return dataio.OutcomeSkipped, err
	}

	existing, getErr := im.cat.GetCategoryBySlug(ctx, slug)
	if getErr != nil && !errors.Is(getErr, catalog.ErrCategoryNotFound) {
		return dataio.OutcomeSkipped, fmt.Errorf("lookup by slug: %w", getErr)
	}
	exists := getErr == nil
	if !exists && !r.RequireDeclared() {
		return dataio.OutcomeSkipped, r.Err()
	}
	if im.opts.DryRun {
		if exists {
			return dataio.OutcomeUpdated, nil
		}
		im.bySlug[slug] = uuid.New() // a placeholder so a child row in this same file still resolves
		return dataio.OutcomeCreated, nil
	}

	locale := im.locale
	if exists {
		upd := catalog.UpdateCategoryInput{ID: existing.ID, ParentID: parentID, Position: position, IsPublished: published, SEONoindex: noindex}
		if parentID == nil && r.Column(colCatParent) && !r.Has(colCatParent) {
			upd.ClearParent = true // an explicitly blank cell moves the category to the root
		}
		if _, err := im.cat.UpdateCategory(ctx, upd); err != nil {
			return dataio.OutcomeSkipped, err
		}
		im.bySlug[slug] = existing.ID
		if err := im.applyContent(ctx, existing.ID, locale, r); err != nil {
			return dataio.OutcomeSkipped, err
		}
		return dataio.OutcomeUpdated, nil
	}

	in := catalog.CreateCategoryInput{
		ParentID:        parentID,
		Slug:            slug,
		Locale:          locale,
		Name:            r.Str(colCatName),
		MetaTitle:       r.Str(colCatMetaTitle),
		MetaDescription: r.Str(colCatMetaDesc),
	}
	if position != nil {
		in.Position = *position
	}
	if published != nil {
		in.IsPublished = *published
	}
	if noindex != nil {
		in.SEONoindex = *noindex
	}
	created, err := im.cat.CreateCategory(ctx, in)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	im.bySlug[slug] = created.ID
	return dataio.OutcomeCreated, nil
}

// Preserves the locale's existing descriptions - the file carries no rich text, so a naive upsert would wipe them
func (im *categoryImporter) applyContent(ctx context.Context, id uuid.UUID, locale string, r *dataio.Row) error {
	if !r.Has(colCatName) && !r.Has(colCatMetaTitle) && !r.Has(colCatMetaDesc) {
		return nil
	}
	full, err := im.cat.GetCategory(ctx, id, locale)
	if err != nil {
		return err
	}
	in := catalog.CategoryContentInput{Locale: locale}
	for _, c := range full.AllContent {
		if c.Locale != locale {
			continue
		}
		in.Name = c.Name
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
	if v := r.Str(colCatName); v != "" {
		in.Name = v
	}
	if v := r.Str(colCatMetaTitle); v != "" {
		in.MetaTitle = v
	}
	if v := r.Str(colCatMetaDesc); v != "" {
		in.MetaDescription = v
	}
	_, err = im.cat.UpsertCategoryContent(ctx, id, in)
	return err
}

func (im *categoryImporter) resolve(ctx context.Context, slug string) (uuid.UUID, error) {
	if id, ok := im.bySlug[slug]; ok {
		return id, nil
	}
	c, err := im.cat.GetCategoryBySlug(ctx, slug)
	if err != nil {
		return uuid.Nil, err
	}
	im.bySlug[slug] = c.ID
	return c.ID, nil
}
