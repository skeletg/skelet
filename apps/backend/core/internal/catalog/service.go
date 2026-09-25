package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// ChannelLookup is the slim view of channel.Service the catalog package needs - Defined locally so catalog → channel stays a one-way dependency and tests can stub it
type ChannelLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// LocaleValidator is the slim view of locale.Service the catalog needs to pre-check that a translation locale code exists - Defined locally for the same reason
type LocaleValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

// DefaultLocaleProvider resolves the global default content locale (the `general/locale/default` config) - Used by content reads to fall back when the requested locale has no row, and by content writes to enforce the "default-locale row must exist while translations exist" invariant
type DefaultLocaleProvider interface {
	DefaultLocale(ctx context.Context) (string, error)
}

// InventoryEnsurer is the slim view of inventory.Service the catalog needs to lazily provision an inventory_items row when a product is created with tracking enabled (either via product type default or per-product override) - Defined locally so catalog → inventory stays one-way and tests can stub it
type InventoryEnsurer interface {
	EnsureItem(ctx context.Context, productID uuid.UUID, tracked bool) error
	// SyncProductTracking reconciles an existing product's inventory item with its effective tracking decision after the tracking override changes (UpdateProduct) - Unlike EnsureItem, passing tracked=false never creates a row - it only flips an existing one off
	SyncProductTracking(ctx context.Context, productID uuid.UUID, tracked bool) error
}

// core holds the shared dependencies + cross-cutting helpers (locale resolution, caches, URL templating, events) plus the attribute / product-type methods that every catalog sub-service builds on - Each sub-service embeds *core (one-directional - no back-reference to the facade), so the dependency graph is core ← sub-services ← Service, not a mutual-embedding cycle
type core struct {
	repo          Repository
	tx            *txrunner.Manager
	media         *objstore.Storage
	channelSvc    ChannelLookup
	locales       LocaleValidator
	defaultLocale DefaultLocaleProvider
	urlRewrites   UrlRewriteSyncer
	inventory     InventoryEnsurer
	// urlConfig supplies general/urls/* behavior flags - nil = built-in defaults
	urlConfig URLConfigReader
	// urlCollisions records when a product couldn't get its intended URL because the path was taken (a suffix was appended) - nil = log only
	urlCollisions URLCollisionRecorder
	bus           *events.Bus
	// categoryTreeCache stores the assembled per-(store,locale) category tree used by the storefront's header navigation - Reads dominate writes (every storefront page); admin publish/unpublish/edit/move drops the namespace via invalidateCategoryTreeCache - Nil disables caching
	categoryTreeCache *cache.Cache
	// channelPathCache memoizes per-channel pretty-URL → product-id resolution (listingService.ResolveProductIDForChannelPath - the storefront catch-all route) - dropped when a listing changes its override - Nil disables
	channelPathCache *cache.Cache
}

// SetChannelPathCache wires the Valkey read-through cache for channel pretty-URL resolution - Optional - nil disables caching
func (s *core) SetChannelPathCache(c *cache.Cache) {
	s.channelPathCache = c
}

// Service is the thin facade - It embeds *core (shared deps + attribute/type methods) and the per-domain services, so callers keep using catalogSvc.<Any method> via promotion - It carries no domain logic of its own
type Service struct {
	*core
	*productService
	*categoryService
	*collectionService
	*optionService
	*mediaService
	*bundleService
	*groupedService
	*downloadService
	*subscriptionTermsService
	*kitService
	*listingService
	// remoteRecs, when wired, overrides the native "also bought" list with a remote recommendation provider - Consulted by ListAlsoBought (which shadows the embedded listingService method) - See remote_recommendation.go
	remoteRecs RemoteRecommendationSource
}

// LinkLabelsByIDs returns id→display-name for a catalog entity kind (product/category/collection) in the given locale - Used by the CMS link picker to label references - Best-effort: missing ids/errors yield no entry (caller falls back to the path) - Unknown kinds return an empty map
func (s *Service) LinkLabelsByIDs(ctx context.Context, entityType string, ids []uuid.UUID, locale string) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string)
	switch entityType {
	case "product":
		if m, err := s.repo.ListProductContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Name
			}
		}
	case "category":
		if m, err := s.repo.ListCategoryContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Name
			}
		}
	case "collection":
		if m, err := s.repo.ListCollectionContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Name
			}
		}
	}
	return out
}

// Deps is the catalog service's dependency set, injected at construction - Media/Locales/DefaultLocale/URLRewrites/Bus/CategoryTreeCache are optional (nil disables the corresponding behavior) but are passed explicitly so the wiring is visible per call site rather than spread across setters - The inventory ensurer is the one exception - it's wired post-construction via SetInventoryEnsurer because inventory's product lookup depends on this service (a construction cycle that injection can't break)
type Deps struct {
	Repo              Repository
	Tx                *txrunner.Manager
	Media             *objstore.Storage
	Channel           ChannelLookup
	Locales           LocaleValidator
	DefaultLocale     DefaultLocaleProvider
	URLRewrites       UrlRewriteSyncer
	Bus               *events.Bus
	CategoryTreeCache *cache.Cache
}

func NewService(d Deps) *Service {
	c := &core{
		repo:              d.Repo,
		tx:                d.Tx,
		media:             d.Media,
		channelSvc:        d.Channel,
		locales:           d.Locales,
		defaultLocale:     d.DefaultLocale,
		urlRewrites:       d.URLRewrites,
		bus:               d.Bus,
		categoryTreeCache: d.CategoryTreeCache,
	}
	return &Service{
		core:                     c,
		productService:           &productService{core: c},
		categoryService:          &categoryService{core: c},
		collectionService:        &collectionService{core: c},
		optionService:            &optionService{core: c},
		mediaService:             &mediaService{core: c},
		bundleService:            &bundleService{core: c},
		groupedService:           &groupedService{core: c},
		downloadService:          &downloadService{core: c},
		subscriptionTermsService: &subscriptionTermsService{core: c},
		kitService:               &kitService{core: c},
		listingService:           &listingService{core: c},
	}
}

// SetInventoryEnsurer wires the inventory provisioning hook - Optional - when nil, products created with tracking enabled don't get an inventory_items row until an admin explicitly enables tracking later
func (s *core) SetInventoryEnsurer(e InventoryEnsurer) {
	s.inventory = e
}

// URLConfigReader supplies the operator-tunable URL-behavior flags (general/urls/*) - Optional - a nil reader falls back to the built-in defaults (redirects on, 301, template fallback on)
type URLConfigReader interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

// SetURLConfig wires the config reader used for URL-behavior settings
func (s *core) SetURLConfig(r URLConfigReader) { s.urlConfig = r }

// URLCollisionRecorder persists a durable, operator-reviewable record when a product's intended URL was already taken and a uniqueness suffix had to be used instead - Wired to the audit log in main
type URLCollisionRecorder interface {
	RecordURLCollision(ctx context.Context, entityType, sku string, entityID uuid.UUID, requested, assigned string)
}

// SetURLCollisionRecorder wires the collision recorder (optional; nil = log only)
func (s *core) SetURLCollisionRecorder(r URLCollisionRecorder) { s.urlCollisions = r }

// noteURLCollision reports that `requested` was taken so `assigned` (a suffixed path) was used - Always logs a warning - also writes a durable audit record when a recorder is wired, so an operator can find + fix it later
func (s *core) noteURLCollision(ctx context.Context, entityType, sku string, entityID uuid.UUID, requested, assigned string) {
	applog.From(ctx).Warn("url path taken; assigned a suffixed path instead",
		"entity_type", entityType, "sku", sku, "entity_id", entityID,
		"requested", requested, "assigned", assigned)
	if s.urlCollisions != nil {
		s.urlCollisions.RecordURLCollision(ctx, entityType, sku, entityID, requested, assigned)
	}
}

// boolSetting reads a general/urls bool, defaulting to `def` when unset, unreadable, or no reader is wired
func (s *core) boolSetting(ctx context.Context, path string, def bool) bool {
	if s.urlConfig == nil {
		return def
	}
	v, err := s.urlConfig.GetBool(ctx, path, config.DefaultScope)
	if err != nil {
		return def
	}
	return v
}

// autoRedirectEnabled reports whether a URL change should leave a redirect
func (s *core) autoRedirectEnabled(ctx context.Context) bool {
	return s.boolSetting(ctx, "general/urls/auto_redirect_on_change", true)
}

// urlFallbackEnabled reports whether an unrenderable template degrades to a safe slug (true) or errors (false)
func (s *core) urlFallbackEnabled(ctx context.Context) bool {
	return s.boolSetting(ctx, "general/urls/url_template_fallback", true)
}

// renameRedirectType resolves the configured redirect type for auto rename redirects (301 default, 302 if set)
func (s *core) renameRedirectType(ctx context.Context) int {
	if s.urlConfig == nil {
		return 301
	}
	if v, err := s.urlConfig.GetString(ctx, "general/urls/redirect_type", config.DefaultScope); err == nil && v == "302" {
		return 302
	}
	return 301
}

// createRenameRedirect leaves a redirect from an entity's old path to it after a URL change, honoring the auto-redirect on/off toggle + 301/302 type - Shared by product / category / collection URL moves - Best-effort
func (s *core) createRenameRedirect(ctx context.Context, oldPath, entityType string, entityID uuid.UUID) {
	if s.urlRewrites == nil || strings.TrimSpace(oldPath) == "" || !s.autoRedirectEnabled(ctx) {
		return
	}
	if err := s.urlRewrites.CreateRedirect(ctx, oldPath, entityType, entityID, s.renameRedirectType(ctx)); err != nil {
		applog.From(ctx).Error("leave redirect after url change",
			"entity_type", entityType, "entity_id", entityID, "from", oldPath, "err", err)
	}
}

func (s *core) invalidateCategoryTreeCache(ctx context.Context) {
	if s.categoryTreeCache == nil {
		return
	}
	s.categoryTreeCache.Flush(ctx)
}

// resolveDefaultLocale returns the configured global default locale code, or "" when no provider is wired or the lookup fails - Reads use this to fall back; writes use it to enforce the default-row-must-exist invariant
func (s *core) resolveDefaultLocale(ctx context.Context) string {
	if s.defaultLocale == nil {
		return ""
	}
	code, err := s.defaultLocale.DefaultLocale(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(code)
}

// ==================== ProductType ====================

func (s *core) CreateProductType(ctx context.Context, input CreateProductTypeInput) (ProductType, error) {
	if err := validateSlug(input.Slug); err != nil {
		return ProductType{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return ProductType{}, fmt.Errorf("name is required")
	}
	return s.repo.CreateProductType(ctx, input)
}

func (s *core) GetProductType(ctx context.Context, id uuid.UUID) (ProductType, error) {
	return s.repo.GetProductTypeWithAttributes(ctx, id)
}

// GetProductTypesByIDs is the batched lookup used by the storefront DataLoader - Attributes are NOT hydrated - that's the trade-off for one-query batching - Field resolvers that only need name/slug get what they want; resolvers that need the full attribute graph keep using GetProductType for individual ids
func (s *core) GetProductTypesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductType, error) {
	return s.repo.GetProductTypesByIDs(ctx, ids)
}

func (s *core) ListProductTypes(ctx context.Context) ([]ProductType, error) {
	return s.repo.ListProductTypes(ctx)
}

func (s *core) UpdateProductType(ctx context.Context, input UpdateProductTypeInput) (ProductType, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return ProductType{}, fmt.Errorf("name is required")
	}
	if input.URLTemplate != nil {
		if err := s.validateAndRecomputeForTemplateChange(ctx, input.ID, *input.URLTemplate); err != nil {
			return ProductType{}, err
		}
	}
	return s.repo.UpdateProductType(ctx, input)
}

// SetProductTypeURLPathTemplate is the focused entry-point for changing a product type's URL path template - Validates the template against the type's attributes, pre-flights every product the change would affect (auto + no override), persists the new template, and recomputes each product's url_path - Bulk failures abort with the first offending product, leaving the template unchanged so the admin can fix the data first
func (s *core) SetProductTypeURLPathTemplate(ctx context.Context, productTypeID uuid.UUID, template string) (ProductType, error) {
	if err := s.validateAndRecomputeForTemplateChange(ctx, productTypeID, template); err != nil {
		return ProductType{}, err
	}
	tpl := template
	updated, err := s.repo.UpdateProductType(ctx, UpdateProductTypeInput{
		ID:          productTypeID,
		URLTemplate: &tpl,
	})
	if err != nil {
		return ProductType{}, err
	}
	products, err := s.repo.ListProductsByType(ctx, productTypeID)
	if err != nil {
		return ProductType{}, err
	}
	for _, p := range products {
		if !p.URLAuto || p.URLTemplateOverride != "" {
			continue
		}
		if err := s.regenerateProductURLPath(ctx, p.ID, true); err != nil {
			return ProductType{}, fmt.Errorf("recompute %s: %w", p.SKU, err)
		}
	}
	return updated, nil
}

// validateAndRecomputeForTemplateChange validates a new URL template against a product type's assigned attributes: it must parse, and every {attr:…} token must name an assigned, non-multiselect attribute - It deliberately does NOT reject the change when individual products lack a value for a token - regeneration degrades those products to a safe slug (see regenerateProductURLPath), so one under-populated product can't block a template change for the entire product type
func (s *core) validateAndRecomputeForTemplateChange(ctx context.Context, productTypeID uuid.UUID, template string) error {
	if template == "" {
		// Clearing a template can only relax: products fall back to slug
		return nil
	}
	pt, err := s.repo.GetProductTypeWithAttributes(ctx, productTypeID)
	if err != nil {
		return err
	}
	tokens, err := parseURLTemplate(template)
	if err != nil {
		return err
	}
	return validateURLTemplate(tokens, pt)
}

func (s *core) DeleteProductType(ctx context.Context, id uuid.UUID) error { // the products FK is RESTRICT, so the repo turns a still-in-use type into ErrProductTypeInUse rather than a 500
	return s.repo.DeleteProductType(ctx, id)
}

// ==================== Attribute ====================

func (s *core) CreateAttribute(ctx context.Context, input CreateAttributeInput) (Attribute, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Attribute{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Attribute{}, fmt.Errorf("name is required")
	}
	if !input.InputType.IsValid() {
		return Attribute{}, fmt.Errorf("%w: %q", ErrInvalidInputType, input.InputType)
	}
	// Reference attributes must declare exactly which entity type they point at; non-reference attributes must not carry a target
	if input.InputType.IsReference() {
		if !input.RelationshipTarget.IsValid() {
			return Attribute{}, fmt.Errorf("%w: reference attributes require a valid relationshipTarget", ErrInvalidReferenceTarget)
		}
		if len(input.DefaultValue) > 0 {
			return Attribute{}, fmt.Errorf("%w: reference attributes take no default value", ErrInvalidDefaultValue)
		}
	} else if input.RelationshipTarget != "" {
		return Attribute{}, fmt.Errorf("%w: relationshipTarget is only valid for reference attributes", ErrInvalidReferenceTarget)
	}
	if len(input.Values) > 0 && !input.InputType.AcceptsValues() {
		return Attribute{}, fmt.Errorf("%w: input_type %q", ErrValuesNotApplicable, input.InputType)
	}
	for _, v := range input.Values {
		if err := validateSlug(v.Slug); err != nil {
			return Attribute{}, fmt.Errorf("value %q: %w", v.Slug, err)
		}
		if strings.TrimSpace(v.Name) == "" {
			return Attribute{}, fmt.Errorf("attribute value name is required")
		}
	}
	// Defaults at create time are scalar-only: selectable attributes reference AttributeValue IDs which don't exist until the rows are inserted, so set those via UpdateAttribute after the values exist
	if len(input.DefaultValue) > 0 {
		if input.InputType.AcceptsValues() {
			return Attribute{}, fmt.Errorf("%w: set defaults for selectable attributes via UpdateAttribute after creating values", ErrInvalidDefaultValue)
		}
		if err := validateScalarDefault(input.InputType, input.DefaultValue); err != nil {
			return Attribute{}, err
		}
	}
	// Required ⇒ default invariant - CreateAttribute can't satisfy this for selectables (no value IDs yet), so admins create them as non-required and toggle required + default together in the detail page's UpdateAttribute call
	if input.ValueRequired && len(input.DefaultValue) == 0 && !input.InputType.IsReference() {
		if input.InputType.AcceptsValues() {
			return Attribute{}, fmt.Errorf("%w: create the attribute first with valueRequired=false, then set both default and required together once values exist", ErrRequiredAttributeNeedsDefault)
		}
		return Attribute{}, fmt.Errorf("%w: scalar required attributes must include defaultValue in the same call", ErrRequiredAttributeNeedsDefault)
	}
	// Default H1Position to "before" - matches the storefront's default placement when an indexable filter is rendered - Reject any other non-empty value so callers can't pass arbitrary text
	if input.H1Position == "" {
		input.H1Position = H1PositionBefore
	}
	if !input.H1Position.IsValid() {
		return Attribute{}, fmt.Errorf("invalid h1Position %q", input.H1Position)
	}
	return s.repo.CreateAttribute(ctx, input)
}

func (s *core) GetAttribute(ctx context.Context, id uuid.UUID) (Attribute, error) {
	return s.repo.GetAttributeWithValues(ctx, id)
}

func (s *core) ListAttributes(ctx context.Context) ([]Attribute, error) {
	return s.repo.ListAttributes(ctx)
}

func (s *core) UpdateAttribute(ctx context.Context, input UpdateAttributeInput) (Attribute, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Attribute{}, fmt.Errorf("name is required")
	}
	if input.H1Position != nil && !input.H1Position.IsValid() {
		return Attribute{}, fmt.Errorf("invalid h1Position %q", *input.H1Position)
	}
	// Load current state once - We need it to validate selectable defaults (referenced value IDs must belong to this attribute) and to project the post-update final state for the required⇒default invariant
	var current *Attribute
	loadCurrent := func() (*Attribute, error) {
		if current != nil {
			return current, nil
		}
		c, err := s.repo.GetAttributeWithValues(ctx, input.ID)
		if err != nil {
			return nil, err
		}
		current = &c
		return current, nil
	}

	// When changing the default, type-check against the attribute's shape - Scalar defaults validate JSON shape; selectables verify every referenced value ID belongs to this attribute
	if input.DefaultValue != nil && len(*input.DefaultValue) > 0 {
		attr, err := loadCurrent()
		if err != nil {
			return Attribute{}, err
		}
		if attr.InputType.AcceptsValues() {
			if err := validateSelectableDefault(*attr, *input.DefaultValue); err != nil {
				return Attribute{}, err
			}
		} else {
			if err := validateScalarDefault(attr.InputType, *input.DefaultValue); err != nil {
				return Attribute{}, err
			}
		}
	}

	// Project the final state (current ⊕ this update) and enforce the required⇒default invariant - Catches every bad combination in one place: setting required without an existing/incoming default, clearing default while still required, etc.
	if input.ValueRequired != nil || input.DefaultValue != nil {
		attr, err := loadCurrent()
		if err != nil {
			return Attribute{}, err
		}
		finalRequired := attr.ValueRequired
		if input.ValueRequired != nil {
			finalRequired = *input.ValueRequired
		}
		hasFinalDefault := len(attr.DefaultValue) > 0
		if input.DefaultValue != nil {
			hasFinalDefault = len(*input.DefaultValue) > 0
		}
		if finalRequired && !hasFinalDefault {
			return Attribute{}, fmt.Errorf("%w: attribute %q would end up required without a default", ErrRequiredAttributeNeedsDefault, attr.Slug)
		}
	}
	return s.repo.UpdateAttribute(ctx, input)
}

func (s *core) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.CountProductsUsingConfigurableOption(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: used as a configurable option on %d product(s); remove it from configurable options first", ErrAttributeUsedAsConfigurableOption, n)
	}
	return s.repo.DeleteAttribute(ctx, id)
}

// ==================== AttributeValue ====================

// AddAttributeValue enforces that the parent attribute actually accepts values (dropdown/multiselect/swatch) - Otherwise we'd end up with orphan value rows that the admin UI wouldn't know what to do with
func (s *core) AddAttributeValue(ctx context.Context, attributeID uuid.UUID, v AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(v.Slug); err != nil {
		return AttributeValue{}, err
	}
	if strings.TrimSpace(v.Name) == "" {
		return AttributeValue{}, fmt.Errorf("name is required")
	}
	attr, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return AttributeValue{}, err
	}
	if !attr.InputType.AcceptsValues() {
		return AttributeValue{}, fmt.Errorf("%w: attribute %q has input_type %q", ErrValuesNotApplicable, attr.Slug, attr.InputType)
	}
	return s.repo.AddAttributeValue(ctx, attributeID, v)
}

func (s *core) UpdateAttributeValue(ctx context.Context, id uuid.UUID, v AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(v.Slug); err != nil {
		return AttributeValue{}, err
	}
	if strings.TrimSpace(v.Name) == "" {
		return AttributeValue{}, fmt.Errorf("name is required")
	}
	// Snapshot the pre-update state so a slug change can be detected and propagated to dependent product URLs - Errors here aren't fatal - fall back to "no cascade" rather than blocking the value rename, mirroring the category-slug branch posture
	prev, prevErr := s.repo.GetAttributeValueByID(ctx, id)
	updated, err := s.repo.UpdateAttributeValue(ctx, id, v)
	if err != nil {
		return AttributeValue{}, err
	}
	if prevErr == nil && prev.Slug != updated.Slug {
		if err := s.regenerateProductsForAttributeValue(ctx, prev.AttributeID, updated.ID); err != nil {
			applog.From(ctx).Error("regenerate products on attribute value rename",
				"op", "catalog.Service.UpdateAttributeValue",
				"attribute_id", prev.AttributeID,
				"value_id", updated.ID,
				"old_slug", prev.Slug,
				"new_slug", updated.Slug,
				"err", err)
		}
	}
	return updated, nil
}

// regenerateProductsForAttributeValue cascades a value's slug rename to every product whose templated URL depends on it - The repo query already filters to products of types whose template references the attribute (via the `{attr:<slug>}` LIKE pattern below), so the loop is tight - no per-product template parse needed here
func (s *core) regenerateProductsForAttributeValue(ctx context.Context, attributeID, valueID uuid.UUID) error {
	// Resolve the attribute's slug so we can build the token pattern - A value belongs to exactly one attribute - the GetAttributeByID lookup is one row
	attr, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return fmt.Errorf("get attribute %s: %w", attributeID, err)
	}
	tokenPattern := "%{attr:" + attr.Slug + "}%"
	productIDs, err := s.repo.ListProductIDsByAttributeValue(ctx, valueID, tokenPattern)
	if err != nil {
		return fmt.Errorf("list products by attribute value %s: %w", valueID, err)
	}
	for _, pid := range productIDs {
		if err := s.regenerateProductURLPath(ctx, pid, true); err != nil {
			return fmt.Errorf("regenerate product %s: %w", pid, err)
		}
	}
	return nil
}

func (s *core) DeleteAttributeValue(ctx context.Context, id uuid.UUID) error {
	// A value referenced as the attribute's default would leave a dangling pointer once removed - Only one attribute can own a value, so checking just that one attribute's default_value is sufficient
	v, err := s.repo.GetAttributeValueByID(ctx, id)
	if err != nil {
		return err
	}
	attr, err := s.repo.GetAttributeByID(ctx, v.AttributeID)
	if err != nil {
		return err
	}
	if defaultReferencesValue(attr.DefaultValue, id) {
		return fmt.Errorf("%w: value %q on attribute %q", ErrAttributeValueIsDefault, v.Slug, attr.Slug)
	}
	return s.repo.DeleteAttributeValue(ctx, id)
}

// ==================== AttributeContent / AttributeValueContent ====================

// ListAttributeContent returns every translated description row for one attribute, in locale order
func (s *core) ListAttributeContent(ctx context.Context, attributeID uuid.UUID) ([]AttributeContent, error) {
	out, err := s.repo.ListAttributeContent(ctx, attributeID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []AttributeContent{}, nil
	}
	return out, nil
}

// SetAttributeContent upserts the localized name + description for one (attribute, locale) pair - When both trimmed fields are empty the row is deleted - keeps the table sparse and matches "not translated" semantics on the storefront
func (s *core) SetAttributeContent(ctx context.Context, input UpsertAttributeContentInput) (AttributeContent, error) {
	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		return AttributeContent{}, fmt.Errorf("locale is required")
	}
	name := strings.TrimSpace(input.Name)
	desc := strings.TrimSpace(input.Description)
	if name == "" && desc == "" {
		if err := s.repo.DeleteAttributeContent(ctx, input.AttributeID, locale); err != nil {
			return AttributeContent{}, err
		}
		return AttributeContent{
			AttributeID: input.AttributeID,
			Locale:      locale,
		}, nil
	}
	// Validate only on the set path - clearing a row keyed by a bogus locale must stay possible
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return AttributeContent{}, err
		}
	}
	return s.repo.UpsertAttributeContent(ctx, UpsertAttributeContentInput{
		AttributeID: input.AttributeID,
		Locale:      locale,
		Name:        name,
		Description: desc,
	})
}

// ListAttributeContentByAttributeIDs batch-loads localised descriptions for many attributes at once - used by the front resolver to dress the browse response
func (s *core) ListAttributeContentByAttributeIDs(ctx context.Context, attributeIDs []uuid.UUID) ([]AttributeContent, error) {
	return s.repo.ListAttributeContentByAttributeIDs(ctx, attributeIDs)
}

// ListAttributeValueContentByValueIDs batch-loads localised descriptions for many AttributeValues at once
func (s *core) ListAttributeValueContentByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueContent, error) {
	return s.repo.ListAttributeValueContentByValueIDs(ctx, valueIDs)
}

// ListAttributeValueContent returns every translated description row for one AttributeValue
func (s *core) ListAttributeValueContent(ctx context.Context, valueID uuid.UUID) ([]AttributeValueContent, error) {
	out, err := s.repo.ListAttributeValueContent(ctx, valueID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []AttributeValueContent{}, nil
	}
	return out, nil
}

// SetAttributeValueContent upserts the localized name + description for one (value, locale) pair - Same both-empty-deletes-row contract as SetAttributeContent
func (s *core) SetAttributeValueContent(ctx context.Context, input UpsertAttributeValueContentInput) (AttributeValueContent, error) {
	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		return AttributeValueContent{}, fmt.Errorf("locale is required")
	}
	name := strings.TrimSpace(input.Name)
	desc := strings.TrimSpace(input.Description)
	if name == "" && desc == "" {
		if err := s.repo.DeleteAttributeValueContent(ctx, input.ValueID, locale); err != nil {
			return AttributeValueContent{}, err
		}
		return AttributeValueContent{
			ValueID: input.ValueID,
			Locale:  locale,
		}, nil
	}
	// Validate only on the set path - clearing a row keyed by a bogus locale must stay possible
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return AttributeValueContent{}, err
		}
	}
	return s.repo.UpsertAttributeValueContent(ctx, UpsertAttributeValueContentInput{
		ValueID:     input.ValueID,
		Locale:      locale,
		Name:        name,
		Description: desc,
	})
}

// CascadeAttributeIndexable flips the master indexable flag AND fans the value out to every AttributeValue / per-product slug override under the same attribute - Used by the admin's master switch so a single click can opt an entire facet in or out
func (s *core) CascadeAttributeIndexable(ctx context.Context, attributeID uuid.UUID, isIndexable bool) (CascadeAttributeIndexableResult, error) {
	return s.repo.CascadeAttributeIndexable(ctx, attributeID, isIndexable)
}

// SetAttributeValuesIndexable opts a hand-picked set of facet values in or out - selectable
// buckets read their flag live from Postgres at browse time, so this needs no reindex
func (s *core) SetAttributeValuesIndexable(ctx context.Context, valueIDs []uuid.UUID, isIndexable bool) (int64, error) {
	if len(valueIDs) == 0 {
		return 0, nil
	}
	return s.repo.SetAttributeValuesIndexable(ctx, valueIDs, isIndexable)
}

// ==================== ProductAttributeSlug ====================

// ListProductAttributeSlugs returns the per-attribute slug overrides for one product - Returns an empty slice (never nil) so admin callers can render an empty table without checking for nil
func (s *core) ListProductAttributeSlugs(ctx context.Context, productID uuid.UUID) ([]ProductAttributeSlug, error) {
	out, err := s.repo.ListProductAttributeSlugs(ctx, productID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []ProductAttributeSlug{}, nil
	}
	return out, nil
}

// ListProductAttributeSlugsByProductIDs batch-loads overrides for the indexer when rebuilding many products at once
func (s *core) ListProductAttributeSlugsByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeSlug, error) {
	return s.repo.ListProductAttributeSlugsByProductIDs(ctx, productIDs)
}

// SetProductTaxClass sets a product's tax class (empty → "standard") - The tax engine's product_tax_class condition matches on this slug
func (s *core) SetProductTaxClass(ctx context.Context, id uuid.UUID, taxClass string) (Product, error) {
	if taxClass == "" {
		taxClass = "standard"
	}
	return s.repo.SetProductTaxClass(ctx, id, taxClass)
}

// ProductTaxClassesBySKUs maps each SKU to its product's tax class, for per-line tax evaluation at order placement
func (s *core) ProductTaxClassesBySKUs(ctx context.Context, skus []string) (map[string]string, error) {
	return s.repo.ProductTaxClassesBySKUs(ctx, skus)
}

// ProductCostsBySKUs maps each SKU to its product's unit COGS (cost_cents), for the order-placement margin snapshot - SKUs with no cost are absent
func (s *core) ProductCostsBySKUs(ctx context.Context, skus []string) (map[string]int64, error) {
	return s.repo.ProductCostsBySKUs(ctx, skus)
}

// SetProductAttributeSlug pins a URL slug to one (product, scalar-attribute) pair - Rejects the call when the attribute is selectable - those manage slugs through AttributeValue, not per-product overrides - Facet indexability is attribute-level and is not set here
func (s *core) SetProductAttributeSlug(ctx context.Context, input UpsertProductAttributeSlugInput) (ProductAttributeSlug, error) {
	if err := validateSlug(input.Slug); err != nil {
		return ProductAttributeSlug{}, err
	}
	attr, err := s.repo.GetAttributeByID(ctx, input.AttributeID)
	if err != nil {
		return ProductAttributeSlug{}, err
	}
	if attr.InputType.AcceptsValues() {
		return ProductAttributeSlug{}, fmt.Errorf("selectable attributes manage slugs through AttributeValue, not per-product overrides")
	}
	return s.repo.UpsertProductAttributeSlug(ctx, input)
}

// DeleteProductAttributeSlug removes the override for one (product, attribute[, channel]) row. channelID nil clears the global row; non-nil clears that channel's row only - After deletion the value falls back to the global row (channel delete) or to slugify(value) + is_indexable=false (global delete)
func (s *core) DeleteProductAttributeSlug(ctx context.Context, productID, attributeID uuid.UUID, channelID *uuid.UUID) error {
	return s.repo.DeleteProductAttributeSlug(ctx, productID, attributeID, channelID)
}

// ==================== ProductAttributeLocaleValue ====================

// ListProductAttributeLocaleValues returns every plain_text translation row on one product, all locales
func (s *core) ListProductAttributeLocaleValues(ctx context.Context, productID uuid.UUID) ([]ProductAttributeLocaleValue, error) {
	out, err := s.repo.ListProductAttributeLocaleValues(ctx, productID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []ProductAttributeLocaleValue{}, nil
	}
	return out, nil
}

// ListProductAttributeLocaleValuesByProductIDs batch-loads translations for many products - indexer + storefront list rendering
func (s *core) ListProductAttributeLocaleValuesByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]ProductAttributeLocaleValue, error) {
	return s.repo.ListProductAttributeLocaleValuesByProductIDs(ctx, productIDs)
}

// SetProductAttributeLocaleValue upserts the translation of one plain_text scalar value for one locale - Empty trimmed value deletes the row - the locale falls back to the canonical JSONB value - Rejected for any attribute whose input type isn't plain_text: numeric/boolean/date values are locale-neutral facts, and selectable values translate through AttributeValueContent instead
func (s *core) SetProductAttributeLocaleValue(ctx context.Context, productID, attributeID uuid.UUID, locale, value string) (ProductAttributeLocaleValue, error) {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ProductAttributeLocaleValue{}, fmt.Errorf("locale is required")
	}
	attr, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return ProductAttributeLocaleValue{}, err
	}
	if attr.InputType != InputTypePlainText {
		return ProductAttributeLocaleValue{}, fmt.Errorf("attribute %q is %s: only plain_text scalar values are translatable", attr.Slug, attr.InputType)
	}
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return ProductAttributeLocaleValue{}, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		if err := s.repo.DeleteProductAttributeLocaleValue(ctx, productID, attributeID, locale); err != nil {
			return ProductAttributeLocaleValue{}, err
		}
		s.publishProductAttributeTranslationChanged(ctx, productID)
		return ProductAttributeLocaleValue{
			ProductID:   productID,
			AttributeID: attributeID,
			Locale:      locale,
		}, nil
	}
	// Validate only on the set path - clearing a row keyed by a bogus locale must stay possible
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return ProductAttributeLocaleValue{}, err
		}
	}
	out, err := s.repo.UpsertProductAttributeLocaleValue(ctx, ProductAttributeLocaleValue{
		ProductID:   productID,
		AttributeID: attributeID,
		Locale:      locale,
		Value:       value,
	})
	if err != nil {
		return ProductAttributeLocaleValue{}, err
	}
	s.publishProductAttributeTranslationChanged(ctx, productID)
	return out, nil
}

// publishProductAttributeTranslationChanged nudges the search indexer after a translation write - the per-locale docs embed the value, so they're stale until the product reindexes
func (s *core) publishProductAttributeTranslationChanged(ctx context.Context, productID uuid.UUID) {
	if s.bus == nil {
		return
	}
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return
	}
	s.bus.Publish(ctx, &ProductUpdated{
		ProductID:     p.ID,
		ProductTypeID: p.ProductTypeID,
		Slug:          p.SKU,
		SKU:           p.SKU,
		ChangedFields: []string{"attributes"},
	})
}

// ==================== AttributeValueChannelSlug ====================

// ListAttributeValueChannelSlugs returns every channel slug override for one selectable value
func (s *core) ListAttributeValueChannelSlugs(ctx context.Context, valueID uuid.UUID) ([]AttributeValueChannelSlug, error) {
	out, err := s.repo.ListAttributeValueChannelSlugsByValueIDs(ctx, []uuid.UUID{valueID})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []AttributeValueChannelSlug{}, nil
	}
	return out, nil
}

// ListAttributeValueChannelSlugsByValueIDs batch-loads channel slug overrides for many values at once - browse facet dressing
func (s *core) ListAttributeValueChannelSlugsByValueIDs(ctx context.Context, valueIDs []uuid.UUID) ([]AttributeValueChannelSlug, error) {
	return s.repo.ListAttributeValueChannelSlugsByValueIDs(ctx, valueIDs)
}

// ListAttributeValueChannelSlugsForChannel returns the full slug vocabulary override set of one channel - browse-time resolution
func (s *core) ListAttributeValueChannelSlugsForChannel(ctx context.Context, channelID uuid.UUID) ([]AttributeValueChannelSlug, error) {
	return s.repo.ListAttributeValueChannelSlugsForChannel(ctx, channelID)
}

// SetAttributeValueChannelSlug upserts one channel's filter-URL slug for a selectable AttributeValue - Empty slug deletes the override
func (s *core) SetAttributeValueChannelSlug(ctx context.Context, valueID, channelID uuid.UUID, slug string) (AttributeValueChannelSlug, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		if err := s.repo.DeleteAttributeValueChannelSlug(ctx, valueID, channelID); err != nil {
			return AttributeValueChannelSlug{}, err
		}
		return AttributeValueChannelSlug{ValueID: valueID, ChannelID: channelID}, nil
	}
	if err := validateSlug(slug); err != nil {
		return AttributeValueChannelSlug{}, err
	}
	val, err := s.repo.GetAttributeValueByID(ctx, valueID)
	if err != nil {
		return AttributeValueChannelSlug{}, err
	}
	return s.repo.UpsertAttributeValueChannelSlug(ctx, valueID, val.AttributeID, channelID, slug)
}

// DeleteAttributeValueChannelSlug clears one channel override - that channel falls back to the global AttributeValue.Slug
func (s *core) DeleteAttributeValueChannelSlug(ctx context.Context, valueID, channelID uuid.UUID) error {
	return s.repo.DeleteAttributeValueChannelSlug(ctx, valueID, channelID)
}

// ==================== Assignment ====================

func (s *core) AssignAttribute(ctx context.Context, input AssignAttributeInput) (ProductType, error) {
	if _, err := s.repo.GetProductTypeByID(ctx, input.ProductTypeID); err != nil {
		return ProductType{}, err
	}
	if _, err := s.repo.GetAttributeByID(ctx, input.AttributeID); err != nil {
		return ProductType{}, err
	}
	if err := s.repo.AssignAttribute(ctx, input); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, input.ProductTypeID)
}

func (s *core) UnassignAttribute(ctx context.Context, productTypeID, attributeID uuid.UUID) (ProductType, error) {
	n, err := s.repo.CountProductsUsingConfigurableOptionForType(ctx, attributeID, productTypeID)
	if err != nil {
		return ProductType{}, err
	}
	if n > 0 {
		return ProductType{}, fmt.Errorf("%w: used as a configurable option on %d product(s) of this type; remove it from configurable options first", ErrAttributeUsedAsConfigurableOption, n)
	}
	if err := s.repo.UnassignAttribute(ctx, productTypeID, attributeID); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, productTypeID)
}

func (s *core) CreateAttributeGroup(ctx context.Context, input CreateAttributeGroupInput) (ProductType, error) {
	if _, err := s.repo.GetProductTypeByID(ctx, input.ProductTypeID); err != nil {
		return ProductType{}, ErrProductTypeNotFound
	}
	if _, err := s.repo.CreateAttributeGroup(ctx, input); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, input.ProductTypeID)
}

func (s *core) UpdateAttributeGroup(ctx context.Context, input UpdateAttributeGroupInput) (ProductType, error) {
	group, err := s.repo.GetAttributeGroupByID(ctx, input.ID)
	if err != nil {
		return ProductType{}, ErrAttributeGroupNotFound
	}
	if _, err := s.repo.UpdateAttributeGroup(ctx, input); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, group.ProductTypeID)
}

func (s *core) DeleteAttributeGroup(ctx context.Context, id uuid.UUID) (ProductType, error) {
	group, err := s.repo.GetAttributeGroupByID(ctx, id)
	if err != nil {
		return ProductType{}, ErrAttributeGroupNotFound
	}
	if err := s.repo.DeleteAttributeGroup(ctx, id); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, group.ProductTypeID)
}

func (s *core) MoveAttributeToGroup(ctx context.Context, productTypeID, attributeID uuid.UUID, groupID *uuid.UUID) (ProductType, error) {
	if groupID != nil {
		if _, err := s.repo.GetAttributeGroupByID(ctx, *groupID); err != nil {
			return ProductType{}, ErrAttributeGroupNotFound
		}
	}
	if err := s.repo.MoveAttributeToGroup(ctx, productTypeID, attributeID, groupID); err != nil {
		return ProductType{}, err
	}
	return s.repo.GetProductTypeWithAttributes(ctx, productTypeID)
}

// ==================== helpers ====================

// validateSlug: lowercase letters, digits, underscores, and hyphens - Empty and whitespace-bearing slugs rejected - Matches the permission registry's convention so everything in the system uses the same rules
func validateSlug(slug string) error {
	return contentpkg.ValidateSlug(slug, ErrInvalidSlug)
}
