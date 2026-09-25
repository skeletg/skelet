package customer

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Use these constants when subscribing, to avoid typos
const (
	EventRegistering   = "customer.registering"
	EventRegistered    = "customer.registered"
	EventUpdating      = "customer.updating"
	EventUpdated       = "customer.updated"
	EventDeleting      = "customer.deleting"
	EventDeleted       = "customer.deleted"
	EventEmailVerified = "customer.email_verified"
	// Storefront self-service changes, async-only - surfaced on the Synapse activity timeline, and distinct from the admin-driven customer.updated above
	EventProfileUpdated  = "customer.profile_updated"
	EventPasswordChanged = "customer.password_changed"
	EventAddressChanged  = "customer.address_changed"
)

// Fires BEFORE a self-registration is persisted - sync handlers abort by returning an error or enrich by mutating exported fields, which the emitting service reads after the chain
// Only from Register, never AdminCreateCustomer - admin-created customers are vouched for and bypass the anti-fraud hooks
type CustomerRegistering struct {
	ChannelID uuid.UUID
	Email     string
	FirstName string
	LastName  string
	// A free-form list enrichers append to, e.g. a risk scorer stamping "high-risk" so the post-commit hook can escalate
	Tags []string
	// Override seam - non-nil picks that customer group instead of the default, used by auto-segmentation plugins routing corporate-domain emails into B2B
	GroupID *uuid.UUID
	// nil uses the legal default for the customer's locale, GDPR off-by-default in the EU and on-by-default in the US - non-nil means a plugin decided
	MarketingConsent *bool
}

func (*CustomerRegistering) Type() string { return EventRegistering }

func (c *CustomerRegistering) EventChannelID() uuid.UUID { return c.ChannelID }

// A sync webhook on customer.registering may respond {"reject": bool, "reason": string, "tags": [string]}
// An empty body is accept-with-no-changes; malformed JSON returns an error and aborts the registration
func (c *CustomerRegistering) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool     `json:"reject"`
		Reason string   `json:"reason"`
		Tags   []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if len(resp.Tags) > 0 {
		c.Tags = append(c.Tags, resp.Tags...)
	}
	return false, "", nil
}

// Fires AFTER the customer row commits - async, so it can't abort
type CustomerRegistered struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
	FirstName  string
	LastName   string
	Source     string
	Tags       []string
}

func (*CustomerRegistered) Type() string { return EventRegistered }

func (c *CustomerRegistered) EventChannelID() uuid.UUID { return c.ChannelID }

// Fires BEFORE an admin update is persisted - handlers may abort or append to Tags, and ChangedFields lets them skip updates they don't care about
type CustomerUpdating struct {
	CustomerID    uuid.UUID
	ChannelID     uuid.UUID
	ChangedFields []string
	Tags          []string
}

func (*CustomerUpdating) Type() string { return EventUpdating }

func (c *CustomerUpdating) EventChannelID() uuid.UUID { return c.ChannelID }

// Response schema {"reject": bool, "reason": string, "tags": [string]}
func (c *CustomerUpdating) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool     `json:"reject"`
		Reason string   `json:"reason"`
		Tags   []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if len(resp.Tags) > 0 {
		c.Tags = append(c.Tags, resp.Tags...)
	}
	return false, "", nil
}

type CustomerUpdated struct {
	CustomerID    uuid.UUID
	ChannelID     uuid.UUID
	Email         string
	FirstName     string
	LastName      string
	IsActive      bool
	ChangedFields []string
	Source        string
	Tags          []string
}

const SourceGDPR = "gdpr"

func (*CustomerUpdated) Type() string { return EventUpdated }

func (c *CustomerUpdated) EventChannelID() uuid.UUID { return c.ChannelID }

// Fires BEFORE the admin delete commits - handlers may abort, say an external CRM with open tickets - Response {"reject": bool, "reason": string}
type CustomerDeleting struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
}

func (*CustomerDeleting) Type() string                { return EventDeleting }
func (c *CustomerDeleting) EventChannelID() uuid.UUID { return c.ChannelID }

func (c *CustomerDeleting) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool   `json:"reject"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	return false, "", nil
}

type CustomerDeleted struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
}

func (*CustomerDeleted) Type() string                { return EventDeleted }
func (c *CustomerDeleted) EventChannelID() uuid.UUID { return c.ChannelID }

type CustomerEmailVerified struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
}

func (*CustomerEmailVerified) Type() string                { return EventEmailVerified }
func (c *CustomerEmailVerified) EventChannelID() uuid.UUID { return c.ChannelID }

// Storefront self-service, async-only - ChangedFields lists the fields that actually changed
type CustomerProfileUpdated struct {
	CustomerID    uuid.UUID
	ChannelID     uuid.UUID
	ChangedFields []string
}

func (*CustomerProfileUpdated) Type() string                { return EventProfileUpdated }
func (c *CustomerProfileUpdated) EventChannelID() uuid.UUID { return c.ChannelID }

// A logged-in customer changing their own password, distinct from the reset-token flow in auth - async-only
type CustomerPasswordChanged struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
}

func (*CustomerPasswordChanged) Type() string                { return EventPasswordChanged }
func (c *CustomerPasswordChanged) EventChannelID() uuid.UUID { return c.ChannelID }

// Action is "add" | "update" | "delete" - async-only
type CustomerAddressChanged struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	AddressID  uuid.UUID
	Action     string
}

func (*CustomerAddressChanged) Type() string                { return EventAddressChanged }
func (c *CustomerAddressChanged) EventChannelID() uuid.UUID { return c.ChannelID }
