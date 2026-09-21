package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

// Carved off the catalog Service god-struct - embeds *core for shared deps, and Service embeds this back so callers keep calling catalogSvc.CreateProduct
type productService struct{ *core }

// ==================== Product CRUD ====================

// Normalizes in place and returns the loaded product type plus the SKU base for the caller's collision-retry loop - all create-time rejections live here
func (s *productService) validateAndNormalizeCreateInput(ctx context.Context, input *CreateProductInput) (ProductType, string, error) {
	if strings.TrimSpace(input.Locale) == "" {
		return ProductType{}, "", fmt.Errorf("locale is required")
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && input.Locale != def {
		return ProductType{}, "", fmt.Errorf("%w: got %q, want %q", ErrLocaleMustBeDefault, input.Locale, def)
	}
	if strings.TrimSpace(input.Name) == "" {
		return ProductType{}, "", fmt.Errorf("name is required")
	}
	if input.Kind == "" {
		input.Kind = KindSimple
	}
	if !input.Kind.IsKnown() {
		return ProductType{}, "", fmt.Errorf("%w: %q", ErrInvalidProductKind, string(input.Kind))
	}
	if input.DefaultPrice < 0 {
		return ProductType{}, "", fmt.Errorf("default_price must be non-negative")
	}
	// Required - products are global with no store to inherit from, so an implicit code stamped the wrong one on non-USD catalogs
	input.DefaultCurrencyCode = strings.ToUpper(strings.TrimSpace(input.DefaultCurrencyCode))
	if len(input.DefaultCurrencyCode) != 3 {
		return ProductType{}, "", ErrInvalidCurrency
	}
	pt, err := s.repo.GetProductTypeByID(ctx, input.ProductTypeID)
	if err != nil {
		return ProductType{}, "", err
	}
	// Default to true when the product type has a template
	if input.URLAuto == nil {
		v := pt.URLTemplate != ""
		input.URLAuto = &v
	}
	if *input.URLAuto && input.URL != nil && strings.TrimSpace(*input.URL) != "" {
		return ProductType{}, "", ErrManualPathWhileAuto
	}
	// url is NOT NULL - seed from the name, regenerateProductURL refines it after create
	if input.URL == nil || strings.TrimSpace(*input.URL) == "" {
		seed := slugifyForSKU(input.Name)
		if seed == "" {
			seed = "product"
		}
		input.URL = &seed
	}
	// Empty falls back to a slugified name so imports and fixtures still satisfy the NOT NULL constraint
	base := strings.TrimSpace(input.SKU)
	if base == "" {
		base = slugifyForSKU(input.Name)
		if base == "" {
			base = "product"
		}
	}
	input.SKU = base
	return pt, base, nil
}

func (s *productService) CreateProduct(ctx context.Context, input CreateProductInput) (Product, error) {
	pt, base, err := s.validateAndNormalizeCreateInput(ctx, &input)
	if err != nil {
		return Product{}, err
	}
	// Sync pre-tx hook - external systems can reject the create or contribute tags folded into metadata
	if s.bus != nil {
		ing := &ProductCreating{
			ProductTypeID: input.ProductTypeID,
			SKU:           input.SKU,
		}
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Product{}, err
		}
		if len(ing.Tags) > 0 {
			if input.Metadata == nil {
				input.Metadata = Metadata{}
			}
			input.Metadata["webhook_tags"] = strings.Join(ing.Tags, ",")
		}
	}
	// Each attempt is its own transaction so a failed content write can't orphan a product, and a SKU collision retries cleanly
	var p Product
	for attempt := range 100 {
		if attempt > 0 {
			input.SKU = fmt.Sprintf("%s-%d", base, attempt)
		}
		err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
			r := s.repo.WithTx(t)
			created, err := r.CreateProduct(ctx, input)
			if err != nil {
				return err
			}
			if _, err := r.UpsertProductContent(ctx, created.ID, ProductContentInput{
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
			p = created
			return nil
		})
		if errors.Is(err, ErrSKUTaken) {
			continue
		}
		if err != nil {
			return Product{}, err
		}
		break
	}
	if p.ID == (uuid.UUID{}) {
		return Product{}, fmt.Errorf("could not generate unique SKU from %q after 100 attempts", base)
	}
	// Lenient at create - missing template tokens are tolerated, strict validation kicks in on later recomputes
	if err := s.regenerateProductURLPath(ctx, p.ID, false); err != nil {
		return Product{}, err
	}
	// The create form's later SetProductAttributes upserts over these, so explicit values still win
	if err := s.applyAttributeDefaults(ctx, p.ID, input.ProductTypeID); err != nil {
		return Product{}, err
	}
	// Skip wrapper kinds whose components own the stock - a wrapper with its own row would report "in stock" with no purchasable units behind it
	if s.inventory != nil && p.Kind.OwnsInventory() {
		tracked := pt.TracksInventory
		if input.TracksInventoryOverride != nil {
			tracked = *input.TracksInventoryOverride
		}
		if tracked {
			if err := s.inventory.EnsureItem(ctx, p.ID, true); err != nil {
				return Product{}, err
			}
		}
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &ProductCreated{
			ProductID:     p.ID,
			ProductTypeID: p.ProductTypeID,
			SKU:           p.SKU,
		})
		// Emit the specific bundle event too so subscribers can filter on it instead of every product.created
		if p.Kind == KindBundle {
			s.bus.Publish(ctx, &BundleCreated{ProductID: p.ID, SKU: p.SKU})
		}
	}
	return s.repo.GetProductWithAttributes(ctx, p.ID)
}

func (s *productService) GetProductByID(ctx context.Context, id uuid.UUID) (Product, error) {
	return s.repo.GetProductByID(ctx, id)
}

// The search indexer uses this to emit per-attribute values into the OpenSearch doc
func (s *productService) GetProductWithAttributes(ctx context.Context, id uuid.UUID) (Product, error) {
	return s.repo.GetProductWithAttributes(ctx, id)
}

// Batched form for the bulk reindex - a page of products in five queries rather
// than five per product
func (s *productService) ListProductsWithAttributesByIDs(ctx context.Context, ids []uuid.UUID) ([]Product, error) {
	return s.repo.ListProductsWithAttributesByIDs(ctx, ids)
}

// The inventory reservation listener uses this to map cart-line SKUs to inventory items
func (s *productService) GetProductBySKU(ctx context.Context, sku string) (Product, error) {
	return s.repo.GetProductBySKU(ctx, sku)
}

// One query for the cart-render hydration path instead of a lookup per line + component
func (s *productService) GetProductsBySKUs(ctx context.Context, skus []string) (map[string]Product, error) {
	return s.repo.GetProductsBySKUs(ctx, skus)
}

// Satisfies cart.ProductResolver so the cart enforces its "SIMPLE only" invariant without importing catalog
func (s *productService) ProductKindBySKU(ctx context.Context, sku string) (string, error) {
	p, err := s.repo.GetProductBySKU(ctx, sku)
	if err != nil {
		return "", err
	}
	return string(p.Kind), nil
}

// For callers holding a Product without AttributeSelections - e.g. configurable children, which come back from ListProductChildren bare
func (s *productService) ListProductAttributeSelections(ctx context.Context, productID uuid.UUID) ([]ProductAttributeSelection, error) {
	return s.repo.ListProductAttributeSelections(ctx, productID)
}

// Powers Product.partOf.configurable
func (s *productService) ListConfigurableParentByChildIDs(ctx context.Context, childIDs []uuid.UUID) (map[uuid.UUID]ConfigurableParentLink, error) {
	return s.repo.ListConfigurableParentByChildIDs(ctx, childIDs)
}

// One query instead of an N+1 across a configurable's variants - mirrors LoadProductsContent
func (s *productService) LoadProductsAttributeSelections(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ProductAttributeSelection, error) {
	return s.repo.LoadProductsAttributeSelections(ctx, ids)
}

// Falls back to the default locale per id, the same rule ListProducts uses - ListProductChildren returns bare rows with no content
func (s *core) LoadProductsContent(ctx context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID]ProductContent, error) {
	out := make(map[uuid.UUID]ProductContent)
	if len(ids) == 0 {
		return out, nil
	}
	if locale != "" {
		batch, err := s.repo.ListProductContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, batch)
	}
	def := s.resolveDefaultLocale(ctx)
	if def == "" || def == locale {
		return out, nil
	}
	missing := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	fallback, err := s.repo.ListProductContentByScope(ctx, missing, def)
	if err != nil {
		return nil, err
	}
	maps.Copy(out, fallback)
	return out, nil
}

// Storefront-safe GetProduct - returns only when a published listing exists on the channel, so a guessed or leaked id can't reveal an unpublished product
func (s *productService) GetProductForChannel(ctx context.Context, id, channelID uuid.UUID, locale string) (Product, error) {
	const op = "catalog.Service.GetProductForChannel"
	if _, err := s.repo.GetPublishedProductForChannel(ctx, id, channelID); err != nil {
		if apperr.IsNotFound(err) {
			return Product{}, err
		}
		return Product{}, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	return s.GetProduct(ctx, id, locale)
}

// Batched storefront hydration for browse/search pages - ~8 queries per PAGE where the per-id GetProductForChannel path costs ~9 per product
// Input order is preserved and unpublished/missing ids drop silently, since the index can briefly lag a depublish
func (s *productService) ListProductsForChannelByIDs(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID, locale string) ([]Product, error) {
	const op = "catalog.Service.ListProductsForChannelByIDs"
	if len(ids) == 0 {
		return []Product{}, nil
	}
	visible, err := s.repo.FilterPublishedProductIDsForChannel(ctx, ids, channelID)
	if err != nil {
		return nil, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	keep := make([]uuid.UUID, 0, len(visible))
	for _, id := range ids {
		if _, ok := visible[id]; ok {
			keep = append(keep, id)
		}
	}
	products, err := s.repo.ListProductsWithAttributesByIDs(ctx, keep)
	if err != nil {
		return nil, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	if len(products) == 0 {
		return products, nil
	}
	return s.hydrateContent(ctx, products, locale)
}

// Publication-gates a candidate set (bundle selections, grouped members, children, collection products) - the map lets callers do O(1) lookups over their own ordering
func (s *productService) FilterVisibleProductIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return s.repo.FilterPublishedProductIDsForChannel(ctx, ids, channelID)
}

// Mutates and returns the input slice for the caller's convenience
func (s *productService) hydrateContent(ctx context.Context, products []Product, locale string) ([]Product, error) {
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	if locale != "" {
		batch, err := s.repo.ListProductContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, p := range products {
			if c, ok := batch[p.ID]; ok {
				cp := c
				products[i].Content = &cp
			}
		}
	}
	def := s.resolveDefaultLocale(ctx)
	if def == "" || def == locale {
		return products, nil
	}
	missing := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if p.Content == nil {
			missing = append(missing, p.ID)
		}
	}
	if len(missing) == 0 {
		return products, nil
	}
	fallback, err := s.repo.ListProductContentByScope(ctx, missing, def)
	if err != nil {
		return nil, err
	}
	for i, p := range products {
		if p.Content != nil {
			continue
		}
		if c, ok := fallback[p.ID]; ok {
			cp := c
			products[i].Content = &cp
		}
	}
	return products, nil
}

// Falls back to the global default locale (general/locale/default) when the requested one has no row
func (s *productService) GetProduct(ctx context.Context, id uuid.UUID, locale string) (Product, error) {
	const op = "catalog.Service.GetProduct"
	p, err := s.repo.GetProductWithAttributes(ctx, id)
	if err != nil {
		if apperr.IsNotFound(err) {
			return Product{}, err
		}
		return Product{}, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	if locale != "" {
		c, err := s.repo.GetProductContent(ctx, id, locale)
		switch {
		case err == nil:
			p.Content = &c
			return p, nil
		case apperr.IsNotFound(err):
		default:
			return Product{}, apperr.WrapClass(err, apperr.ClassInternal, op)
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		c, err := s.repo.GetProductContent(ctx, id, def)
		switch {
		case err == nil:
			p.Content = &c
		case apperr.IsNotFound(err):
		default:
			return Product{}, apperr.WrapClass(err, apperr.ClassInternal, op)
		}
	}
	return p, nil
}

// ListProductsWithGraph is ListProducts plus the full attribute graph in a constant number of batch queries - the admin list would otherwise pay a per-product renderProduct fan-out The ceiling for list queries that take no page - uncapped they are 101k rows and ~17MB on a real catalog, which pins a connection for ~20s and is reachable by any admin who can view the catalog Callers that genuinely need everything (the CSV export) pass 0
const MaxUnpagedRows int32 = 1000

func (s *productService) ListProductsWithGraph(ctx context.Context, locale string, rowLimit int32) ([]Product, error) {
	const op = "catalog.Service.ListProductsWithGraph"
	bare, err := s.repo.ListProducts(ctx, rowLimit)
	if err != nil {
		return nil, err
	}
	if len(bare) == 0 {
		return bare, nil
	}
	ids := make([]uuid.UUID, len(bare))
	for i, p := range bare {
		ids[i] = p.ID
	}
	products, err := s.repo.ListProductsWithAttributesByIDs(ctx, ids)
	if err != nil {
		return nil, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	return s.hydrateContent(ctx, products, locale)
}

// ListPageWithGraph is the paged form of ListProductsWithGraph: the filter and sort run in SQL and only that page is hydrated, so the cost
// is set by page size rather than by catalogue size. Returns the page plus the filtered total for the pager
func (s *productService) ListPageWithGraph(ctx context.Context, f ProductPageFilter) ([]Product, int64, error) {
	const op = "catalog.Service.ListPageWithGraph"
	ids, total, err := s.repo.ListProductIDsPaged(ctx, f)
	if err != nil {
		return nil, 0, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	if len(ids) == 0 {
		return []Product{}, total, nil
	}
	// Preserves the id order the page query established, so the SQL sort survives
	products, err := s.repo.ListProductsWithAttributesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, apperr.WrapClass(err, apperr.ClassInternal, op)
	}
	hydrated, err := s.hydrateContent(ctx, products, f.Locale)
	if err != nil {
		return nil, 0, err
	}
	return hydrated, total, nil
}

func (s *productService) ListProducts(ctx context.Context, locale string, rowLimit int32) ([]Product, error) {
	products, err := s.repo.ListProducts(ctx, rowLimit)
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
	if locale != "" {
		batch, err := s.repo.ListProductContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, p := range products {
			if c, ok := batch[p.ID]; ok {
				cp := c
				products[i].Content = &cp
			}
		}
	}
	def := s.resolveDefaultLocale(ctx)
	if def == "" || def == locale {
		return products, nil
	}
	missing := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if p.Content == nil {
			missing = append(missing, p.ID)
		}
	}
	if len(missing) == 0 {
		return products, nil
	}
	fallback, err := s.repo.ListProductContentByScope(ctx, missing, def)
	if err != nil {
		return nil, err
	}
	for i, p := range products {
		if p.Content != nil {
			continue
		}
		if c, ok := fallback[p.ID]; ok {
			cp := c
			products[i].Content = &cp
		}
	}
	return products, nil
}

// Pre-validates the locale so a typo surfaces as locale.ErrLocaleNotFound rather than a raw FK violation
func (s *productService) UpsertProductContent(ctx context.Context, productID uuid.UUID, input ProductContentInput) (Product, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Product{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Locale) == "" {
		return Product{}, fmt.Errorf("locale is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, input.Locale); err != nil {
			return Product{}, err
		}
	}
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return Product{}, err
	}
	if _, err := s.repo.UpsertProductContent(ctx, productID, input); err != nil {
		return Product{}, err
	}
	out, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	// name and description are indexed straight off this row - without the event a rename never reaches search
	s.publishProductUpdated(ctx, out, "content")
	return out, nil
}

// Refuses to drop the default-locale row while other translations exist, so reads always have a fallback target
func (s *productService) DeleteProductContent(ctx context.Context, productID uuid.UUID, locale string) error {
	if def := s.resolveDefaultLocale(ctx); def != "" && locale == def {
		all, err := s.repo.ListProductContent(ctx, productID)
		if err != nil {
			return err
		}
		for _, c := range all {
			if c.Locale != def {
				return ErrCannotDeleteDefaultLocale
			}
		}
	}
	return s.repo.DeleteProductContent(ctx, productID, locale)
}

// The single post-commit "this product's indexed document is stale" signal - mutations outside UpdateProduct (attributes, content, category membership) reuse it so subscribers keep one handler
func (s *core) publishProductUpdated(ctx context.Context, p Product, changed ...string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &ProductUpdated{
		ProductID:     p.ID,
		ProductTypeID: p.ProductTypeID,
		SKU:           p.SKU,
		ChangedFields: changed,
	})
}

// Shared by the sync pre-commit hook and the post-commit async event so both observe one consistent diff list
func changedProductFields(input UpdateProductInput) []string {
	var changed []string
	if input.SKU != nil {
		changed = append(changed, "sku")
	}
	if input.DefaultPrice != nil {
		changed = append(changed, "defaultPrice")
	}
	if input.DefaultCurrencyCode != nil {
		changed = append(changed, "defaultCurrencyCode")
	}
	if input.URL != nil {
		changed = append(changed, "url")
	}
	if input.URLAuto != nil {
		changed = append(changed, "urlAuto")
	}
	if input.URLTemplateOverride != nil {
		changed = append(changed, "urlTemplateOverride")
	}
	if input.TracksInventoryOverride != nil {
		changed = append(changed, "tracksInventoryOverride")
	}
	return changed
}

func (s *productService) UpdateProduct(ctx context.Context, input UpdateProductInput) (Product, error) {
	if input.DefaultPrice != nil && *input.DefaultPrice < 0 {
		return Product{}, fmt.Errorf("default_price must be non-negative")
	}
	if input.DefaultCurrencyCode != nil {
		if len(*input.DefaultCurrencyCode) != 3 {
			return Product{}, ErrInvalidCurrency
		}
		upper := strings.ToUpper(*input.DefaultCurrencyCode)
		input.DefaultCurrencyCode = &upper
	}
	if input.SKU != nil {
		trimmed := strings.TrimSpace(*input.SKU)
		if trimmed == "" {
			return Product{}, fmt.Errorf("sku is required and cannot be cleared")
		}
		input.SKU = &trimmed
	}
	current, err := s.repo.GetProductByID(ctx, input.ID)
	if err != nil {
		return Product{}, err
	}
	// Sync pre-tx hook - external systems can reject the update or contribute tags
	changed := changedProductFields(input)
	if s.bus != nil {
		ing := &ProductUpdating{
			ProductID:     current.ID,
			ProductTypeID: current.ProductTypeID,
			SKU:           current.SKU,
			ChangedFields: changed,
		}
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Product{}, err
		}
		if len(ing.Tags) > 0 {
			merged := Metadata{}
			maps.Copy(merged, current.Metadata)
			if input.Metadata != nil {
				maps.Copy(merged, *input.Metadata)
			}
			merged["webhook_tags"] = strings.Join(ing.Tags, ",")
			input.Metadata = &merged
		}
	}
	// Reject a manual URL while auto is on, or being turned on
	autoAfter := current.URLAuto
	if input.URLAuto != nil {
		autoAfter = *input.URLAuto
	}
	if autoAfter && input.URL != nil {
		return Product{}, ErrManualPathWhileAuto
	}
	oldPath := current.URL
	updated, err := s.repo.UpdateProduct(ctx, input)
	if err != nil {
		return Product{}, err
	}
	// The override column is inert on its own (Salable reads inventory_items.tracked), so without this reconcile an admin flipping "track inventory" would see no effect
	if s.inventory != nil && current.Kind.OwnsInventory() &&
		(input.TracksInventoryOverride != nil || input.ClearTracksInventoryOverride) {
		pt, err := s.repo.GetProductTypeByID(ctx, current.ProductTypeID)
		if err != nil {
			return Product{}, err
		}
		effective := pt.TracksInventory
		if input.TracksInventoryOverride != nil {
			effective = *input.TracksInventoryOverride
		}
		if err := s.inventory.SyncProductTracking(ctx, current.ID, effective); err != nil {
			return Product{}, err
		}
	}
	// Strict here - a recompute that hits missing data aborts the update
	pathRelevant := input.URL != nil || input.URLAuto != nil || input.URLTemplateOverride != nil
	if pathRelevant {
		if autoAfter {
			if err := s.regenerateProductURLPath(ctx, updated.ID, true); err != nil {
				return Product{}, err
			}
		} else if s.urlRewrites != nil {
			newPath := updated.URL
			if newPath != oldPath {
				// Keeps product.url in step if the manual url collides and gets suffixed
				finalPath, err := s.persistAndSyncWithRetry(ctx, updated.ID, newPath)
				if err != nil {
					return Product{}, err
				}
				if finalPath != newPath {
					s.noteURLCollision(ctx, "product", updated.SKU, updated.ID, newPath, finalPath)
				}
				s.createRenameRedirect(ctx, oldPath, "product", updated.ID)
			}
		}
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &ProductUpdated{
			ProductID:     updated.ID,
			ProductTypeID: updated.ProductTypeID,
			SKU:           updated.SKU,
			ChangedFields: changed,
		})
		if updated.Kind == KindBundle {
			s.bus.Publish(ctx, &BundleUpdated{ProductID: updated.ID, SKU: updated.SKU, ChangedFields: changed})
		}
	}
	return s.repo.GetProductByID(ctx, updated.ID)
}

func (s *productService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	parents, err := s.repo.ListProductParents(ctx, id)
	if err != nil {
		return err
	}
	if len(parents) > 0 {
		return fmt.Errorf("%w (%d parent(s))", ErrProductIsChild, len(parents))
	}
	// Snapshot identity for the post-commit event before the row is gone
	pre, getErr := s.repo.GetProductByID(ctx, id)
	if err := s.repo.DeleteProduct(ctx, id); err != nil {
		return err
	}
	// Best-effort - a leftover rewrite is an SEO problem, not a reason to fail a delete the operator just confirmed
	if s.urlRewrites != nil {
		if err := s.urlRewrites.DeleteAllForEntity(ctx, "product", id); err != nil {
			applog.From(ctx).Error("delete url rewrites",
				"op", "catalog.Service.DeleteProduct",
				"product_id", id,
				"err", err)
		}
	}
	if s.bus != nil && getErr == nil {
		s.bus.Publish(ctx, &ProductDeleted{
			ProductID:     pre.ID,
			ProductTypeID: pre.ProductTypeID,
			SKU:           pre.SKU,
		})
		if pre.Kind == KindBundle {
			s.bus.Publish(ctx, &BundleDeleted{ProductID: pre.ID, SKU: pre.SKU})
		}
	}
	return nil
}

// ==================== Configurable Options ====================

// Only dropdown/swatch are accepted and all must be assigned to the product's type - replaces any previously set options
func (s *productService) SetConfigurableOptions(ctx context.Context, productID uuid.UUID, attributeIDs []uuid.UUID) (Product, error) {
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	// A non-configurable calling this is a programming bug - reject rather than write rows that do nothing
	if !p.IsConfigurable() {
		return Product{}, fmt.Errorf("%w: product %q is kind %q", ErrParentNotConfigurable, p.SKU, string(p.Kind))
	}
	// list-mode configurables are attribute-less by definition
	if p.EffectiveVariantMode() == VariantModeList {
		return Product{}, ErrListModeForbidsOptions
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, p.ProductTypeID)
	if err != nil {
		return Product{}, err
	}

	// Block changes while children are linked - existing children may not satisfy the new options
	existing, err := s.repo.ListProductChildren(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	if len(existing) > 0 {
		return Product{}, fmt.Errorf("cannot change configurable options while children are linked; remove all children first")
	}

	seen := make(map[uuid.UUID]struct{}, len(attributeIDs))
	for _, id := range attributeIDs {
		if _, dup := seen[id]; dup {
			return Product{}, fmt.Errorf("duplicate attribute id in configurable options: %s", id)
		}
		seen[id] = struct{}{}
	}

	typeAttrs := make(map[uuid.UUID]Attribute, len(pt.Attributes))
	for _, a := range pt.Attributes {
		typeAttrs[a.Attribute.ID] = a.Attribute
	}

	options := make([]AssignAttributeInput, 0, len(attributeIDs))
	for i, attrID := range attributeIDs {
		attr, ok := typeAttrs[attrID]
		if !ok {
			return Product{}, fmt.Errorf("%w: %s", ErrConfigurableOptionNotAssigned, attrID)
		}
		if attr.InputType != InputTypeDropdown && attr.InputType != InputTypeSwatch && attr.InputType != InputTypeMultiselect {
			return Product{}, fmt.Errorf("%w: attribute %q has input_type %q; only dropdown, swatch, and multiselect are valid", ErrConfigurableOptionNotSelectable, attr.Slug, attr.InputType)
		}
		options = append(options, AssignAttributeInput{
			AttributeID: attrID,
			SortOrder:   int32(i),
		})
	}

	if err := s.repo.SetConfigurableOptions(ctx, productID, options); err != nil {
		return Product{}, err
	}
	return s.repo.GetProductWithAttributes(ctx, productID)
}

// Defaults a nil (never-set) mode to attribute - a method on Product so the rule lives with the entity
func (p Product) EffectiveVariantMode() VariantMode {
	if p.VariantMode != nil {
		return *p.VariantMode
	}
	return VariantModeAttribute
}

func (p Product) IsConfigurable() bool { return p.Kind == KindConfigurable }

// LIST mode forbids configurable options, so any have to be removed first
func (s *productService) SetProductVariantMode(ctx context.Context, productID uuid.UUID, mode VariantMode) (Product, error) {
	if !mode.IsValid() {
		return Product{}, fmt.Errorf("invalid variant mode %q", mode)
	}
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	if !p.IsConfigurable() {
		return Product{}, fmt.Errorf("%w: product %q is kind %q", ErrParentNotConfigurable, p.SKU, string(p.Kind))
	}
	if mode == VariantModeList {
		opts, err := s.repo.ListConfigurableOptions(ctx, productID)
		if err != nil {
			return Product{}, err
		}
		if len(opts) > 0 {
			return Product{}, ErrListModeForbidsOptions
		}
	}
	// The mirror guard: attribute mode with linked children but no option axis is a trap - options cannot be added while children are linked
	// and the storefront's matchVariant would vacuously match the first child with no options to pick
	if mode == VariantModeAttribute && p.EffectiveVariantMode() == VariantModeList {
		children, err := s.repo.ListProductChildren(ctx, productID)
		if err != nil {
			return Product{}, err
		}
		if len(children) > 0 {
			return Product{}, fmt.Errorf("%w: unlink the %d list children first", ErrConfigurableNeedsOption, len(children))
		}
	}
	return s.repo.SetProductVariantMode(ctx, productID, mode)
}

// ==================== Product Children ====================

// Bare product rows - no attribute selections, no localized content
func (s *productService) ListProductChildren(ctx context.Context, parentID uuid.UUID) ([]Product, error) {
	return s.repo.ListProductChildren(ctx, parentID)
}

// Validates the parent is configurable, the child covers every configurable option, and its value combination is unique among siblings
func (s *productService) AddProductChild(ctx context.Context, parentID, childID uuid.UUID, sortOrder int32) (Product, error) {
	if parentID == childID {
		return Product{}, ErrCannotAddSelfAsChild
	}

	// BFS up the ancestor graph - reject if childID shows up, that would loop the DAG
	cycle, err := s.wouldCreateCycle(ctx, parentID, childID)
	if err != nil {
		return Product{}, err
	}
	if cycle {
		return Product{}, ErrCyclicChildLink
	}

	parent, err := s.repo.GetProductByID(ctx, parentID)
	if err != nil {
		return Product{}, err
	}
	// Kind-axis invariants first - a wrong-kind parent/child is a programming bug, not the operational "missing color value" case the attribute checks handle
	if !parent.IsConfigurable() {
		return Product{}, fmt.Errorf("%w: parent %q is kind %q", ErrParentNotConfigurable, parent.SKU, string(parent.Kind))
	}

	child, err := s.repo.GetProductByID(ctx, childID)
	if err != nil {
		return Product{}, err
	}
	if child.Kind != KindSimple {
		return Product{}, fmt.Errorf("%w: child %q is kind %q", ErrChildNotSimple, child.SKU, string(child.Kind))
	}
	if parent.ProductTypeID != child.ProductTypeID {
		return Product{}, ErrChildProductTypeMismatch
	}

	// A simple can be a variant of only ONE configurable (UNIQUE(child_id)) - surface a clean conflict instead of a raw index violation
	existingParents, err := s.repo.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{childID})
	if err != nil {
		return Product{}, err
	}
	if link := existingParents[childID]; link.ParentID != uuid.Nil && link.ParentID != parentID {
		return Product{}, fmt.Errorf("%w: child %q already belongs to configurable %s", ErrChildAlreadyHasParent, child.SKU, link.ParentID)
	}

	opts, err := s.repo.ListConfigurableOptions(ctx, parentID)
	if err != nil {
		return Product{}, err
	}

	// Attribute-mode needs an option axis before children link - list-mode links freely and skips the uniqueness checks below
	if parent.EffectiveVariantMode() == VariantModeAttribute && len(opts) == 0 {
		return Product{}, ErrConfigurableNeedsOption
	}

	if len(opts) > 0 {
		optIDs := make([]uuid.UUID, len(opts))
		for i, o := range opts {
			optIDs[i] = o.ID
		}

		childSelectable, err := s.repo.ListProductSelectableValuesByIDs(ctx, []uuid.UUID{childID}, optIDs)
		if err != nil {
			return Product{}, err
		}
		var missing []string
		for _, o := range opts {
			vals := childSelectable[childID][o.ID]
			if len(vals) == 0 {
				missing = append(missing, o.Slug)
			}
		}
		if len(missing) > 0 {
			return Product{}, fmt.Errorf("%w: %s", ErrChildMissingConfigurableOptions, strings.Join(missing, ", "))
		}

		existingSigs, err := s.existingChildSignatures(ctx, s.repo, parentID, optIDs, uuid.Nil)
		if err != nil {
			return Product{}, err
		}
		childSig := superAttrSignature(optIDs, childSelectable[childID])
		if _, dup := existingSigs[childSig]; dup {
			return Product{}, ErrDuplicateSuperAttributeCombination
		}
	}

	if err := s.repo.AddProductChild(ctx, parentID, childID, sortOrder); err != nil {
		return Product{}, err
	}
	return s.repo.GetProductWithAttributes(ctx, parentID)
}

// BFS over the ancestors of parentID - true if childID is already up there
func (s *productService) wouldCreateCycle(ctx context.Context, parentID, childID uuid.UUID) (bool, error) {
	visited := make(map[uuid.UUID]bool)
	queue := []uuid.UUID{parentID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == childID {
			return true, nil
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		parents, err := s.repo.ListProductParents(ctx, current)
		if err != nil {
			return false, err
		}
		for _, p := range parents {
			queue = append(queue, p.ID)
		}
	}
	return false, nil
}

// Canonical string of a product's option values, used to spot duplicate child combinations
func superAttrSignature(optIDs []uuid.UUID, selectable map[uuid.UUID][]uuid.UUID) string {
	parts := make([]string, 0, len(optIDs))
	for _, attrID := range optIDs {
		key := attrID.String() + "="
		if vals, ok := selectable[attrID]; ok {
			sorted := make([]string, len(vals))
			for i, v := range vals {
				sorted[i] = v.String()
			}
			for i := 1; i < len(sorted); i++ {
				for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
					sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
				}
			}
			key += strings.Join(sorted, ",")
		}
		parts = append(parts, key)
	}
	return strings.Join(parts, "|")
}

// Single source of truth for "no two variants share an option combination" - used at link time, batch generation and after attribute edits so the three can't drift
// Takes an explicit repo so a caller inside a transaction can pass its tx-scoped one
func (s *productService) existingChildSignatures(ctx context.Context, r Repository, parentID uuid.UUID, optIDs []uuid.UUID, excludeID uuid.UUID) (map[string]string, error) {
	children, err := r.ListProductChildren(ctx, parentID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(children))
	for _, c := range children {
		if c.ID != excludeID {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	selectable, err := r.ListProductSelectableValuesByIDs(ctx, ids, optIDs)
	if err != nil {
		return nil, err
	}
	sigs := make(map[string]string, len(ids))
	for _, c := range children {
		if c.ID == excludeID {
			continue
		}
		sigs[superAttrSignature(optIDs, selectable[c.ID])] = c.SKU
	}
	return sigs, nil
}

func (s *productService) RemoveProductChild(ctx context.Context, parentID, childID uuid.UUID) (Product, error) {
	if err := s.repo.RemoveProductChild(ctx, parentID, childID); err != nil {
		return Product{}, err
	}
	return s.repo.GetProductWithAttributes(ctx, parentID)
}

// Best-effort batch - every row is validated up front so the common failures land before any writes Anything that slips through mid-loop stops processing and returns the partial result wrapped with the failing row's index Deliberately not one outer transaction: CreateProduct already retries SKU collisions and a batch-wide rollback on each one is worse than the rare orphan SIMPLE this can leave
func (s *productService) GenerateConfigurableChildren(ctx context.Context, parentID uuid.UUID, inputs []GenerateConfigurableChildInput) ([]Product, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one variant is required")
	}
	parent, rowSelections, err := s.validateConfigurableChildrenBatch(ctx, parentID, inputs)
	if err != nil {
		return nil, err
	}

	created := make([]Product, 0, len(inputs))
	for i, in := range inputs {
		locale := strings.TrimSpace(in.Locale)
		if locale == "" {
			locale = s.resolveDefaultLocale(ctx)
		}
		currency := strings.TrimSpace(in.DefaultCurrencyCode)
		if currency == "" {
			currency = parent.DefaultCurrencyCode
		}
		price := in.DefaultPrice
		if price == 0 {
			price = parent.DefaultPrice
		}
		child, err := s.CreateProduct(ctx, CreateProductInput{
			ProductTypeID:       parent.ProductTypeID,
			Kind:                KindSimple,
			SKU:                 in.SKU,
			DefaultPrice:        price,
			DefaultCurrencyCode: currency,
			Locale:              locale,
			Name:                in.Name,
		})
		if err != nil {
			return created, fmt.Errorf("variant %d (%q): create: %w", i, in.Name, err)
		}
		assignments := make([]AttributeAssignment, 0, len(rowSelections[i]))
		for attrID, valueIDs := range rowSelections[i] {
			assignments = append(assignments, AttributeAssignment{
				AttributeID: attrID,
				ValueIDs:    valueIDs,
			})
		}
		if _, err := s.SetProductAttributes(ctx, child.ID, assignments); err != nil {
			return created, fmt.Errorf("variant %d (%q): set attributes: %w", i, in.Name, err)
		}
		if _, err := s.AddProductChild(ctx, parentID, child.ID, in.SortOrder); err != nil {
			return created, fmt.Errorf("variant %d (%q): link as child: %w", i, in.Name, err)
		}
		created = append(created, child)
	}
	return created, nil
}

// All pre-write validation for GenerateConfigurableChildren - returns the loaded parent and the per-row attribute->value selections the write loop consumes
func (s *productService) validateConfigurableChildrenBatch(ctx context.Context, parentID uuid.UUID, inputs []GenerateConfigurableChildInput) (Product, []map[uuid.UUID][]uuid.UUID, error) {
	if len(inputs) == 0 {
		return Product{}, nil, fmt.Errorf("at least one variant is required")
	}
	parent, err := s.repo.GetProductByID(ctx, parentID)
	if err != nil {
		return Product{}, nil, err
	}
	if !parent.IsConfigurable() {
		return Product{}, nil, fmt.Errorf("%w: parent %q is kind %q", ErrParentNotConfigurable, parent.SKU, string(parent.Kind))
	}
	opts, err := s.repo.ListConfigurableOptions(ctx, parentID)
	if err != nil {
		return Product{}, nil, err
	}
	if len(opts) == 0 {
		return Product{}, nil, fmt.Errorf("parent %q has no configurable options; set them before generating variants", parent.SKU)
	}
	optIDs := make([]uuid.UUID, len(opts))
	optByID := make(map[uuid.UUID]Attribute, len(opts))
	for i, o := range opts {
		optIDs[i] = o.ID
		optByID[o.ID] = o
	}

	// ListConfigurableOptions returns Attributes without their Values, so hydrate each one - N is 1-3 options, not worth a batch query
	valueToAttr := make(map[uuid.UUID]uuid.UUID)
	for _, o := range opts {
		hydrated, err := s.repo.GetAttributeWithValues(ctx, o.ID)
		if err != nil {
			return Product{}, nil, fmt.Errorf("load values for option %q: %w", o.Slug, err)
		}
		for _, v := range hydrated.Values {
			valueToAttr[v.ID] = o.ID
		}
	}

	signatures := make(map[string]int, len(inputs))
	rowSignatures := make([]string, len(inputs))
	rowSelections := make([]map[uuid.UUID][]uuid.UUID, len(inputs)) // attribute_id → []value_id
	for i, in := range inputs {
		if len(in.AttributeValueIDs) != len(opts) {
			return Product{}, nil, fmt.Errorf("variant %d: expected %d configurable option values, got %d", i, len(opts), len(in.AttributeValueIDs))
		}
		sel := make(map[uuid.UUID][]uuid.UUID, len(opts))
		for _, vID := range in.AttributeValueIDs {
			attrID, ok := valueToAttr[vID]
			if !ok {
				return Product{}, nil, fmt.Errorf("variant %d: value %s does not belong to any configurable option of parent %q", i, vID, parent.SKU)
			}
			if _, dup := sel[attrID]; dup {
				return Product{}, nil, fmt.Errorf("variant %d: two values supplied for option %q", i, optByID[attrID].Slug)
			}
			sel[attrID] = []uuid.UUID{vID}
		}
		for _, oID := range optIDs {
			if _, ok := sel[oID]; !ok {
				return Product{}, nil, fmt.Errorf("variant %d: missing value for option %q", i, optByID[oID].Slug)
			}
		}
		rowSelections[i] = sel
		sig := superAttrSignature(optIDs, sel)
		rowSignatures[i] = sig
		if existing, dup := signatures[sig]; dup {
			return Product{}, nil, fmt.Errorf("variants %d and %d have the same option combination", existing, i)
		}
		signatures[sig] = i
	}

	existingSigs, err := s.existingChildSignatures(ctx, s.repo, parentID, optIDs, uuid.Nil)
	if err != nil {
		return Product{}, nil, err
	}
	for i, sig := range rowSignatures {
		if conflict, found := existingSigs[sig]; found {
			return Product{}, nil, fmt.Errorf("variant %d: option combination already exists on child %q", i, conflict)
		}
	}
	return parent, rowSelections, nil
}

// ==================== Attribute Assignment ====================

// Central write path for product attribute data - branches on input_type (scalar to JSONB, selectable to the junction) and applies atomically
// Upsert semantics: attributes absent from the batch are left alone, clear one with empty ValueIDs or HasScalar plus a nil ScalarValue
func (s *productService) SetProductAttributes(ctx context.Context, productID uuid.UUID, assignments []AttributeAssignment) (Product, error) {
	p, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, p.ProductTypeID)
	if err != nil {
		return Product{}, err
	}

	scalars, selections, err := s.resolveAttributeWrites(ctx, pt, p.Attributes, p.AttributeSelections, assignments)
	if err != nil {
		return Product{}, err
	}

	// Re-validate variant uniqueness in the same transaction - AddProductChild guards link time, but later edits would slip past
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := r.SetProductAttributes(ctx, productID, scalars, selections); err != nil {
			return err
		}
		return s.revalidateVariantUniqueness(ctx, r, productID)
	})
	if err != nil {
		return Product{}, err
	}
	if err := s.regenerateProductURLPath(ctx, productID, true); err != nil {
		return Product{}, err
	}
	// A cleared scalar's translations describe a value that no longer exists - drop them so they can't resurface
	for _, a := range assignments {
		if a.HasScalar && a.ScalarValue == nil {
			if err := s.repo.DeleteProductAttributeLocaleValuesForAttribute(ctx, productID, a.AttributeID); err != nil {
				return Product{}, err
			}
		}
	}
	out, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	// every attr_* / attrslug_* field in the indexed doc derives from here
	s.publishProductUpdated(ctx, out, "attributes")
	return out, nil
}

// Validates the attribute is a reference type on the product's type, every target matches its declared relationshipTarget, and every target entity exists
func (s *productService) SetProductAttributeReferences(ctx context.Context, productID, attributeID uuid.UUID, refs []ProductAttributeReference) (Product, error) {
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return Product{}, err
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, p.ProductTypeID)
	if err != nil {
		return Product{}, err
	}
	var attr *Attribute
	for i := range pt.Attributes {
		if pt.Attributes[i].Attribute.ID == attributeID {
			attr = &pt.Attributes[i].Attribute
			break
		}
	}
	if attr == nil {
		return Product{}, fmt.Errorf("%w: %s", ErrAttributeNotAssignedToType, attributeID)
	}
	if !attr.InputType.IsReference() {
		return Product{}, fmt.Errorf("%w: %q", ErrAttributeNotReference, attr.Slug)
	}

	for _, ref := range refs {
		if ref.TargetType != attr.RelationshipTarget {
			return Product{}, fmt.Errorf("%w: attribute %q targets %q, got %q", ErrReferenceTargetMismatch, attr.Slug, attr.RelationshipTarget, ref.TargetType)
		}
	}

	// All refs share one target type, so a single existence lookup covers them
	if len(refs) > 0 {
		ids := make([]uuid.UUID, 0, len(refs))
		for _, ref := range refs {
			ids = append(ids, ref.TargetID)
		}
		found, err := s.referenceTargetsExist(ctx, attr.RelationshipTarget, ids, s.resolveDefaultLocale(ctx))
		if err != nil {
			return Product{}, err
		}
		for _, ref := range refs {
			if !found[ref.TargetID] {
				return Product{}, fmt.Errorf("%w: %s %s", ErrReferenceTargetNotFound, ref.TargetType, ref.TargetID)
			}
		}
	}

	ordered := make([]ProductAttributeReference, len(refs))
	for i, ref := range refs {
		ref.Position = int32(i)
		ordered[i] = ref
	}
	if err := s.repo.SetProductAttributeReferences(ctx, productID, attributeID, ordered); err != nil {
		return Product{}, err
	}
	return s.repo.GetProductWithAttributes(ctx, productID)
}

// Same content-scope lookups LinkLabelsByIDs uses
func (s *productService) referenceTargetsExist(ctx context.Context, target ReferenceTarget, ids []uuid.UUID, locale string) (map[uuid.UUID]bool, error) {
	found := make(map[uuid.UUID]bool, len(ids))
	switch target {
	case ReferenceTargetProduct:
		m, err := s.repo.ListProductContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for id := range m {
			found[id] = true
		}
	case ReferenceTargetCategory:
		m, err := s.repo.ListCategoryContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for id := range m {
			found[id] = true
		}
	case ReferenceTargetCollection:
		m, err := s.repo.ListCollectionContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for id := range m {
			found[id] = true
		}
	}
	return found, nil
}

// After an attribute edit, re-check this product's combination against its siblings under every parent it's linked to - mirrors the link-time check in AddProductChild
func (s *productService) revalidateVariantUniqueness(ctx context.Context, r Repository, productID uuid.UUID) error {
	parents, err := r.ListProductParents(ctx, productID)
	if err != nil {
		return err
	}
	for _, parent := range parents {
		opts, err := r.ListConfigurableOptions(ctx, parent.ID)
		if err != nil {
			return err
		}
		if len(opts) == 0 {
			continue
		}
		optIDs := make([]uuid.UUID, len(opts))
		for i, o := range opts {
			optIDs[i] = o.ID
		}
		siblingSigs, err := s.existingChildSignatures(ctx, r, parent.ID, optIDs, productID)
		if err != nil {
			return err
		}
		selfSel, err := r.ListProductSelectableValuesByIDs(ctx, []uuid.UUID{productID}, optIDs)
		if err != nil {
			return err
		}
		// Coverage first, mirroring AddProductChild - an edit that clears an option
		// value would leave a variant the picker can never match, silently unsellable
		var missing []string
		for _, o := range opts {
			if len(selfSel[productID][o.ID]) == 0 {
				missing = append(missing, o.Slug)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%w: %s", ErrChildMissingConfigurableOptions, strings.Join(missing, ", "))
		}
		selfSig := superAttrSignature(optIDs, selfSel[productID])
		if _, dup := siblingSigs[selfSig]; dup {
			return ErrDuplicateSuperAttributeCombination
		}
	}
	return nil
}

// Merges the batch over existing scalars/selections so attributes absent from it keep their values
func (s *productService) resolveAttributeWrites(
	ctx context.Context,
	pt ProductType,
	existingScalars ProductScalarAttributes,
	existingSelections []ProductAttributeSelection,
	assignments []AttributeAssignment,
) (ProductScalarAttributes, []ProductAttributeSelection, error) {
	allowed := make(map[uuid.UUID]Attribute, len(pt.Attributes))
	for _, a := range pt.Attributes {
		allowed[a.Attribute.ID] = a.Attribute
	}

	scalars := existingScalars
	if scalars == nil {
		scalars = ProductScalarAttributes{}
	}
	touchedAttrs := make(map[uuid.UUID]struct{}, len(assignments))
	for _, a := range assignments {
		touchedAttrs[a.AttributeID] = struct{}{}
	}

	var newSelections []ProductAttributeSelection
	for _, a := range assignments {
		attr, ok := allowed[a.AttributeID]
		if !ok {
			return nil, nil, fmt.Errorf("%w: %s", ErrAttributeNotAssignedToType, a.AttributeID)
		}
		if attr.InputType.AcceptsValues() {
			if a.HasScalar {
				return nil, nil, fmt.Errorf("%w: %s", ErrScalarValueOnSelectableAttr, attr.Slug)
			}
			switch attr.InputType {
			case InputTypeDropdown, InputTypeSwatch:
				if len(a.ValueIDs) > 1 {
					return nil, nil, fmt.Errorf("%w: %s has %d values", ErrDropdownRequiresSingleValue, attr.Slug, len(a.ValueIDs))
				}
			}
			if attr.ValueRequired && len(a.ValueIDs) == 0 {
				return nil, nil, fmt.Errorf("%w: %s", ErrRequiredAttributeMissing, attr.Slug)
			}
			resolved, err := s.resolveAttributeValues(ctx, attr, a.ValueIDs)
			if err != nil {
				return nil, nil, err
			}
			for i, v := range resolved {
				newSelections = append(newSelections, ProductAttributeSelection{
					AttributeID: attr.ID,
					Value:       v,
					SortOrder:   int32(i),
				})
			}
			delete(scalars, attr.Slug)
		} else {
			if len(a.ValueIDs) > 0 {
				return nil, nil, fmt.Errorf("%w: %s", ErrSelectableValueOnScalarAttr, attr.Slug)
			}
			if !a.HasScalar {
				continue
			}
			if a.ScalarValue == nil {
				delete(scalars, attr.Slug)
				continue
			}
			if err := validateScalarValue(attr.InputType, a.ScalarValue); err != nil {
				return nil, nil, fmt.Errorf("attribute %q: %w", attr.Slug, err)
			}
			if attr.ValueRequired && isZeroScalar(a.ScalarValue) {
				return nil, nil, fmt.Errorf("%w: %s", ErrRequiredAttributeMissing, attr.Slug)
			}
			scalars[attr.Slug] = a.ScalarValue
		}
	}

	mergedSelections := newSelections
	for _, existing := range existingSelections {
		if _, touched := touchedAttrs[existing.AttributeID]; touched {
			continue
		}
		mergedSelections = append(mergedSelections, existing)
	}
	return scalars, mergedSelections, nil
}

// Removes one attribute entirely, both paths, so callers don't have to build a cleared assignment
func (s *productService) ClearProductAttribute(ctx context.Context, productID, attributeID uuid.UUID) (Product, error) {
	attr, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return Product{}, err
	}
	a := AttributeAssignment{
		AttributeID: attributeID,
	}
	if attr.InputType.AcceptsValues() {
		a.ValueIDs = []uuid.UUID{} // empty = clear
	} else {
		a.HasScalar = true
		a.ScalarValue = nil
	}
	return s.SetProductAttributes(ctx, productID, []AttributeAssignment{a})
}

// ==================== helpers ====================

// Rejects an id that's unknown or belongs to a different attribute - either would corrupt filtering semantics
func (s *productService) resolveAttributeValues(ctx context.Context, attr Attribute, ids []uuid.UUID) ([]AttributeValue, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	loaded, err := s.repo.GetAttributeWithValues(ctx, attr.ID)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]AttributeValue, len(loaded.Values))
	for _, v := range loaded.Values {
		byID[v.ID] = v
	}
	out := make([]AttributeValue, 0, len(ids))
	for _, id := range ids {
		v, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: value %s is not a value of attribute %q", ErrAttributeValueMismatch, id, attr.Slug)
		}
		out = append(out, v)
	}
	return out, nil
}

// Date parsing is strict ISO 8601
func validateScalarValue(inputType InputType, v any) error {
	if v == nil {
		return nil // treated as clear upstream
	}
	switch inputType {
	case InputTypePlainText:
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%w: expected string for plain_text", ErrInvalidScalarType)
		}
	case InputTypeNumeric:
		switch v.(type) {
		case float64, int, int32, int64:
		default:
			return fmt.Errorf("%w: expected number for numeric", ErrInvalidScalarType)
		}
	case InputTypeBoolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%w: expected bool for boolean", ErrInvalidScalarType)
		}
	case InputTypeDate:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected string date for date", ErrInvalidScalarType)
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("%w: %q is not a YYYY-MM-DD date: %w", ErrInvalidScalarType, s, err)
		}
	case InputTypeDateTime:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected string datetime for date_time", ErrInvalidScalarType)
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return fmt.Errorf("%w: %q is not an RFC3339 datetime: %w", ErrInvalidScalarType, s, err)
		}
	default:
		return fmt.Errorf("%w: unhandled input type %q", ErrInvalidScalarType, inputType)
	}
	return nil
}

// Built from the product type's full attribute view so selectable defaults resolve value IDs without an extra round trip
func (s *productService) applyAttributeDefaults(ctx context.Context, productID, productTypeID uuid.UUID) error {
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, productTypeID)
	if err != nil {
		return fmt.Errorf("load product type for defaults: %w", err)
	}
	var assignments []AttributeAssignment
	for _, pta := range pt.Attributes {
		attr := pta.Attribute
		if len(attr.DefaultValue) == 0 {
			continue
		}
		if attr.InputType.AcceptsValues() {
			ids, err := decodeSelectableDefaultIDs(attr.InputType, attr.DefaultValue)
			if err != nil {
				// A malformed default means something bypassed the service - skip rather than fail the whole create
				continue
			}
			if len(ids) == 0 {
				continue
			}
			assignments = append(assignments, AttributeAssignment{
				AttributeID: attr.ID,
				ValueIDs:    ids,
			})
		} else {
			var v any
			if err := json.Unmarshal(attr.DefaultValue, &v); err != nil {
				continue
			}
			assignments = append(assignments, AttributeAssignment{
				AttributeID: attr.ID,
				ScalarValue: v,
				HasScalar:   true,
			})
		}
	}
	if len(assignments) == 0 {
		return nil
	}
	if _, err := s.SetProductAttributes(ctx, productID, assignments); err != nil {
		return fmt.Errorf("apply attribute defaults: %w", err)
	}
	return nil
}

// dropdown/swatch is a single id wrapped as [], multiselect is an array of ids
func decodeSelectableDefaultIDs(inputType InputType, raw json.RawMessage) ([]uuid.UUID, error) {
	switch inputType {
	case InputTypeDropdown, InputTypeSwatch:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		return []uuid.UUID{id}, nil
	case InputTypeMultiselect:
		var ss []string
		if err := json.Unmarshal(raw, &ss); err != nil {
			return nil, err
		}
		out := make([]uuid.UUID, 0, len(ss))
		for _, s := range ss {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("input type %q does not accept selectable defaults", inputType)
	}
}

// Reuses validateScalarValue so the "what counts as valid" rule lives in one place
func validateScalarDefault(inputType InputType, raw json.RawMessage) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefaultValue, err)
	}
	if err := validateScalarValue(inputType, v); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefaultValue, err)
	}
	return nil
}

// dropdown/swatch expect one value-id string, multiselect a non-empty array - attr.Values must already be loaded
func validateSelectableDefault(attr Attribute, raw json.RawMessage) error {
	known := make(map[uuid.UUID]struct{}, len(attr.Values))
	for _, v := range attr.Values {
		known[v.ID] = struct{}{}
	}
	parseID := func(s string) (uuid.UUID, error) {
		id, err := uuid.Parse(s)
		if err != nil {
			return uuid.UUID{}, fmt.Errorf("%w: %q is not a valid uuid", ErrInvalidDefaultValue, s)
		}
		if _, ok := known[id]; !ok {
			return uuid.UUID{}, fmt.Errorf("%w: value %s is not a value of attribute %q", ErrInvalidDefaultValue, id, attr.Slug)
		}
		return id, nil
	}
	switch attr.InputType {
	case InputTypeDropdown, InputTypeSwatch:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("%w: expected a value-id string for %s default", ErrInvalidDefaultValue, attr.InputType)
		}
		if _, err := parseID(s); err != nil {
			return err
		}
	case InputTypeMultiselect:
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return fmt.Errorf("%w: expected an array of value-id strings for multiselect default", ErrInvalidDefaultValue)
		}
		if len(ids) == 0 {
			return fmt.Errorf("%w: multiselect default must contain at least one value id", ErrInvalidDefaultValue)
		}
		seen := make(map[uuid.UUID]struct{}, len(ids))
		for _, s := range ids {
			id, err := parseID(s)
			if err != nil {
				return err
			}
			if _, dup := seen[id]; dup {
				return fmt.Errorf("%w: duplicate value id %s in multiselect default", ErrInvalidDefaultValue, id)
			}
			seen[id] = struct{}{}
		}
	default:
		return fmt.Errorf("%w: input type %q does not accept selectable defaults", ErrInvalidDefaultValue, attr.InputType)
	}
	return nil
}

// DeleteAttributeValue uses this to refuse removing a value that's currently wired as a default
func defaultReferencesValue(raw json.RawMessage, valueID uuid.UUID) bool {
	if len(raw) == 0 {
		return false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if id, err := uuid.Parse(single); err == nil && id == valueID {
			return true
		}
		return false
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, s := range many {
			if id, err := uuid.Parse(s); err == nil && id == valueID {
				return true
			}
		}
	}
	return false
}

// Only [a-z0-9] survives - everything else collapses into a single hyphen
func slugifyForSKU(name string) string {
	var b strings.Builder
	prev := rune('-')
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(b.String(), "-")
}

// Booleans don't count - false is a valid value
func isZeroScalar(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) == ""
	case float64:
		return x == 0
	case int:
		return x == 0
	case int32:
		return x == 0
	case int64:
		return x == 0
	case nil:
		return true
	}
	return false
}

// ==================== URL path auto-generation ====================

// Recomputes url_path from the effective template, persists it, then syncs the canonical rewrite - no-op when URLAuto is false strict aborts on a missing primary category or attribute value; lenient tolerates missing tokens and lets the slug stand, and is only used at create time where those are attached by later mutations A template resolving to empty clears url_path and re-syncs canonical back to the slug
func (s *core) regenerateProductURLPath(ctx context.Context, productID uuid.UUID, strict bool) error {
	product, err := s.repo.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return err
	}
	oldURL := product.URL
	// Products are created in the default locale, so the first stored content is a sensible {name} source
	urlName := product.SKU
	if len(product.AllContent) > 0 {
		urlName = product.AllContent[0].Name
	}
	// Fail-safe so one broken attribute can't leave a product without a URL, or abort a bulk regeneration across the whole product type
	safeFallback := func() string {
		if strings.TrimSpace(oldURL) != "" {
			return oldURL
		}
		if name := slugifyScalar(urlName); name != "" {
			return name
		}
		return slugifyForSKU(product.SKU)
	}

	var desired string
	switch {
	case !product.URLAuto:
		// Manual mode - keep the explicit url, it's re-synced below
		desired = oldURL
	default:
		template := product.URLTemplateOverride
		if template == "" {
			pt, err := s.repo.GetProductTypeByID(ctx, product.ProductTypeID)
			if err != nil {
				return err
			}
			template = pt.URLTemplate
		}
		if template == "" {
			// Auto enabled but no template - name/slug seed
			desired = safeFallback()
			break
		}
		rendered, ok, rerr := s.renderProductTemplate(ctx, product, template, urlName, strict, s.urlFallbackEnabled(ctx))
		if rerr != nil {
			// Infrastructure error, or the template error itself when fallback is off - either way propagate
			return rerr
		}
		if ok {
			desired = rendered
		} else {
			desired = safeFallback() // template / data problem - degrade, don't abort
		}
	}
	if strings.TrimSpace(desired) == "" {
		desired = safeFallback()
	}

	finalPath, err := s.persistAndSyncWithRetry(ctx, productID, desired)
	if err != nil {
		return err
	}
	// The intended path was taken and picked up a uniqueness suffix - record it so an operator can resolve the clash
	if desired != "" && finalPath != desired {
		s.noteURLCollision(ctx, "product", product.SKU, productID, desired, finalPath)
	}
	// Leave a 301 so an auto URL move doesn't break existing or indexed links
	s.leaveRedirectIfMoved(ctx, productID, oldURL, finalPath)
	return nil
}

// With fallbackEnabled a template/data problem returns ("", false, nil) after logging so the caller degrades to a safe slug, otherwise it propagates
// Genuine infrastructure errors always come back as a non-nil error
func (s *core) renderProductTemplate(ctx context.Context, product Product, template, urlName string, strict, fallbackEnabled bool) (string, bool, error) {
	// Swallow a template/data failure when fallback is enabled, else surface it
	degrade := func(stage string, cause error) (string, bool, error) {
		if !fallbackEnabled {
			return "", false, cause
		}
		s.logURLFallback(ctx, product, stage, cause, strict)
		return "", false, nil
	}
	tokens, err := parseURLTemplate(template)
	if err != nil {
		return degrade("parse template", err)
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, product.ProductTypeID)
	if err != nil {
		return "", false, err
	}
	if err := validateURLTemplate(tokens, pt); err != nil {
		// e.g. the template references an attribute that was since unassigned or switched to multiselect
		return degrade("validate template", err)
	}
	primaryByID, err := s.repo.ListPrimaryCategoryByProductIDs(ctx, []uuid.UUID{product.ID})
	if err != nil {
		return "", false, err
	}
	attrBySlug := make(map[string]Attribute, len(pt.Attributes))
	for _, a := range pt.Attributes {
		attrBySlug[a.Attribute.Slug] = a.Attribute
	}
	rendered, err := renderURLTemplate(tokens, urlName, primaryByID[product.ID], product.AttributeSelections, product.Attributes, attrBySlug)
	if err != nil {
		return degrade("render template", err)
	}
	return rendered, true, nil
}

// Keeps old and indexed links resolving after an auto move - honors the on/off + 301/302 settings, best-effort
func (s *core) leaveRedirectIfMoved(ctx context.Context, productID uuid.UUID, oldURL, newURL string) {
	if oldURL == newURL {
		return
	}
	s.createRenameRedirect(ctx, oldURL, "product", productID)
}

// Operator-triggered regenerations warn so the issue gets noticed, create-time is expected and stays at debug
func (s *core) logURLFallback(ctx context.Context, product Product, stage string, cause error, strict bool) {
	args := []any{
		"op", "catalog.regenerateProductURLPath",
		"product_id", product.ID,
		"sku", product.SKU,
		"stage", stage,
		"err", cause,
	}
	if strict {
		applog.From(ctx).Warn("product url template fell back to a safe slug", args...)
	} else {
		applog.From(ctx).Debug("product url template fell back to a safe slug", args...)
	}
}

// Retries with -2, -3, … suffixes when another entity already owns the path, and returns the final one
func (s *core) persistAndSyncWithRetry(ctx context.Context, productID uuid.UUID, path string) (string, error) {
	if s.urlRewrites == nil {
		// No rewriter wired - just persist the desired path (tests + CLI)
		if err := s.repo.UpdateProductURL(ctx, productID, path); err != nil {
			return "", err
		}
		return path, nil
	}
	for attempt := range 100 {
		candidate := path
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", path, attempt+1)
		}
		if err := s.urlRewrites.SyncCanonical(ctx, "product", productID, candidate); err != nil {
			if errors.Is(err, urlrewrite.ErrPathConflict) {
				continue
			}
			return "", err
		}
		if err := s.repo.UpdateProductURL(ctx, productID, candidate); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("could not allocate a unique url path from %q after 100 attempts", path)
}
