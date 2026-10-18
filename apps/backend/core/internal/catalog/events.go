package catalog

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
)

const (
	EventProductCreating       = "product.creating"
	EventProductCreated        = "product.created"
	EventProductUpdating       = "product.updating"
	EventProductUpdated        = "product.updated"
	EventProductDeleted        = "product.deleted"
	EventProductListingChanged = "product.listing.changed"

	EventCategoryCreated = "category.created"
	EventCategoryUpdated = "category.updated"
	EventCategoryDeleted = "category.deleted"
	EventCategoryMoved   = "category.moved"

	EventCollectionCreated         = "collection.created"
	EventCollectionUpdated         = "collection.updated"
	EventCollectionDeleted         = "collection.deleted"
	EventCollectionProductsChanged = "collection.products.changed"

	EventBundleCreated = "bundle.created"
	EventBundleUpdated = "bundle.updated"
	EventBundleDeleted = "bundle.deleted"

	EventAttributeUpdated = "attribute.updated"
	EventAttributeDeleted = "attribute.deleted"

	EventProductTypeURLTemplateChanged = "product_type.url_template.changed"
	EventAttributeValueSlugChanged     = "attribute.value.slug.changed"
)

// Fires BEFORE the insert - sync handlers may abort (PIM duplicate-SKU check, naming policy) or append tags that get merged into the product's metadata, response schema {"reject": bool, "reason": string, "tags": [string]}
type ProductCreating struct {
	ProductTypeID uuid.UUID
	SKU           string
	Tags          []string
}

func (*ProductCreating) Type() string { return EventProductCreating }

func (e *ProductCreating) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		syncveto.Response
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if len(resp.Tags) > 0 {
		e.Tags = append(e.Tags, resp.Tags...)
	}
	return false, "", nil
}

// Fires BEFORE the update commits - ChangedFields lists the input keys the admin touched so listeners can skip irrelevant updates, and sync handlers may abort or append tags
type ProductUpdating struct {
	ProductID     uuid.UUID
	ProductTypeID uuid.UUID
	SKU           string
	ChangedFields []string
	Tags          []string
}

func (*ProductUpdating) Type() string { return EventProductUpdating }

func (e *ProductUpdating) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		syncveto.Response
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if len(resp.Tags) > 0 {
		e.Tags = append(e.Tags, resp.Tags...)
	}
	return false, "", nil
}

type ProductCreated struct { // fires AFTER the insert commits, async-only
	ProductID     uuid.UUID
	ProductTypeID uuid.UUID
	SKU           string
}

func (*ProductCreated) Type() string { return EventProductCreated }

// Fires AFTER the update commits, async-only - ChangedFields lists which top-level fields the admin touched so subscribers can skip irrelevant updates
type ProductUpdated struct {
	ProductID     uuid.UUID
	ProductTypeID uuid.UUID
	SKU           string
	ChangedFields []string
}

func (*ProductUpdated) Type() string { return EventProductUpdated }

// Fires AFTER the delete commits, async-only - subscribers must use the snapshot fields here, the row is gone by the time it fires
type ProductDeleted struct {
	ProductID     uuid.UUID
	ProductTypeID uuid.UUID
	SKU           string
}

func (*ProductDeleted) Type() string { return EventProductDeleted }

// Fires AFTER a per-channel listing upsert or delete, async-only - carries the pair, the new is_published and which kind of change it was so the search indexer doesn't have to read the row back
type ProductListingChanged struct {
	ProductID   uuid.UUID
	ChannelID   uuid.UUID
	IsPublished bool
	Cleared     bool // true when the listing row was deleted entirely
}

func (*ProductListingChanged) Type() string { return EventProductListingChanged }

func (e *ProductListingChanged) EventChannelID() uuid.UUID { return e.ChannelID }

type CategoryCreated struct { // this and CategoryUpdated/CategoryDeleted fire AFTER the mutation commits, async-only
	CategoryID uuid.UUID
	Slug       string
	ParentID   *uuid.UUID
}

func (*CategoryCreated) Type() string { return EventCategoryCreated }

type CategoryUpdated struct {
	CategoryID    uuid.UUID
	Slug          string
	ParentID      *uuid.UUID
	ChangedFields []string
}

func (*CategoryUpdated) Type() string { return EventCategoryUpdated }

type CategoryDeleted struct {
	CategoryID uuid.UUID
	Slug       string
}

func (*CategoryDeleted) Type() string { return EventCategoryDeleted }

// Enqueued in the same tx as a rename or re-parent, so the product URL regeneration under the subtree survives a crash after commit
type CategoryMoved struct {
	CategoryID uuid.UUID
}

func (*CategoryMoved) Type() string { return EventCategoryMoved }

type CollectionCreated struct { // this and the other Collection* events fire AFTER the mutation commits, async-only
	CollectionID uuid.UUID
	Slug         string
}

func (*CollectionCreated) Type() string { return EventCollectionCreated }

type CollectionUpdated struct {
	CollectionID  uuid.UUID
	Slug          string
	ChangedFields []string
}

func (*CollectionUpdated) Type() string { return EventCollectionUpdated }

type CollectionDeleted struct {
	CollectionID uuid.UUID
	Slug         string
}

func (*CollectionDeleted) Type() string { return EventCollectionDeleted }

// Added and Removed always name the affected products, so a search indexer rebuilds exactly those docs instead of re-pulling the whole collection
type CollectionProductsChanged struct {
	CollectionID uuid.UUID
	Added        []uuid.UUID
	Removed      []uuid.UUID
}

func (*CollectionProductsChanged) Type() string { return EventCollectionProductsChanged }

type BundleCreated struct { // this and BundleUpdated/BundleDeleted fire AFTER the bundle (configurable parent + options/selections) mutates, async-only
	ProductID uuid.UUID
	SKU       string
}

func (*BundleCreated) Type() string { return EventBundleCreated }

type BundleUpdated struct {
	ProductID     uuid.UUID
	SKU           string
	ChangedFields []string
}

func (*BundleUpdated) Type() string { return EventBundleUpdated }

type BundleDeleted struct {
	ProductID uuid.UUID
	SKU       string
}

func (*BundleDeleted) Type() string { return EventBundleDeleted }

// Fires AFTER an attribute or its values, assignments or flags change - ProductIDs stays off the webhook payload, which can't carry a catalogue-sized list
type AttributeUpdated struct {
	AttributeID   uuid.UUID
	Slug          string
	ChangedFields []string
	ProductIDs    []uuid.UUID `json:"-"`
}

func (*AttributeUpdated) Type() string { return EventAttributeUpdated }

type AttributeDeleted struct {
	AttributeID uuid.UUID
	Slug        string
	ProductIDs  []uuid.UUID `json:"-"`
}

func (*AttributeDeleted) Type() string { return EventAttributeDeleted }

// Enqueued in the same tx as the template change, so the bulk product URL recompute survives a crash after commit
type ProductTypeURLTemplateChanged struct {
	ProductTypeID uuid.UUID
}

func (*ProductTypeURLTemplateChanged) Type() string { return EventProductTypeURLTemplateChanged }

type AttributeValueSlugChanged struct {
	AttributeID uuid.UUID
	ValueID     uuid.UUID
}

func (*AttributeValueSlugChanged) Type() string { return EventAttributeValueSlugChanged }
