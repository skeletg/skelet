package customer

import "github.com/google/uuid"

// Accessors for sdk.CustomerRegistering, pinned in internal/plugin - Register reads back the tags (into customer.registered),
// the group and the marketing consent

func (c *CustomerRegistering) RegisteringChannelID() uuid.UUID       { return c.ChannelID }
func (c *CustomerRegistering) RegisteringEmail() string              { return c.Email }
func (c *CustomerRegistering) RegisteringFirstName() string          { return c.FirstName }
func (c *CustomerRegistering) RegisteringLastName() string           { return c.LastName }
func (c *CustomerRegistering) RegisteringTags() []string             { return append([]string(nil), c.Tags...) }
func (c *CustomerRegistering) RegisteringGroupID() *uuid.UUID        { return c.GroupID }
func (c *CustomerRegistering) RegisteringMarketingConsent() *bool    { return c.MarketingConsent }
func (c *CustomerRegistering) AddRegisteringTags(tags ...string)     { c.Tags = append(c.Tags, tags...) }
func (c *CustomerRegistering) SetRegisteringGroup(groupID uuid.UUID) { c.GroupID = &groupID }
func (c *CustomerRegistering) SetRegisteringMarketingConsent(optedIn bool) {
	c.MarketingConsent = &optedIn
}

// sdk.CustomerDeleting - refuse-only, so no setters
func (c *CustomerDeleting) DeletingCustomerID() uuid.UUID { return c.CustomerID }
func (c *CustomerDeleting) DeletingChannelID() uuid.UUID  { return c.ChannelID }
func (c *CustomerDeleting) DeletingEmail() string         { return c.Email }
