package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/applog"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/media"
)

// ==================== Category CRUD ====================

func (s *categoryService) CreateCategory(ctx context.Context, input CreateCategoryInput) (Category, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Category{}, err
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Category{}, fmt.Errorf("locale is required")
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return Category{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if strings.TrimSpace(input.Name) == "" {
		return Category{}, fmt.Errorf("name is required")
	}

	// Build the URL path from parent's path + own slug
	urlPath := input.Slug
	if input.ParentID != nil {
		parent, err := s.repo.GetCategoryByID(ctx, *input.ParentID)
		if err != nil {
			return Category{}, fmt.Errorf("get parent category: %w", err)
		}
		urlPath = parent.URLPath + "/" + input.Slug
	}

	var created Category
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		// Override url_path with the computed value before insert
		in := input
		in.Slug = input.Slug
		c, err := r.CreateCategory(ctx, in)
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
		// Best-effort: a stale canonical doesn't roll back the create (the category exists - the redirect can be re-synced), but we MUST log so a quietly-broken redirect surfaces in ops
		if err := s.urlRewrites.SyncCanonical(ctx, "category", created.ID, urlPath); err != nil {
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

// CountProductsInCategory returns how many products are directly assigned to a category (its own membership - descendant categories are not rolled up) - Backs the admin Category.productCount field
func (s *categoryService) CountProductsInCategory(ctx context.Context, id uuid.UUID) (int64, error) {
	return s.repo.CountProductsInCategory(ctx, id)
}

func (s *categoryService) ListRootCategories(ctx context.Context, locale string) ([]Category, error) {
	all, err := s.repo.ListAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		s.resolveCategoryCoverURL(&all[i])
	}
	if len(all) > 0 {
		ids := make([]uuid.UUID, len(all))
		for i, c := range all {
			ids[i] = c.ID
		}
		if locale != "" {
			contentMap, err := s.repo.ListCategoryContentByScope(ctx, ids, locale)
			if err != nil {
				return nil, err
			}
			for i := range all {
				if content, ok := contentMap[all[i].ID]; ok {
					cp := content
					all[i].Content = &cp
				}
			}
		}
		if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
			missing := make([]uuid.UUID, 0, len(all))
			for _, c := range all {
				if c.Content == nil {
					missing = append(missing, c.ID)
				}
			}
			if len(missing) > 0 {
				fallback, err := s.repo.ListCategoryContentByScope(ctx, missing, def)
				if err != nil {
					return nil, err
				}
				for i := range all {
					if all[i].Content != nil {
						continue
					}
					if content, ok := fallback[all[i].ID]; ok {
						cp := content
						all[i].Content = &cp
					}
				}
			}
		}
	}
	return buildCategoryTree(all, nil), nil
}

// buildCategoryTree recursively organises a flat list of categories into a parent→children tree. parentID nil = collect root nodes
func buildCategoryTree(all []Category, parentID *uuid.UUID) []Category {
	var result []Category
	for _, c := range all {
		match := (parentID == nil && c.ParentID == nil) ||
			(parentID != nil && c.ParentID != nil && *c.ParentID == *parentID)
		if !match {
			continue
		}
		id := c.ID
		c.Children = buildCategoryTree(all, &id)
		result = append(result, c)
	}
	return result
}

func (s *categoryService) ListCategoryChildren(ctx context.Context, parentID uuid.UUID, locale string) ([]Category, error) {
	children, err := s.repo.ListCategoryChildren(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for i := range children {
		s.resolveCategoryCoverURL(&children[i])
	}
	if len(children) == 0 {
		return children, nil
	}
	ids := make([]uuid.UUID, len(children))
	for i, c := range children {
		ids[i] = c.ID
	}
	if locale != "" {
		batch, err := s.repo.ListCategoryContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, c := range children {
			if content, ok := batch[c.ID]; ok {
				cp := content
				children[i].Content = &cp
			}
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		missing := make([]uuid.UUID, 0, len(children))
		for _, c := range children {
			if c.Content == nil {
				missing = append(missing, c.ID)
			}
		}
		if len(missing) > 0 {
			fallback, err := s.repo.ListCategoryContentByScope(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			for i, c := range children {
				if c.Content != nil {
					continue
				}
				if content, ok := fallback[c.ID]; ok {
					cp := content
					children[i].Content = &cp
				}
			}
		}
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
	ids := make([]uuid.UUID, len(ancestors))
	for i, a := range ancestors {
		ids[i] = a.ID
	}
	batch, err := s.repo.ListCategoryContentByScope(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	for i, a := range ancestors {
		if content, ok := batch[a.ID]; ok {
			cp := content
			ancestors[i].Content = &cp
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		missing := make([]uuid.UUID, 0, len(ancestors))
		for _, a := range ancestors {
			if a.Content == nil {
				missing = append(missing, a.ID)
			}
		}
		if len(missing) > 0 {
			fallback, err := s.repo.ListCategoryContentByScope(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			for i, a := range ancestors {
				if a.Content != nil {
					continue
				}
				if content, ok := fallback[a.ID]; ok {
					cp := content
					ancestors[i].Content = &cp
				}
			}
		}
	}
	return ancestors, nil
}

func (s *categoryService) ListCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID) ([]FacetOverride, error) {
	return s.repo.ListCategoryFacetOverrides(ctx, categoryID)
}

// SetCategoryFacetOverrides replaces the category's whole override set - positions are
// re-numbered from the slice order so callers can't hand in colliding sort keys, and an
// empty slice resets the category to attribute-order defaults
func (s *categoryService) SetCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID, overrides []FacetOverride) error {
	if _, err := s.repo.GetCategoryByID(ctx, categoryID); err != nil {
		return err
	}
	seen := make(map[uuid.UUID]struct{}, len(overrides))
	for i := range overrides {
		if _, dup := seen[overrides[i].AttributeID]; dup {
			return ErrDuplicateFacetOverride
		}
		seen[overrides[i].AttributeID] = struct{}{}
		overrides[i].SortOrder = int32(i)
	}
	return s.repo.ReplaceCategoryFacetOverrides(ctx, categoryID, overrides)
}

func (s *categoryService) UpdateCategory(ctx context.Context, input UpdateCategoryInput) (Category, error) {
	if input.Slug != nil {
		if err := validateSlug(*input.Slug); err != nil {
			return Category{}, err
		}
	}

	current, err := s.repo.GetCategoryByID(ctx, input.ID)
	if err != nil {
		return Category{}, err
	}
	oldURLPath := current.URLPath

	updated, err := s.repo.UpdateCategory(ctx, input)
	if err != nil {
		return Category{}, err
	}

	// Recompute URL path when either slug OR parent changed - Slug rename moves the leaf token - re-parenting moves the entire prefix - Both produce a new url_path for this category and every descendant, plus a 301 from the prior path - Pre-fix this branch only fired on slug change, leaving re-parented categories on a stale prefix
	slugChanged := input.Slug != nil && *input.Slug != current.Slug
	parentChanged := !uuidPtrEqual(current.ParentID, updated.ParentID)
	if slugChanged || parentChanged {
		ancestors, err := s.GetCategoryAncestors(ctx, updated.ID, "")
		if err != nil {
			return Category{}, err
		}
		newURLPath := buildCategoryURLPath(ancestors, updated.Slug)
		if err := s.repo.UpdateCategoryURLPath(ctx, updated.ID, newURLPath); err != nil {
			return Category{}, err
		}
		updated.URLPath = newURLPath
		if s.urlRewrites != nil {
			// Best-effort: a stale canonical or missing redirect on slug change is a downstream problem (SEO), not a reason to roll back the category update - Log so it's visible in ops instead of disappearing silently
			logger := applog.From(ctx)
			if err := s.urlRewrites.SyncCanonical(ctx, "category", updated.ID, newURLPath); err != nil {
				logger.Error("sync canonical url rewrite",
					"op", "catalog.Service.UpdateCategory",
					"category_id", updated.ID,
					"url_path", newURLPath,
					"err", err)
			}
			s.createRenameRedirect(ctx, oldURLPath, "category", updated.ID)
		}
		// Propagate new path prefix to all descendants
		if err := s.propagateURLPaths(ctx, updated.ID, newURLPath); err != nil {
			return Category{}, err
		}
		// Cascade to products whose primary category is this category or one of its descendants - Templates using {category}/{categorySlug} otherwise keep the stale prefix until some unrelated edit triggers a regen
		if err := s.regenerateProductsInCategorySubtree(ctx, updated.ID); err != nil {
			return Category{}, err
		}
	}

	if s.bus != nil {
		var changed []string
		if input.Slug != nil {
			changed = append(changed, "slug")
		}
		if input.ParentID != nil {
			changed = append(changed, "parent_id")
		}
		s.bus.Publish(ctx, &CategoryUpdated{
			CategoryID:    updated.ID,
			Slug:          updated.Slug,
			ParentID:      updated.ParentID,
			ChangedFields: changed,
		})
	}
	s.invalidateCategoryTreeCache(ctx)
	return s.repo.GetCategoryByID(ctx, updated.ID)
}

// propagateURLPaths recursively updates url_path for all descendants after a parent slug change, also syncing canonical URL rewrites
func (s *categoryService) propagateURLPaths(ctx context.Context, parentID uuid.UUID, parentURLPath string) error {
	children, err := s.repo.ListCategoryChildren(ctx, parentID)
	if err != nil {
		return err
	}
	for _, child := range children {
		newPath := parentURLPath + "/" + child.Slug
		oldPath := child.URLPath
		if err := s.repo.UpdateCategoryURLPath(ctx, child.ID, newPath); err != nil {
			return err
		}
		if s.urlRewrites != nil {
			logger := applog.From(ctx)
			if err := s.urlRewrites.SyncCanonical(ctx, "category", child.ID, newPath); err != nil {
				logger.Error("sync canonical url rewrite",
					"op", "catalog.Service.propagateURLPaths",
					"category_id", child.ID,
					"url_path", newPath,
					"err", err)
			}
			s.createRenameRedirect(ctx, oldPath, "category", child.ID)
		}
		if err := s.propagateURLPaths(ctx, child.ID, newPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *categoryService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	children, err := s.repo.ListCategoryChildren(ctx, id)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return ErrCategoryHasChildren
	}
	products, err := s.repo.ListProductsByCategory(ctx, id)
	if err != nil {
		return err
	}
	if len(products) > 0 {
		return ErrCategoryHasProducts
	}
	// Snapshot slug before delete - listeners need the human handle since the row vanishes
	snap, _ := s.repo.GetCategoryByID(ctx, id)
	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		if err := s.urlRewrites.DeleteAllForEntity(ctx, "category", id); err != nil {
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

// ==================== Category Content ====================

func (s *categoryService) UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, input CategoryContentInput) (Category, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Category{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Category{}, fmt.Errorf("locale is required")
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
	s.invalidateCategoryTreeCache(ctx)
	return s.repo.GetCategoryByID(ctx, categoryID)
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
	if err := s.repo.DeleteCategoryContent(ctx, categoryID, locale); err != nil {
		return err
	}
	s.invalidateCategoryTreeCache(ctx)
	return nil
}

// ==================== Product-Category ====================

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
	wasPrimary := false
	primaryByID, err := s.repo.ListPrimaryCategoryByProductIDs(ctx, []uuid.UUID{productID})
	if err == nil {
		if cur := primaryByID[productID]; cur != nil && cur.ID == categoryID {
			wasPrimary = true
		}
	}
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
	if len(byProduct) > 0 {
		seen := make(map[uuid.UUID]struct{})
		var categoryIDs []uuid.UUID
		for _, cats := range byProduct {
			for _, c := range cats {
				if _, ok := seen[c.ID]; !ok {
					categoryIDs = append(categoryIDs, c.ID)
					seen[c.ID] = struct{}{}
				}
			}
		}
		var primary map[uuid.UUID]CategoryContent
		if locale != "" {
			primary, err = s.repo.ListCategoryContentByScope(ctx, categoryIDs, locale)
			if err != nil {
				return nil, err
			}
		}
		var fallback map[uuid.UUID]CategoryContent
		if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
			fallback, err = s.repo.ListCategoryContentByScope(ctx, categoryIDs, def)
			if err != nil {
				return nil, err
			}
		}
		for pid, cats := range byProduct {
			for i, c := range cats {
				if content, ok := primary[c.ID]; ok {
					cp := content
					cats[i].Content = &cp
					continue
				}
				if content, ok := fallback[c.ID]; ok {
					cp := content
					cats[i].Content = &cp
				}
			}
			byProduct[pid] = cats
		}
	}
	for pid, cats := range byProduct {
		for i := range cats {
			s.resolveCategoryCoverURL(&cats[i])
		}
		byProduct[pid] = cats
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
		return RequestCategoryCoverResult{}, fmt.Errorf("size_bytes must be positive")
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
	ext := media.ExtensionForMime(input.MimeType)
	s3Key := fmt.Sprintf("catalog/categories/%s/cover/%s%s", input.CategoryID, coverID, ext)

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

	return RequestCategoryCoverResult{
		Category:  c,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().Add(presignTTL),
	}, nil
}

func (s *categoryService) ConfirmCategoryCover(ctx context.Context, categoryID uuid.UUID, width, height int32) (Category, error) {
	if width <= 0 || height <= 0 {
		return Category{}, fmt.Errorf("width and height must be positive")
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
		if err == nil && !exists {
			return Category{}, ErrUploadNotReceived
		}
	}
	c, err := s.repo.ConfirmCategoryCover(ctx, categoryID, width, height)
	if err != nil {
		return Category{}, err
	}
	if s.media != nil && c.CoverImage != nil {
		c.CoverImage.URL = s.media.PublicURL(c.CoverImage.S3Key)
	}
	return c, nil
}

func (s *categoryService) ClearCategoryCover(ctx context.Context, categoryID uuid.UUID) (Category, error) {
	return s.repo.ClearCategoryCover(ctx, categoryID)
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

// Called from UpdateCategory after a slug or parent change so {category}/{categorySlug}-templated product URLs don't drift - strict: a failure (missing data, unrenderable template) surfaces back and rolls back, mirroring SetProductPrimaryCategory
func (s *categoryService) regenerateProductsInCategorySubtree(ctx context.Context, rootID uuid.UUID) error {
	ids, err := s.repo.ListProductIDsInPrimaryCategoryDescendants(ctx, rootID)
	if err != nil {
		return fmt.Errorf("list products under category subtree %s: %w", rootID, err)
	}
	for _, pid := range ids {
		if err := s.regenerateProductURLPath(ctx, pid, true); err != nil {
			return fmt.Errorf("regenerate product %s: %w", pid, err)
		}
	}
	return nil
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

// ==================== Per-store category listings ====================

// SetCategoryChannelListing creates or updates a per-store listing for a category - SortOrderOverride is a pointer so callers can distinguish "explicit null" (meaning "clear the override; inherit category.Position") from "leave it unchanged" (we don't honour that here - the caller passes the desired final state) - Returns the loaded category with all its store listings populated
func (s *categoryService) SetCategoryChannelListing(ctx context.Context, in SetCategoryChannelListingInput) (Category, error) {
	if _, err := s.repo.UpsertCategoryChannelListing(ctx, in); err != nil {
		return Category{}, err
	}
	c, err := s.repo.GetCategoryByID(ctx, in.CategoryID)
	if err != nil {
		return Category{}, err
	}
	listings, err := s.repo.ListCategoryChannelListings(ctx, in.CategoryID)
	if err != nil {
		return Category{}, err
	}
	c.ChannelListings = listings
	s.resolveCategoryCoverURL(&c)
	s.invalidateCategoryTreeCache(ctx)
	return c, nil
}

// ClearCategoryChannelListing removes the listing row for (category, store) - With no row, the category is treated as not-listed (hidden) on that store by storefront resolvers - Returns the reloaded category
func (s *categoryService) ClearCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) (Category, error) {
	if err := s.repo.DeleteCategoryChannelListing(ctx, categoryID, channelID); err != nil {
		return Category{}, err
	}
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
	s.invalidateCategoryTreeCache(ctx)
	return c, nil
}

// ListCategoryChannelListings returns every store listing for a category - Used by the admin detail loader
func (s *categoryService) ListCategoryChannelListings(ctx context.Context, categoryID uuid.UUID) ([]CategoryChannelListing, error) {
	return s.repo.ListCategoryChannelListings(ctx, categoryID)
}

// GetCategoryChannelListing returns one listing or ErrCategoryChannelListingNotFound
func (s *categoryService) GetCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) (CategoryChannelListing, error) {
	return s.repo.GetCategoryChannelListing(ctx, categoryID, channelID)
}

// Roots come back nested (Children populated recursively) so the admin tree in CHANNEL renders the same hierarchy as GLOBAL, limited to categories listed+published on the store - cached in Valkey per (store, locale), admin mutations invalidate the namespace so reads stay fresh within a request, and the 20m TTL is only an upper bound for catastrophic cases (cache misconfig, namespace lock contention)
func (s *categoryService) ListPublishedRootCategoriesForChannel(ctx context.Context, channelID uuid.UUID, locale string) ([]Category, error) {
	cacheKey := "tree:" + channelID.String() + ":" + locale
	if hit, ok := cache.GetJSON[[]Category](ctx, s.categoryTreeCache, cacheKey); ok {
		return hit, nil
	}
	all, err := s.repo.ListPublishedCategoriesForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		s.resolveCategoryCoverURL(&all[i])
	}
	if err := s.populateCategoryLocale(ctx, all, locale); err != nil {
		return nil, err
	}
	tree := buildCategoryTree(all, nil)
	s.categoryTreeCache.SetJSON(ctx, cacheKey, tree, 20*time.Minute)
	return tree, nil
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

// Requested locale first, then the system default for anything still empty - the fallback matters for the admin tree, fetched without a locale (Apollo normalises categories by id, so locale-keyed queries would clobber each other), and guarantees `name` comes from some translation rather than the slug
func (s *categoryService) populateCategoryLocale(ctx context.Context, cats []Category, locale string) error {
	if len(cats) == 0 {
		return nil
	}
	def := s.resolveDefaultLocale(ctx)
	primary := locale
	if primary == "" {
		primary = def
	}
	ids := make([]uuid.UUID, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	if primary != "" {
		byID, err := s.repo.ListCategoryContentByScope(ctx, ids, primary)
		if err != nil {
			return err
		}
		for i := range cats {
			if content, ok := byID[cats[i].ID]; ok {
				cp := content
				cats[i].Content = &cp
			}
		}
	}
	if def != "" && def != primary {
		missing := make([]uuid.UUID, 0, len(cats))
		for _, c := range cats {
			if c.Content == nil {
				missing = append(missing, c.ID)
			}
		}
		if len(missing) > 0 {
			fallback, err := s.repo.ListCategoryContentByScope(ctx, missing, def)
			if err != nil {
				return err
			}
			for i := range cats {
				if cats[i].Content != nil {
					continue
				}
				if content, ok := fallback[cats[i].ID]; ok {
					cp := content
					cats[i].Content = &cp
				}
			}
		}
	}
	// Last-resort fallback: load whatever translation exists for any category that's still without content, so the admin tree can show a real name instead of just the slug for categories whose only translation is in a locale other than the active/default
	for i := range cats {
		if cats[i].Content != nil {
			continue
		}
		rows, err := s.repo.ListCategoryContent(ctx, cats[i].ID)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			cp := rows[0]
			cats[i].Content = &cp
		}
	}
	for i := range cats {
		s.resolveCategoryCoverURL(&cats[i])
	}
	return nil
}
