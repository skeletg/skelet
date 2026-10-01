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
)

const collectionCoverKeyFormat = "catalog/collections/%s/cover/%s%s"

var collectionEntityType = string(urlrewrite.EntityTypeCollection)

func collectionCoverKey(collectionID, coverID uuid.UUID, ext string) string {
	return fmt.Sprintf(collectionCoverKeyFormat, collectionID, coverID, ext)
}

// ==================== Collection CRUD ====================

func (s *collectionService) CreateCollection(ctx context.Context, input CreateCollectionInput) (Collection, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Collection{}, err
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Collection{}, ErrLocaleRequired
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return Collection{}, fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if strings.TrimSpace(input.Name) == "" {
		return Collection{}, ErrNameRequired
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
		if err := s.urlRewrites.SyncCanonical(ctx, collectionEntityType, created.ID, input.Slug); err != nil {
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
	ids := make([]uuid.UUID, len(collections))
	for i := range collections {
		s.resolveCollectionCoverURL(&collections[i])
		ids[i] = collections[i].ID
	}
	byID, err := s.resolveCollectionContent(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	setCollectionContent(collections, byID)
	return collections, nil
}

func setCollectionContent(cols []Collection, byID map[uuid.UUID]CollectionContent) {
	for i := range cols {
		if content, ok := byID[cols[i].ID]; ok {
			cp := content
			cols[i].Content = &cp
		}
	}
}

// Requested locale first, then the system default for whatever is still missing - one query per tier
func (s *collectionService) resolveCollectionContent(ctx context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID]CollectionContent, error) {
	out := make(map[uuid.UUID]CollectionContent, len(ids))
	tiers := []string{locale}
	if def := s.resolveDefaultLocale(ctx); def != locale {
		tiers = append(tiers, def)
	}
	for _, code := range tiers {
		var todo []uuid.UUID
		for _, id := range ids {
			if _, ok := out[id]; !ok {
				todo = append(todo, id)
			}
		}
		if code == "" || len(todo) == 0 {
			continue
		}
		byID, err := s.repo.ListCollectionContentByScope(ctx, todo, code)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, byID)
	}
	return out, nil
}

// Every storefront-visible change that isn't a field update - the collection's pages purge
func (s *collectionService) collectionChanged(ctx context.Context, c Collection, changed ...string) {
	if s.bus != nil {
		s.bus.Publish(ctx, &CollectionUpdated{CollectionID: c.ID, Slug: c.Slug, ChangedFields: changed})
	}
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
		if err := s.urlRewrites.SyncCanonical(ctx, collectionEntityType, input.ID, *input.Slug); err != nil {
			logger.Error("sync canonical url rewrite",
				"op", "catalog.Service.UpdateCollection",
				"collection_id", input.ID,
				"slug", *input.Slug,
				"err", err)
		}
		s.createRenameRedirect(ctx, oldSlug, collectionEntityType, input.ID)
	}

	if s.bus != nil {
		changed := changedCollectionFields(input)
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

func changedCollectionFields(in UpdateCollectionInput) []string {
	var changed []string
	add := func(set bool, name string) {
		if set {
			changed = append(changed, name)
		}
	}
	add(in.Slug != nil, "slug")
	add(in.IsPublished != nil, "is_published")
	add(in.SEONoindex != nil, "seo_noindex")
	add(in.Metadata != nil, "metadata")
	add(in.PrivateMetadata != nil, "private_metadata")
	return changed
}

func (s *collectionService) DeleteCollection(ctx context.Context, id uuid.UUID) error {
	snap, err := s.repo.GetCollectionByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCollection(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		if err := s.urlRewrites.DeleteAllForEntity(ctx, collectionEntityType, id); err != nil {
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
		return Collection{}, ErrNameRequired
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Collection{}, ErrLocaleRequired
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
	c, err := s.repo.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	s.collectionChanged(ctx, c, "content")
	return c, nil
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
	c, err := s.repo.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCollectionContent(ctx, collectionID, locale); err != nil {
		return err
	}
	s.collectionChanged(ctx, c, "content")
	return nil
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

func (s *collectionService) ListProductsByCollection(ctx context.Context, collectionID uuid.UUID, locale string, limit int32) ([]Product, error) {
	products, err := s.repo.ListProductsByCollection(ctx, collectionID, limit)
	if err != nil {
		return nil, err
	}
	return s.withProductsContent(ctx, products, locale)
}

func (s *core) withProductsContent(ctx context.Context, products []Product, locale string) ([]Product, error) {
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

// One tx, so an id that isn't a member rolls the whole reorder back instead of leaving it half applied
func (s *collectionService) ReorderCollectionProducts(ctx context.Context, collectionID uuid.UUID, productIDs []uuid.UUID) (Collection, error) {
	if len(productIDs) > int(MaxUnpagedRows) {
		return Collection{}, fmt.Errorf("%w: %d products, at most %d", ErrTooManyItems, len(productIDs), MaxUnpagedRows)
	}
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
	c, err := s.repo.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	s.collectionChanged(ctx, c, "product_order")
	return c, nil
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

	seen := make(map[uuid.UUID]struct{})
	var ids []uuid.UUID
	for _, cols := range byProduct {
		for _, c := range cols {
			if _, ok := seen[c.ID]; !ok {
				ids = append(ids, c.ID)
				seen[c.ID] = struct{}{}
			}
		}
	}
	byID, err := s.resolveCollectionContent(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	for _, cols := range byProduct {
		setCollectionContent(cols, byID)
		for i := range cols {
			s.resolveCollectionCoverURL(&cols[i])
		}
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
		return RequestCollectionCoverResult{}, ErrInvalidSize
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
	s3Key := collectionCoverKey(input.CollectionID, coverID, media.ExtensionForMime(input.MimeType))

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
	s.collectionChanged(ctx, c, "cover")

	return RequestCollectionCoverResult{
		Collection: c,
		UploadURL:  uploadURL,
		ExpiresAt:  time.Now().Add(presignTTL),
	}, nil
}

// Pinned to the cover checked here, like the category twin
func (s *collectionService) ConfirmCollectionCover(ctx context.Context, collectionID uuid.UUID, width, height int32) (Collection, error) {
	if width <= 0 || height <= 0 {
		return Collection{}, ErrInvalidDimensions
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
		if err != nil {
			return Collection{}, fmt.Errorf("%w: %w", ErrUploadNotReceived, err)
		}
		if !exists {
			return Collection{}, ErrUploadNotReceived
		}
	}
	c, err := s.repo.ConfirmCollectionCover(ctx, collectionID, current.CoverImage.ID, width, height)
	if err != nil {
		return Collection{}, err
	}
	s.resolveCollectionCoverURL(&c)
	s.collectionChanged(ctx, c, "cover")
	return c, nil
}

func (s *collectionService) ClearCollectionCover(ctx context.Context, collectionID uuid.UUID) (Collection, error) {
	c, err := s.repo.ClearCollectionCover(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	s.collectionChanged(ctx, c, "cover")
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
	if err := checkFacetOverrides(overrides); err != nil {
		return err
	}
	c, err := s.repo.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return err
	}
	if err := s.repo.ReplaceCollectionFacetOverrides(ctx, collectionID, overrides); err != nil {
		return err
	}
	s.collectionChanged(ctx, c, "facet_overrides")
	return nil
}
