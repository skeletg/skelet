package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

const (
	categoryTreeCacheTTL    = 20 * time.Minute
	categoryTreeCachePrefix = "tree:"
	categoryCoverKeyFormat  = "catalog/categories/%s/cover/%s%s"
	maxFacetOverrides       = 200 // far past any real facet sidebar, low enough that one save stays one small tx
)

var categoryEntityType = string(urlrewrite.EntityTypeCategory)

func categoryCoverKey(categoryID, coverID uuid.UUID, ext string) string {
	return fmt.Sprintf(categoryCoverKeyFormat, categoryID, coverID, ext)
}

// ==================== Category CRUD ====================

func (s *categoryService) CreateCategory(ctx context.Context, input CreateCategoryInput) (Category, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Category{}, err
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Category{}, ErrLocaleRequired
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return Category{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if strings.TrimSpace(input.Name) == "" {
		return Category{}, ErrNameRequired
	}

	// Build the URL path from parent's path + own slug
	urlPath := input.Slug
	if input.ParentID != nil {
		parent, err := s.repo.GetCategoryByID(ctx, *input.ParentID)
		if err != nil {
			return Category{}, fmt.Errorf("get parent category: %w", err)
		}
		if err := checkCategoryDepth(ctx, s.repo, parent.ID, 0); err != nil {
			return Category{}, err
		}
		urlPath = parent.URLPath + "/" + input.Slug
	}

	var created Category
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		c, err := r.CreateCategory(ctx, input)
		if err != nil {
			return err
		}
		if err := r.UpdateCategoryURLPath(ctx, c.ID, urlPath); err != nil {
			return err
		}
		if _, err := r.UpsertCategoryContent(ctx, c.ID, CategoryContentInput{
			Locale:           input.Locale,
			Name:             input.Name,
			ShortDescription: input.ShortDescription,
			Description:      input.Description,
			MetaTitle:        input.MetaTitle,
			MetaDescription:  input.MetaDescription,
			MetaKeywords:     input.MetaKeywords,
		}); err != nil {
			return err
		}
		created = c
		created.URLPath = urlPath
		return nil
	}); err != nil {
		return Category{}, err
	}

	if s.urlRewrites != nil {
		// Best-effort: a stale canonical doesn't undo the create (the category exists - the redirect can be re-synced), but we MUST log so a quietly-broken redirect surfaces in ops
		if err := s.urlRewrites.SyncCanonical(ctx, categoryEntityType, created.ID, urlPath); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite",
				"op", "catalog.Service.CreateCategory",
				"category_id", created.ID,
				"url_path", urlPath,
				"err", err,
			)
		}
	}

	s.invalidateCategoryTreeCache(ctx)
	if s.bus != nil {
		s.bus.Publish(ctx, &CategoryCreated{
			CategoryID: created.ID,
			Slug:       created.Slug,
			ParentID:   created.ParentID,
		})
	}
	return s.repo.GetCategoryByID(ctx, created.ID)
}

func (s *categoryService) GetCategory(ctx context.Context, id uuid.UUID, locale string) (Category, error) {
	const op = "catalog.Service.GetCategory"
	c, err := s.repo.GetCategoryByID(ctx, id)
	if err != nil {
		if apperr.IsNotFound(err) {
			return Category{}, err
		}
		return Category{}, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	if locale != "" {
		// Locale content is optional - a missing translation is not an error path, but a real DB failure here MUST propagate instead of silently leaving Content nil
		content, err := s.repo.GetCategoryContent(ctx, id, locale)
		switch {
		case err == nil:
			cp := content
			c.Content = &cp
		case apperr.IsNotFound(err):
			// expected: this locale has no translation row - fall through to the default-locale fallback below
		default:
			return Category{}, apperr.WrapClass(err, apperr.ClassInternal, op)
		}
	}
	if c.Content == nil {
		if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
			content, err := s.repo.GetCategoryContent(ctx, id, def)
			switch {
			case err == nil:
				cp := content
				c.Content = &cp
			case apperr.IsNotFound(err):
				// expected: even the default locale has no row - caller gets a Category with Content == nil
			default:
				return Category{}, apperr.WrapClass(err, apperr.ClassInternal, op)
			}
		}
	}
	s.resolveCategoryCoverURL(&c)
	// Eager-load per-store listings so the admin Category.listings field is populated without a follow-up call - Storefront paths use the listing-aware reader instead and never go through this method
	listings, err := s.repo.ListCategoryChannelListings(ctx, id)
	if err != nil {
		return Category{}, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	c.ChannelListings = listings
	return c, nil
}

// GetCategoryBySlug resolves a category by its (globally unique) slug - Thin pass-through to the repository - used by bulk import/export (catalogio) to turn human-friendly category slugs into IDs - Returns ErrCategoryNotFound when no category carries the slug
func (s *categoryService) GetCategoryBySlug(ctx context.Context, slug string) (Category, error) {
	return s.repo.GetCategoryBySlug(ctx, slug)
}

// ListCategoryTranslations returns every locale's content row for each category, in one query - list paths only carry the resolved locale, so a caller wanting the full set (the admin Category.content field) reads it here
func (s *categoryService) ListCategoryTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CategoryContent, error) {
	return s.repo.ListCategoryContentForCategories(ctx, ids)
}

// CountProductsInCategory returns how many products are directly assigned to a category (its own membership - descendant categories are not rolled up) - Backs the admin Category.productCount field
func (s *categoryService) CountProductsInCategory(ctx context.Context, id uuid.UUID) (int64, error) {
	return s.repo.CountProductsInCategory(ctx, id)
}

// The batched form for a rendered tree - a category with no members is absent from the map
func (s *categoryService) CountProductsInCategories(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	return s.repo.CountProductsByCategoryIDs(ctx, ids)
}

func (s *categoryService) ListRootCategories(ctx context.Context, locale string) ([]Category, error) {
	all, err := s.repo.ListAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyCategoryContent(ctx, all, locale, false); err != nil {
		return nil, err
	}
	return buildCategoryTree(all, nil), nil
}

// Nests a flat list under its parents in one pass, keeping input order among siblings - parentID nil collects the roots - a node whose parent isn't in the list is dropped, and so is anything on a parent cycle, since no root reaches it
func buildCategoryTree(all []Category, parentID *uuid.UUID) []Category {
	byParent := make(map[uuid.UUID][]int, len(all))
	var roots []int
	for i, c := range all {
		if c.ParentID == nil {
			roots = append(roots, i)
			continue
		}
		byParent[*c.ParentID] = append(byParent[*c.ParentID], i)
	}
	placed := make([]bool, len(all)) // a subtree rooted on a cycle would otherwise recurse forever
	var build func(idx []int) []Category
	build = func(idx []int) []Category {
		var out []Category
		for _, i := range idx {
			if placed[i] {
				continue
			}
			placed[i] = true
			c := all[i]
			c.Children = build(byParent[c.ID])
			out = append(out, c)
		}
		return out
	}
	if parentID != nil {
		return build(byParent[*parentID])
	}
	return build(roots)
}

func (s *categoryService) ListCategoryChildren(ctx context.Context, parentID uuid.UUID, locale string) ([]Category, error) {
	children, err := s.repo.ListCategoryChildren(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if err := s.applyCategoryContent(ctx, children, locale, false); err != nil {
		return nil, err
	}
	return children, nil
}

// GetCategoryAncestors returns the root→immediate-parent chain - When locale is non-empty, each ancestor's Content is populated with the locale row (with a fallback to the global default locale), so the GraphQL layer can return a non-null name for ancestor entries - Without the fallback, ancestor names come back null and the dashboard tree falls back to rendering the slug after the cache merges those ids in
func (s *categoryService) GetCategoryAncestors(ctx context.Context, id uuid.UUID, locale string) ([]Category, error) {
	ancestors, err := s.repo.GetCategoryAncestors(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(ancestors) == 0 || locale == "" {
		return ancestors, nil
	}
	byID, err := s.resolveCategoryContent(ctx, categoryIDs(ancestors), locale, false)
	if err != nil {
		return nil, err
	}
	setCategoryContent(ancestors, byID)
	return ancestors, nil
}

func (s *categoryService) ListCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID) ([]FacetOverride, error) {
	return s.repo.ListCategoryFacetOverrides(ctx, categoryID)
}

// SetCategoryFacetOverrides replaces the category's whole override set - positions are re-numbered from the slice order so callers can't hand in colliding sort keys, and an empty slice resets the category to attribute-order defaults
func (s *categoryService) SetCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID, overrides []FacetOverride) error {
	if err := checkFacetOverrides(overrides); err != nil {
		return err
	}
	c, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return err
	}
	if err := s.repo.ReplaceCategoryFacetOverrides(ctx, categoryID, overrides); err != nil {
		return err
	}
	s.categoryChanged(ctx, c, "facet_overrides")
	return nil
}

// Shared by both facet-override setters: bounded, no duplicate attribute, positions re-numbered from the slice order
func checkFacetOverrides(overrides []FacetOverride) error {
	if len(overrides) > maxFacetOverrides {
		return fmt.Errorf("%w: %d facet overrides, at most %d", ErrTooManyItems, len(overrides), maxFacetOverrides)
	}
	seen := make(map[uuid.UUID]struct{}, len(overrides))
	for i := range overrides {
		if _, dup := seen[overrides[i].AttributeID]; dup {
			return ErrDuplicateFacetOverride
		}
		seen[overrides[i].AttributeID] = struct{}{}
		overrides[i].SortOrder = int32(i)
	}
	return nil
}

// A slug or parent change moves this category's url_path and every descendant's in one tx, together with the outbox row that later re-renders the products under them
func (s *categoryService) UpdateCategory(ctx context.Context, input UpdateCategoryInput) (Category, error) {
	if input.Slug != nil {
		if err := validateSlug(*input.Slug); err != nil {
			return Category{}, err
		}
	}
	reparent := input.ParentID != nil && !input.ClearParent
	if reparent && *input.ParentID == input.ID {
		return Category{}, ErrCategoryCycle
	}

	var updated Category
	var moves []CategoryPathMove
	relocated := false
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if input.Slug != nil || input.ParentID != nil || input.ClearParent {
			if err := r.LockCategoryTree(ctx); err != nil {
				return err
			}
		}
		current, err := r.GetCategoryByID(ctx, input.ID)
		if err != nil {
			return err
		}
		if reparent && !uuidPtrEqual(current.ParentID, input.ParentID) {
			if err := checkCategoryPlacement(ctx, r, input.ID, *input.ParentID); err != nil {
				return err
			}
		}
		if updated, err = r.UpdateCategory(ctx, input); err != nil {
			return err
		}
		if updated.Slug == current.Slug && uuidPtrEqual(current.ParentID, updated.ParentID) {
			return nil
		}
		relocated = true
		ancestors, err := r.GetCategoryAncestors(ctx, updated.ID)
		if err != nil {
			return err
		}
		if moves, err = r.RewriteCategorySubtreeURLPaths(ctx, updated.ID, buildCategoryURLPath(ancestors, updated.Slug)); err != nil {
			return err
		}
		return s.enqueueCategoryMoved(ctx, t, &CategoryMoved{CategoryID: updated.ID})
	}); err != nil {
		return Category{}, err
	}

	s.syncMovedCategoryPaths(ctx, moves)
	if relocated && s.outbox == nil {
		s.publishCategoryMovedUndurably(ctx, &CategoryMoved{CategoryID: updated.ID})
	}
	s.invalidateCategoryTreeCache(ctx)
	if s.bus != nil {
		changed := changedCategoryFields(input)
		for _, m := range moves {
			if m.ID == updated.ID {
				changed = append(changed, "url_path")
				continue
			}
			s.bus.Publish(ctx, &CategoryUpdated{CategoryID: m.ID, Slug: m.Slug, ParentID: m.ParentID, ChangedFields: []string{"url_path"}})
		}
		s.bus.Publish(ctx, &CategoryUpdated{
			CategoryID:    updated.ID,
			Slug:          updated.Slug,
			ParentID:      updated.ParentID,
			ChangedFields: changed,
		})
	}
	return s.repo.GetCategoryByID(ctx, updated.ID)
}

// Every field UpdateCategoryInput can carry, named the way the category webhooks always have
func changedCategoryFields(in UpdateCategoryInput) []string {
	var changed []string
	add := func(set bool, name string) {
		if set {
			changed = append(changed, name)
		}
	}
	add(in.Slug != nil, "slug")
	add(in.ParentID != nil || in.ClearParent, "parent_id")
	add(in.Position != nil, "position")
	add(in.IsPublished != nil, "is_published")
	add(in.SEONoindex != nil, "seo_noindex")
	add(in.Metadata != nil, "metadata")
	add(in.PrivateMetadata != nil, "private_metadata")
	return changed
}

// A parent inside the moved subtree closes a loop, and the subtree's deepest leaf must still fit under the depth cap
func checkCategoryPlacement(ctx context.Context, r Repository, id, parentID uuid.UUID) error {
	if parentID == id {
		return ErrCategoryCycle
	}
	chain, err := r.GetCategoryAncestors(ctx, parentID)
	if err != nil {
		return err
	}
	for _, a := range chain {
		if a.ID == id {
			return ErrCategoryCycle
		}
	}
	height, err := r.CategorySubtreeHeight(ctx, id)
	if err != nil {
		return err
	}
	return categoryDepthFits(len(chain), height)
}

func checkCategoryDepth(ctx context.Context, r Repository, parentID uuid.UUID, height int32) error {
	chain, err := r.GetCategoryAncestors(ctx, parentID)
	if err != nil {
		return err
	}
	return categoryDepthFits(len(chain), height)
}

// The parent sits at level len(chain), its new child one below, and the child's own subtree hangs height levels further
func categoryDepthFits(parentChain int, height int32) error {
	if parentChain+1+int(height) >= maxCategoryDepth {
		return ErrCategoryTooDeep
	}
	return nil
}

// Best-effort and after commit: a stale canonical or missing redirect is an SEO problem, not a reason to undo the move - logged so it surfaces in ops
// Canonical first, since until it moves the entity's own row still holds the old path and the redirect insert would die on the unique index
func (s *categoryService) syncMovedCategoryPaths(ctx context.Context, moves []CategoryPathMove) {
	if s.urlRewrites == nil {
		return
	}
	for _, m := range moves {
		if err := s.urlRewrites.SyncCanonical(ctx, categoryEntityType, m.ID, m.NewPath); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite",
				"op", "catalog.Service.UpdateCategory",
				"category_id", m.ID,
				"url_path", m.NewPath,
				"err", err)
		}
		s.createRenameRedirect(ctx, m.OldPath, categoryEntityType, m.ID)
	}
}

// The children/products checks and the delete share one tx under a row lock, so a child or a product assigned meanwhile waits on it instead of slipping in between
func (s *categoryService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	var snap Category
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		var err error
		if snap, err = r.LockCategoryByID(ctx, id); err != nil {
			return err
		}
		children, err := r.ListCategoryChildren(ctx, id)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			return ErrCategoryHasChildren
		}
		n, err := r.CountProductsInCategory(ctx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrCategoryHasProducts
		}
		return r.DeleteCategory(ctx, id)
	}); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		if err := s.urlRewrites.DeleteAllForEntity(ctx, categoryEntityType, id); err != nil {
			applog.From(ctx).Error("delete url rewrites",
				"op", "catalog.Service.DeleteCategory",
				"category_id", id,
				"err", err)
		}
	}
	s.invalidateCategoryTreeCache(ctx)
	if s.bus != nil {
		s.bus.Publish(ctx, &CategoryDeleted{CategoryID: id, Slug: snap.Slug})
	}
	return nil
}

// Every storefront-visible change that isn't a field update - the tree cache drops and the category's pages purge
func (s *categoryService) categoryChanged(ctx context.Context, c Category, changed ...string) {
	s.invalidateCategoryTreeCache(ctx)
	if s.bus != nil {
		s.bus.Publish(ctx, &CategoryUpdated{CategoryID: c.ID, Slug: c.Slug, ParentID: c.ParentID, ChangedFields: changed})
	}
}

// ==================== Category Content ====================

func (s *categoryService) UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, input CategoryContentInput) (Category, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Category{}, ErrNameRequired
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Category{}, ErrLocaleRequired
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, input.Locale); err != nil {
			return Category{}, err
		}
	}
	if _, err := s.repo.GetCategoryByID(ctx, categoryID); err != nil {
		return Category{}, err
	}
	if _, err := s.repo.UpsertCategoryContent(ctx, categoryID, input); err != nil {
		return Category{}, err
	}
	c, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	s.categoryChanged(ctx, c, "content")
	return c, nil
}

func (s *categoryService) DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error {
	if def := s.resolveDefaultLocale(ctx); def != "" && locale == def {
		all, err := s.repo.ListCategoryContent(ctx, categoryID)
		if err != nil {
			return err
		}
		for _, c := range all {
			if c.Locale != def {
				return ErrCannotDeleteDefaultLocale
			}
		}
	}
	c, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCategoryContent(ctx, categoryID, locale); err != nil {
		return err
	}
	s.categoryChanged(ctx, c, "content")
	return nil
}

// ==================== Product-Category ====================

// Re-adding an existing member updates its position, and with isPrimary also makes it the primary
func (s *categoryService) AddProductToCategory(ctx context.Context, productID, categoryID uuid.UUID, position int32, isPrimary bool) (Product, error) {
	if _, err := s.repo.GetCategoryByID(ctx, categoryID); err != nil {
		return Product{}, err
	}
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return Product{}, err
	}
	if isPrimary {
		// Clear any existing primary before inserting to avoid unique constraint violation - wrap in a single tx so there's no window with zero primaries
		if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
			r := s.repo.WithTx(t)
			if err := r.AddProductToCategory(ctx, productID, categoryID, position, false); err != nil {
				return err
			}
			return r.SetPrimaryProductCategory(ctx, productID, categoryID)
		}); err != nil {
			return Product{}, err
		}
		// Primary changed → recompute URL path (strict)
		if err := s.regenerateProductURLPath(ctx, productID, true); err != nil {
			return Product{}, err
		}
	} else {
		if err := s.repo.AddProductToCategory(ctx, productID, categoryID, position, false); err != nil {
			return Product{}, err
		}
	}
	out, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	// the indexed doc carries category_ids and category pages filter on it, so membership has to rebuild it
	changed := []string{"categories"}
	if isPrimary {
		changed = append(changed, "primaryCategory")
	}
	s.publishProductUpdated(ctx, out, changed...)
	return out, nil
}

func (s *categoryService) RemoveProductFromCategory(ctx context.Context, productID, categoryID uuid.UUID) (Product, error) {
	// If the category being removed is the product's primary, we'll need to recompute the URL path afterwards (which may fail strict validation)
	primaryByID, err := s.repo.ListPrimaryCategoryByProductIDs(ctx, []uuid.UUID{productID})
	if err != nil {
		return Product{}, err
	}
	cur := primaryByID[productID]
	wasPrimary := cur != nil && cur.ID == categoryID
	if err := s.repo.RemoveProductFromCategory(ctx, productID, categoryID); err != nil {
		return Product{}, err
	}
	if wasPrimary {
		if err := s.regenerateProductURLPath(ctx, productID, true); err != nil {
			return Product{}, err
		}
	}
	out, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	changed := []string{"categories"}
	if wasPrimary {
		changed = append(changed, "primaryCategory")
	}
	s.publishProductUpdated(ctx, out, changed...)
	return out, nil
}

func (s *categoryService) SetProductPrimaryCategory(ctx context.Context, productID, categoryID uuid.UUID) (Product, error) {
	// Validate the product is already in the category
	categories, err := s.repo.ListCategoriesByProduct(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	found := false
	for _, c := range categories {
		if c.ID == categoryID {
			found = true
			break
		}
	}
	if !found {
		return Product{}, ErrCategoryProductNotFound
	}
	if err := s.repo.SetPrimaryProductCategory(ctx, productID, categoryID); err != nil {
		return Product{}, err
	}
	if err := s.regenerateProductURLPath(ctx, productID, true); err != nil {
		return Product{}, err
	}
	out, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	// primary drives the product's url path, which the doc carries
	s.publishProductUpdated(ctx, out, "primaryCategory")
	return out, nil
}

// ListCategoriesByProduct returns the categories a single product is assigned to - Thin wrapper around the repo method - surfaced here so the search indexer can satisfy its ProductSource interface without the search package importing the repository directly
func (s *categoryService) ListCategoriesByProduct(ctx context.Context, productID uuid.UUID) ([]Category, error) {
	return s.repo.ListCategoriesByProduct(ctx, productID)
}

func (s *categoryService) ListCategoriesByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]Category, error) {
	byProduct, err := s.repo.ListCategoriesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{})
	var ids []uuid.UUID
	for _, cats := range byProduct {
		for _, c := range cats {
			if _, ok := seen[c.ID]; !ok {
				ids = append(ids, c.ID)
				seen[c.ID] = struct{}{}
			}
		}
	}
	byID, err := s.resolveCategoryContent(ctx, ids, locale, false)
	if err != nil {
		return nil, err
	}
	for _, cats := range byProduct {
		setCategoryContent(cats, byID)
		for i := range cats {
			s.resolveCategoryCoverURL(&cats[i])
		}
	}
	return byProduct, nil
}

func (s *categoryService) ListPrimaryCategoryByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]*Category, error) {
	result, err := s.repo.ListPrimaryCategoryByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	for pid, c := range result {
		s.resolveCategoryCoverURL(c)
		result[pid] = c
	}
	return result, nil
}

// ==================== Cover image ====================

func (s *categoryService) RequestCategoryCoverUpload(ctx context.Context, input RequestCategoryCoverInput) (RequestCategoryCoverResult, error) {
	if s.media == nil {
		return RequestCategoryCoverResult{}, fmt.Errorf("media storage not configured")
	}
	if !media.IsAllowedImage(input.MimeType) {
		return RequestCategoryCoverResult{}, fmt.Errorf("%w: %s", ErrInvalidMimeType, input.MimeType)
	}
	if input.SizeBytes <= 0 {
		return RequestCategoryCoverResult{}, ErrInvalidSize
	}
	if input.SizeBytes > s.media.MaxUploadBytes() {
		return RequestCategoryCoverResult{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, input.SizeBytes, s.media.MaxUploadBytes())
	}
	if _, err := s.repo.GetCategoryByID(ctx, input.CategoryID); err != nil {
		return RequestCategoryCoverResult{}, err
	}

	coverID, err := uuid.NewV7()
	if err != nil {
		return RequestCategoryCoverResult{}, fmt.Errorf("generate cover id: %w", err)
	}
	s3Key := categoryCoverKey(input.CategoryID, coverID, media.ExtensionForMime(input.MimeType))

	uploadURL, err := s.media.PresignUpload(ctx, s3Key, input.MimeType, input.SizeBytes, presignTTL)
	if err != nil {
		return RequestCategoryCoverResult{}, fmt.Errorf("presign: %w", err)
	}

	c, err := s.repo.SetCategoryCover(ctx, input.CategoryID, coverID, s3Key, input.MimeType, input.SizeBytes, input.AltText)
	if err != nil {
		return RequestCategoryCoverResult{}, err
	}
	if c.CoverImage != nil {
		c.CoverImage.URL = s.media.PublicURL(s3Key)
	}
	s.categoryChanged(ctx, c, "cover")

	return RequestCategoryCoverResult{
		Category:  c,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().Add(presignTTL),
	}, nil
}

// The confirm is pinned to the cover checked here, so a re-upload that replaced it in between reads as not pending rather than confirming the wrong object
func (s *categoryService) ConfirmCategoryCover(ctx context.Context, categoryID uuid.UUID, width, height int32) (Category, error) {
	if width <= 0 || height <= 0 {
		return Category{}, ErrInvalidDimensions
	}
	current, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	if current.CoverImage == nil {
		return Category{}, ErrCategoryNoCover
	}
	if current.CoverImage.Status != string(MediaStatusPending) {
		return Category{}, ErrCategoryCoverNotPending
	}
	if s.media != nil {
		_, _, exists, err := s.media.Head(ctx, current.CoverImage.S3Key)
		if err != nil {
			return Category{}, fmt.Errorf("%w: %w", ErrUploadNotReceived, err)
		}
		if !exists {
			return Category{}, ErrUploadNotReceived
		}
	}
	c, err := s.repo.ConfirmCategoryCover(ctx, categoryID, current.CoverImage.ID, width, height)
	if err != nil {
		return Category{}, err
	}
	s.resolveCategoryCoverURL(&c)
	s.categoryChanged(ctx, c, "cover")
	return c, nil
}

func (s *categoryService) ClearCategoryCover(ctx context.Context, categoryID uuid.UUID) (Category, error) {
	c, err := s.repo.ClearCategoryCover(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	s.categoryChanged(ctx, c, "cover")
	return c, nil
}

// ==================== Helpers ====================

// uuidPtrEqual compares two nullable uuid pointers by value - Both nil is equal, one-nil-one-not is unequal, both set is value comparison - Used by UpdateCategory to detect parent_id changes without false positives when ParentID isn't being touched
func uuidPtrEqual(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func buildCategoryURLPath(ancestors []Category, slug string) string {
	if len(ancestors) == 0 {
		return slug
	}
	parts := make([]string, 0, len(ancestors)+1)
	for _, a := range ancestors {
		parts = append(parts, a.Slug)
	}
	parts = append(parts, slug)
	return strings.Join(parts, "/")
}

func (s *categoryService) resolveCategoryCoverURL(c *Category) {
	if s.media == nil || c.CoverImage == nil || c.CoverImage.S3Key == "" {
		return
	}
	c.CoverImage.URL = s.media.PublicURL(c.CoverImage.S3Key)
}

func categoryIDs(cats []Category) []uuid.UUID {
	ids := make([]uuid.UUID, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	return ids
}

func setCategoryContent(cats []Category, byID map[uuid.UUID]CategoryContent) {
	for i := range cats {
		if content, ok := byID[cats[i].ID]; ok {
			cp := content
			cats[i].Content = &cp
		}
	}
}

// Requested locale first, then the system default, then with anyLocale whatever translation a category has - one query per tier, never one per category
func (s *categoryService) resolveCategoryContent(ctx context.Context, ids []uuid.UUID, locale string, anyLocale bool) (map[uuid.UUID]CategoryContent, error) {
	out := make(map[uuid.UUID]CategoryContent, len(ids))
	missing := func() []uuid.UUID {
		var todo []uuid.UUID
		for _, id := range ids {
			if _, ok := out[id]; !ok {
				todo = append(todo, id)
			}
		}
		return todo
	}
	tiers := []string{locale}
	if def := s.resolveDefaultLocale(ctx); def != locale {
		tiers = append(tiers, def)
	}
	for _, code := range tiers {
		todo := missing()
		if code == "" || len(todo) == 0 {
			continue
		}
		byID, err := s.repo.ListCategoryContentByScope(ctx, todo, code)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, byID)
	}
	if todo := missing(); anyLocale && len(todo) > 0 {
		rows, err := s.repo.ListCategoryContentForCategories(ctx, todo)
		if err != nil {
			return nil, err
		}
		for id, r := range rows {
			if len(r) > 0 {
				out[id] = r[0]
			}
		}
	}
	return out, nil
}

// Content per the resolveCategoryContent tiers plus cover URLs, in place
func (s *categoryService) applyCategoryContent(ctx context.Context, cats []Category, locale string, anyLocale bool) error {
	if len(cats) == 0 {
		return nil
	}
	byID, err := s.resolveCategoryContent(ctx, categoryIDs(cats), locale, anyLocale)
	if err != nil {
		return err
	}
	setCategoryContent(cats, byID)
	for i := range cats {
		s.resolveCategoryCoverURL(&cats[i])
	}
	return nil
}

// ==================== Per-store category listings ====================

// SetCategoryChannelListing creates or updates a per-store listing for a category - SortOrderOverride is a pointer so callers can distinguish "explicit null" (meaning "clear the override; inherit category.Position") from "leave it unchanged" (we don't honour that here - the caller passes the desired final state) - Returns the loaded category with all its store listings populated
func (s *categoryService) SetCategoryChannelListing(ctx context.Context, in SetCategoryChannelListingInput) (Category, error) {
	if _, err := s.repo.UpsertCategoryChannelListing(ctx, in); err != nil {
		return Category{}, err
	}
	return s.reloadWithListings(ctx, in.CategoryID)
}

// ClearCategoryChannelListing removes the listing row for (category, store) - With no row, the category is treated as not-listed (hidden) on that store by storefront resolvers - Returns the reloaded category
func (s *categoryService) ClearCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) (Category, error) {
	if err := s.repo.DeleteCategoryChannelListing(ctx, categoryID, channelID); err != nil {
		return Category{}, err
	}
	return s.reloadWithListings(ctx, categoryID)
}

func (s *categoryService) reloadWithListings(ctx context.Context, categoryID uuid.UUID) (Category, error) {
	c, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	listings, err := s.repo.ListCategoryChannelListings(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	c.ChannelListings = listings
	s.resolveCategoryCoverURL(&c)
	s.categoryChanged(ctx, c, "listings")
	return c, nil
}

// ListCategoryChannelListings returns every store listing for a category - Used by the admin detail loader
func (s *categoryService) ListCategoryChannelListings(ctx context.Context, categoryID uuid.UUID) ([]CategoryChannelListing, error) {
	return s.repo.ListCategoryChannelListings(ctx, categoryID)
}

// Storefront tree: roots come back nested (Children populated recursively), limited to categories listed+published on the store - cached in Valkey per (store, locale), admin mutations invalidate the namespace so reads stay fresh within a request, and the TTL is only an upper bound for catastrophic cases (cache misconfig, namespace lock contention)
func (s *categoryService) ListPublishedRootCategoriesForChannel(ctx context.Context, channelID uuid.UUID, locale string) ([]Category, error) {
	cacheKey := categoryTreeCachePrefix + channelID.String() + ":" + locale
	if hit, ok := cache.GetJSON[[]Category](ctx, s.categoryTreeCache, cacheKey); ok {
		return hit, nil
	}
	all, err := s.repo.ListPublishedCategoriesForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if err := s.populateCategoryLocale(ctx, all, locale); err != nil {
		return nil, err
	}
	tree := buildCategoryTree(all, nil)
	s.categoryTreeCache.SetJSON(ctx, cacheKey, tree, categoryTreeCacheTTL)
	return tree, nil
}

// ListRootCategoriesListedOnChannel is the admin's store-scoped tree: every category listed on the store, published or not, nested like ListRootCategories - uncached, the storefront's published tree is the hot path
func (s *categoryService) ListRootCategoriesListedOnChannel(ctx context.Context, channelID uuid.UUID, locale string) ([]Category, error) {
	all, err := s.repo.ListCategoriesListedOnChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if err := s.populateCategoryLocale(ctx, all, locale); err != nil {
		return nil, err
	}
	return buildCategoryTree(all, nil), nil
}

// ListCategoryChildrenListedOnChannel returns the immediate children of a parent category listed on a store, published or not - admin only
func (s *categoryService) ListCategoryChildrenListedOnChannel(ctx context.Context, channelID, parentID uuid.UUID, locale string) ([]Category, error) {
	cats, err := s.repo.ListCategoryChildrenListedOnChannel(ctx, channelID, parentID)
	if err != nil {
		return nil, err
	}
	if err := s.populateCategoryLocale(ctx, cats, locale); err != nil {
		return nil, err
	}
	return cats, nil
}

// ListPublishedCategoryChildrenForChannel returns the immediate children of a parent category that are visible on a particular store
func (s *categoryService) ListPublishedCategoryChildrenForChannel(ctx context.Context, channelID, parentID uuid.UUID, locale string) ([]Category, error) {
	cats, err := s.repo.ListPublishedCategoryChildrenForChannel(ctx, channelID, parentID)
	if err != nil {
		return nil, err
	}
	if err := s.populateCategoryLocale(ctx, cats, locale); err != nil {
		return nil, err
	}
	return cats, nil
}

func (s *categoryService) ListProductsByCategory(ctx context.Context, categoryID uuid.UUID, locale string, limit int32) ([]Product, error) {
	products, err := s.repo.ListProductsByCategory(ctx, categoryID, limit)
	if err != nil {
		return nil, err
	}
	return s.withProductsContent(ctx, products, locale)
}

// GetPublishedCategoryForChannel returns a category visible on a particular store - Returns ErrCategoryNotFound when the category exists but is not listed on the store, or is unpublished globally
func (s *categoryService) GetPublishedCategoryForChannel(ctx context.Context, channelID, categoryID uuid.UUID, locale string) (Category, error) {
	c, err := s.repo.GetPublishedCategoryForChannel(ctx, channelID, categoryID)
	if err != nil {
		return Category{}, err
	}
	if locale != "" {
		one := []Category{c}
		if err := s.populateCategoryLocale(ctx, one, locale); err != nil {
			return Category{}, err
		}
		return one[0], nil
	}
	s.resolveCategoryCoverURL(&c)
	return c, nil
}

// FilterVisibleCategoryIDsForChannel returns the subset of candidate IDs whose own (category, channel) listing is published - No ancestor chain walk - use FilterFullyVisibleCategoryIDsForChannel when breadcrumb integrity matters - Empty input returns the empty set
func (s *categoryService) FilterVisibleCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return s.repo.FilterPublishedCategoryIDsForChannel(ctx, ids, channelID)
}

// FilterFullyVisibleCategoryIDsForChannel returns the subset of candidate IDs whose entire chain to root is listed-and-published on the store - Used by the strict ancestor rule on Query.category / categoryChildren
func (s *categoryService) FilterFullyVisibleCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return s.repo.FilterFullyVisibleCategoryIDsForChannel(ctx, ids, channelID)
}

// Requested locale first, then the system default, then any translation at all - the fallback matters for the admin tree, fetched without a locale (Apollo normalises categories by id, so locale-keyed queries would clobber each other), and guarantees `name` comes from some translation rather than the slug
func (s *categoryService) populateCategoryLocale(ctx context.Context, cats []Category, locale string) error {
	return s.applyCategoryContent(ctx, cats, locale, true)
}
