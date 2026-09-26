package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

// ==================== Collection CRUD ====================

func (s *collectionService) CreateCollection(ctx context.Context, input CreateCollectionInput) (Collection, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Collection{}, err
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Collection{}, fmt.Errorf("locale is required")
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return Collection{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if strings.TrimSpace(input.Name) == "" {
		return Collection{}, fmt.Errorf("name is required")
	}

	var created Collection
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		c, err := r.CreateCollection(ctx, input)
		if err != nil {
			return err
		}
		if _, err := r.UpsertCollectionContent(ctx, c.ID, CollectionContentInput{
			Locale:          input.Locale,
			Name:            input.Name,
			Description:     input.Description,
			MetaTitle:       input.MetaTitle,
			MetaDescription: input.MetaDescription,
			MetaKeywords:    input.MetaKeywords,
		}); err != nil {
			return err
		}
		created = c
		return nil
	}); err != nil {
		return Collection{}, err
	}

	if s.urlRewrites != nil {
		if err := s.urlRewrites.SyncCanonical(ctx, "collection", created.ID, input.Slug); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite",
				"op", "catalog.Service.CreateCollection",
				"collection_id", created.ID,
				"slug", input.Slug,
				"err", err)
		}
	}

	if s.bus != nil {
		s.bus.Publish(ctx, &CollectionCreated{CollectionID: created.ID, Slug: created.Slug})
	}
	return s.repo.GetCollectionByID(ctx, created.ID)
}

func (s *collectionService) GetCollection(ctx context.Context, id uuid.UUID, locale string) (Collection, error) {
	const op = "catalog.Service.GetCollection"
	c, err := s.repo.GetCollectionByID(ctx, id)
	if err != nil {
		if apperr.IsNotFound(err) {
			return Collection{}, err
		}
		return Collection{}, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	if locale != "" {
		content, err := s.repo.GetCollectionContent(ctx, id, locale)
		switch {
		case err == nil:
			cp := content
			c.Content = &cp
		case apperr.IsNotFound(err):
			// fall through to default-locale fallback
		default:
			return Collection{}, apperr.WrapClass(err, apperr.ClassInternal, op)
		}
	}
	if c.Content == nil {
		if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
			content, err := s.repo.GetCollectionContent(ctx, id, def)
			switch {
			case err == nil:
				cp := content
				c.Content = &cp
			case apperr.IsNotFound(err):
				// no translation at all - caller gets Content == nil
			default:
				return Collection{}, apperr.WrapClass(err, apperr.ClassInternal, op)
			}
		}
	}
	s.resolveCollectionCoverURL(&c)
	return c, nil
}

// ListCollectionTranslations returns every locale's content row for each collection, in one query - list paths only carry the resolved locale, so a caller wanting the full set (the admin Collection.content field) reads it here
func (s *collectionService) ListCollectionTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CollectionContent, error) {
	return s.repo.ListCollectionContentForCollections(ctx, ids)
}

func (s *collectionService) ListCollections(ctx context.Context, locale string) ([]Collection, error) {
	collections, err := s.repo.ListCollections(ctx)
	if err != nil {
		return nil, err
	}
	for i := range collections {
		s.resolveCollectionCoverURL(&collections[i])
	}
	if len(collections) == 0 {
		return collections, nil
	}
	ids := make([]uuid.UUID, len(collections))
	for i, c := range collections {
		ids[i] = c.ID
	}
	if locale != "" {
		batch, err := s.repo.ListCollectionContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, c := range collections {
			if content, ok := batch[c.ID]; ok {
				cp := content
				collections[i].Content = &cp
			}
		}
	}
	def := s.resolveDefaultLocale(ctx)
	if def == "" || def == locale {
		return collections, nil
	}
	missing := make([]uuid.UUID, 0, len(collections))
	for _, c := range collections {
		if c.Content == nil {
			missing = append(missing, c.ID)
		}
	}
	if len(missing) == 0 {
		return collections, nil
	}
	fallback, err := s.repo.ListCollectionContentByScope(ctx, missing, def)
	if err != nil {
		return nil, err
	}
	for i, c := range collections {
		if c.Content != nil {
			continue
		}
		if content, ok := fallback[c.ID]; ok {
			cp := content
			collections[i].Content = &cp
		}
	}
	return collections, nil
}

func (s *collectionService) UpdateCollection(ctx context.Context, input UpdateCollectionInput) (Collection, error) {
	if input.Slug != nil {
		if err := validateSlug(*input.Slug); err != nil {
			return Collection{}, err
		}
	}

	// Load current slug before updating to detect slug changes for redirects
	current, err := s.repo.GetCollectionByID(ctx, input.ID)
	if err != nil {
		return Collection{}, err
	}
	oldSlug := current.Slug

	if _, err := s.repo.UpdateCollection(ctx, input); err != nil {
		return Collection{}, err
	}

	if s.urlRewrites != nil && input.Slug != nil && *input.Slug != oldSlug {
		logger := applog.From(ctx)
		// Canonical FIRST: until it moves, this entity's own row still holds the old path, so the
		// redirect insert dies on the unique index and the rename leaves no 301 behind
		if err := s.urlRewrites.SyncCanonical(ctx, "collection", input.ID, *input.Slug); err != nil {
			logger.Error("sync canonical url rewrite",
				"op", "catalog.Service.UpdateCollection",
				"collection_id", input.ID,
				"slug", *input.Slug,
				"err", err)
		}
		s.createRenameRedirect(ctx, oldSlug, "collection", input.ID)
	}

	if s.bus != nil {
		var changed []string
		if input.Slug != nil {
			changed = append(changed, "slug")
		}
		updatedSlug := oldSlug
		if input.Slug != nil {
			updatedSlug = *input.Slug
		}
		s.bus.Publish(ctx, &CollectionUpdated{
			CollectionID:  input.ID,
			Slug:          updatedSlug,
			ChangedFields: changed,
		})
	}
	return s.repo.GetCollectionByID(ctx, input.ID)
}

func (s *collectionService) DeleteCollection(ctx context.Context, id uuid.UUID) error {
	snap, _ := s.repo.GetCollectionByID(ctx, id)
	if err := s.repo.DeleteCollection(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		if err := s.urlRewrites.DeleteAllForEntity(ctx, "collection", id); err != nil {
			applog.From(ctx).Error("delete url rewrites",
				"op", "catalog.Service.DeleteCollection",
				"collection_id", id,
				"err", err)
		}
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &CollectionDeleted{CollectionID: id, Slug: snap.Slug})
	}
	return nil
}

// ==================== Collection Content ====================

func (s *collectionService) UpsertCollectionContent(ctx context.Context, collectionID uuid.UUID, input CollectionContentInput) (Collection, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Collection{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Collection{}, fmt.Errorf("locale is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, input.Locale); err != nil {
			return Collection{}, err
		}
	}
	if _, err := s.repo.GetCollectionByID(ctx, collectionID); err != nil {
		return Collection{}, err
	}
	if _, err := s.repo.UpsertCollectionContent(ctx, collectionID, input); err != nil {
		return Collection{}, err
	}
	return s.repo.GetCollectionByID(ctx, collectionID)
}

func (s *collectionService) DeleteCollectionContent(ctx context.Context, collectionID uuid.UUID, locale string) error {
	if def := s.resolveDefaultLocale(ctx); def != "" && locale == def {
		all, err := s.repo.ListCollectionContent(ctx, collectionID)
		if err != nil {
			return err
		}
		for _, c := range all {
			if c.Locale != def {
				return ErrCannotDeleteDefaultLocale
			}
		}
	}
	return s.repo.DeleteCollectionContent(ctx, collectionID, locale)
}

// ==================== Collection Products ====================

func (s *collectionService) AddProductToCollection(ctx context.Context, collectionID, productID uuid.UUID, sortOrder int32) (Collection, error) {
	if _, err := s.repo.GetCollectionByID(ctx, collectionID); err != nil {
		return Collection{}, err
	}
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return Collection{}, err
	}
	if err := s.repo.AddProductToCollection(ctx, collectionID, productID, sortOrder); err != nil {
		return Collection{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &CollectionProductsChanged{
			CollectionID: collectionID,
			Added:        []uuid.UUID{productID},
		})
	}
	return s.repo.GetCollectionByID(ctx, collectionID)
}

// ListProductsByCollection returns the products linked to a collection in admin-defined order, batch-hydrated with locale-resolved content - Used by the storefront's /collections/[id] page - the front Collection resolver populates `c.Products` from this so the GraphQL `Collection.products` field returns something other than an empty list
func (s *collectionService) ListProductsByCollection(ctx context.Context, collectionID uuid.UUID, locale string) ([]Product, error) {
	products, err := s.repo.ListProductsByCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return products, nil
	}
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	contentByID, err := s.LoadProductsContent(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	for i := range products {
		if c, ok := contentByID[products[i].ID]; ok {
			cp := c
			products[i].Content = &cp
		}
	}
	return products, nil
}

func (s *collectionService) RemoveProductFromCollection(ctx context.Context, collectionID, productID uuid.UUID) (Collection, error) {
	if err := s.repo.RemoveProductFromCollection(ctx, collectionID, productID); err != nil {
		return Collection{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &CollectionProductsChanged{
			CollectionID: collectionID,
			Removed:      []uuid.UUID{productID},
		})
	}
	return s.repo.GetCollectionByID(ctx, collectionID)
}

func (s *collectionService) ReorderCollectionProducts(ctx context.Context, collectionID uuid.UUID, productIDs []uuid.UUID) (Collection, error) {
	if _, err := s.repo.GetCollectionByID(ctx, collectionID); err != nil {
		return Collection{}, err
	}
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for i, pid := range productIDs {
			if err := r.ReorderCollectionProduct(ctx, collectionID, pid, int32(i)); err != nil {
				return fmt.Errorf("reorder product %s: %w", pid, err)
			}
		}
		return nil
	}); err != nil {
		return Collection{}, err
	}
	return s.repo.GetCollectionByID(ctx, collectionID)
}

// ListCollectionsByProduct returns the collections a single product belongs to - Wraps the repo method so the search indexer can take a *Service without reaching into the repository
func (s *collectionService) ListCollectionsByProduct(ctx context.Context, productID uuid.UUID) ([]Collection, error) {
	return s.repo.ListCollectionsByProduct(ctx, productID)
}

// ListCollectionsByProductIDs batch-fetches collections for multiple products, optionally resolving content at the given locale - Used by the Product.collections field resolver to avoid N+1
func (s *collectionService) ListCollectionsByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]Collection, error) {
	byProduct, err := s.repo.ListCollectionsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}

	if len(byProduct) > 0 {
		seen := make(map[uuid.UUID]struct{})
		var collectionIDs []uuid.UUID
		for _, cols := range byProduct {
			for _, c := range cols {
				if _, ok := seen[c.ID]; !ok {
					collectionIDs = append(collectionIDs, c.ID)
					seen[c.ID] = struct{}{}
				}
			}
		}
		var primary map[uuid.UUID]CollectionContent
		if locale != "" {
			primary, err = s.repo.ListCollectionContentByScope(ctx, collectionIDs, locale)
			if err != nil {
				return nil, err
			}
		}
		var fallback map[uuid.UUID]CollectionContent
		if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
			fallback, err = s.repo.ListCollectionContentByScope(ctx, collectionIDs, def)
			if err != nil {
				return nil, err
			}
		}
		for pid, cols := range byProduct {
			for i, c := range cols {
				if content, ok := primary[c.ID]; ok {
					cp := content
					cols[i].Content = &cp
					continue
				}
				if content, ok := fallback[c.ID]; ok {
					cp := content
					cols[i].Content = &cp
				}
			}
			byProduct[pid] = cols
		}
	}

	for pid, cols := range byProduct {
		for i := range cols {
			s.resolveCollectionCoverURL(&cols[i])
		}
		byProduct[pid] = cols
	}
	return byProduct, nil
}

// ==================== Collection Cover ====================

func (s *collectionService) RequestCollectionCoverUpload(ctx context.Context, input RequestCollectionCoverInput) (RequestCollectionCoverResult, error) {
	if s.media == nil {
		return RequestCollectionCoverResult{}, fmt.Errorf("media storage not configured")
	}
	if !media.IsAllowedImage(input.MimeType) {
		return RequestCollectionCoverResult{}, fmt.Errorf("%w: %s", ErrInvalidMimeType, input.MimeType)
	}
	if input.SizeBytes <= 0 {
		return RequestCollectionCoverResult{}, fmt.Errorf("size_bytes must be positive")
	}
	if input.SizeBytes > s.media.MaxUploadBytes() {
		return RequestCollectionCoverResult{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, input.SizeBytes, s.media.MaxUploadBytes())
	}
	if _, err := s.repo.GetCollectionByID(ctx, input.CollectionID); err != nil {
		return RequestCollectionCoverResult{}, err
	}

	coverID, err := uuid.NewV7()
	if err != nil {
		return RequestCollectionCoverResult{}, fmt.Errorf("generate cover id: %w", err)
	}
	ext := media.ExtensionForMime(input.MimeType)
	s3Key := fmt.Sprintf("catalog/collections/%s/cover/%s%s", input.CollectionID, coverID, ext)

	uploadURL, err := s.media.PresignUpload(ctx, s3Key, input.MimeType, input.SizeBytes, presignTTL)
	if err != nil {
		return RequestCollectionCoverResult{}, fmt.Errorf("presign: %w", err)
	}

	c, err := s.repo.SetCollectionCover(ctx, input.CollectionID, coverID, s3Key, input.MimeType, input.SizeBytes, input.AltText)
	if err != nil {
		return RequestCollectionCoverResult{}, err
	}
	if c.CoverImage != nil {
		c.CoverImage.URL = s.media.PublicURL(s3Key)
	}

	return RequestCollectionCoverResult{
		Collection: c,
		UploadURL:  uploadURL,
		ExpiresAt:  time.Now().Add(presignTTL),
	}, nil
}

func (s *collectionService) ConfirmCollectionCover(ctx context.Context, collectionID uuid.UUID, width, height int32) (Collection, error) {
	if width <= 0 || height <= 0 {
		return Collection{}, fmt.Errorf("width and height must be positive")
	}
	current, err := s.repo.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	if current.CoverImage == nil {
		return Collection{}, ErrCollectionNoCover
	}
	if current.CoverImage.Status != string(MediaStatusPending) {
		return Collection{}, ErrCollectionCoverNotPending
	}
	if s.media != nil {
		_, _, exists, err := s.media.Head(ctx, current.CoverImage.S3Key)
		if err == nil && !exists {
			return Collection{}, ErrUploadNotReceived
		}
	}
	c, err := s.repo.ConfirmCollectionCover(ctx, collectionID, width, height)
	if err != nil {
		return Collection{}, err
	}
	if s.media != nil && c.CoverImage != nil {
		c.CoverImage.URL = s.media.PublicURL(c.CoverImage.S3Key)
	}
	return c, nil
}

func (s *collectionService) ClearCollectionCover(ctx context.Context, collectionID uuid.UUID) (Collection, error) {
	c, err := s.repo.ClearCollectionCover(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	return c, nil
}

// resolveCollectionCoverURL fills in the URL field on a collection's cover image using objstore.Storage.PublicURL - No-ops when cover is nil or storage is not configured
func (s *collectionService) resolveCollectionCoverURL(c *Collection) {
	if s.media == nil || c.CoverImage == nil || c.CoverImage.S3Key == "" {
		return
	}
	c.CoverImage.URL = s.media.PublicURL(c.CoverImage.S3Key)
}

func (s *collectionService) ListCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID) ([]FacetOverride, error) {
	return s.repo.ListCollectionFacetOverrides(ctx, collectionID)
}

// Same contract as the category twin: full replacement, positions re-numbered from
// slice order, empty resets to attribute-order defaults
func (s *collectionService) SetCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID, overrides []FacetOverride) error {
	if _, err := s.repo.GetCollectionByID(ctx, collectionID); err != nil {
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
	return s.repo.ReplaceCollectionFacetOverrides(ctx, collectionID, overrides)
}
