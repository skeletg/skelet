package admin

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Deliberately unauthenticated - a Customer object is only reachable through gated queries (myProfile on a customer JWT, customer/customers on customers.view), and lazy so list queries don't pay a consent query per row
func (r *customerResolver) Consents(ctx context.Context, obj *model.Customer) ([]*model.CustomerConsent, error) {
	customerID, err := parseID(obj.ID, "customer")
	if err != nil {
		return nil, err
	}
	list, err := r.ConsentSvc.ListForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CustomerConsent, len(list))
	for i, c := range list {
		out[i] = toCustomerConsentModel(c)
	}
	return out, nil
}

func (r *mutationResolver) SetCustomerTaxStatus(ctx context.Context, customerID string, taxExempt bool, note *string) (*model.Customer, error) {
	// Channel-scoped like every other per-customer mutation - a bare permission check let an admin scoped to one store zero out tax on another store's customer
	if err := r.requireCustomerManage(ctx, customerID); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(customerID)
	if err != nil {
		return nil, errInvalidID("customerId", err)
	}
	n := ""
	if note != nil {
		n = *note
	}
	if _, err := r.CustomerSvc.SetTaxStatus(ctx, id, taxExempt, n); err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.GetByIDWithAddresses(ctx, id)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

func (r *mutationResolver) SetCustomerVatID(ctx context.Context, customerID string, vatID string) (*model.Customer, error) {
	if err := r.requireCustomerManage(ctx, customerID); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(customerID)
	if err != nil {
		return nil, errInvalidID("customerId", err)
	}
	stored, status := "", tax.VatStatusUnverified
	if strings.TrimSpace(vatID) != "" {
		// A malformed id blocks the save, a VIES outage doesn't - that one stores as "unavailable" for a later re-check
		res, err := tax.ValidateVATID(ctx, vatID)
		if err != nil {
			return nil, apperr.New("VAT_ID_INVALID_FORMAT", apperr.ClassValidation, err.Error())
		}
		stored, status = res.VatID, res.Status
	}
	if _, err := r.CustomerSvc.SetVatID(ctx, id, stored, status); err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.GetByIDWithAddresses(ctx, id)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// GLOBAL customers.manage, not channel-scoped - groups carry no channel_id and drive group pricing on every store
func (r *mutationResolver) CreateCustomerGroup(ctx context.Context, input model.CreateCustomerGroupInput) (*model.CustomerGroup, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return nil, err
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	group, err := r.CustomerSvc.CreateGroup(ctx, customer.CreateCustomerGroupInput{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: description,
	})
	if err != nil {
		return nil, err
	}
	return toCustomerGroupModel(group), nil
}

// Nil fields mean no change, and the grant is global because the row is shared by every store
func (r *mutationResolver) UpdateCustomerGroup(ctx context.Context, id string, input model.UpdateCustomerGroupInput) (*model.CustomerGroup, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return nil, err
	}
	groupID, err := parseID(id, "group")
	if err != nil {
		return nil, err
	}
	group, err := r.CustomerSvc.UpdateGroup(ctx, groupID, customer.UpdateCustomerGroupInput{
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		return nil, err
	}
	return toCustomerGroupModel(group), nil
}

// Refuses while the group still has members or is the current default
func (r *mutationResolver) DeleteCustomerGroup(ctx context.Context, id string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return false, err
	}
	groupID, err := parseID(id, "group")
	if err != nil {
		return false, err
	}
	if err := r.CustomerSvc.DeleteGroup(ctx, groupID); err != nil {
		return false, err
	}
	return true, nil
}

// The most cross-tenant write in this surface - the default decides which group every new registration on every store lands in, so the grant has to be global
func (r *mutationResolver) SetDefaultCustomerGroup(ctx context.Context, id string) (*model.CustomerGroup, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return nil, err
	}
	groupID, err := parseID(id, "group")
	if err != nil {
		return nil, err
	}
	group, err := r.CustomerSvc.SetDefaultGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return toCustomerGroupModel(group), nil
}

func (r *mutationResolver) SetCustomerGroup(ctx context.Context, customerID string, groupID string) (*model.Customer, error) {
	if err := r.requireCustomerManage(ctx, customerID); err != nil {
		return nil, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return nil, err
	}
	gid, err := parseID(groupID, "group")
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.SetCustomerGroup(ctx, cid, gid)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// No verification email goes out - the admin supplies the password and vouches for the address with isEmailVerified
func (r *mutationResolver) AdminCreateCustomer(ctx context.Context, input model.AdminCreateCustomerInput) (*model.Customer, error) {
	storeID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.manage", storeID, nil); err != nil {
		return nil, err
	}
	domainInput := customer.AdminCreateCustomerInput{
		ChannelID:       storeID,
		Email:           input.Email,
		Password:        input.Password,
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Phone:           input.Phone,
		IsActive:        input.IsActive,
		IsEmailVerified: input.IsEmailVerified,
	}
	if input.CustomerGroupSlug != nil {
		domainInput.CustomerGroupSlug = *input.CustomerGroupSlug
	}
	c, err := r.CustomerSvc.AdminCreateCustomer(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// Changing the email does NOT auto-unverify it - pass isEmailVerified false in the same call when that's wanted
func (r *mutationResolver) AdminUpdateCustomer(ctx context.Context, id string, input model.AdminUpdateCustomerInput) (*model.Customer, error) {
	if err := r.requireCustomerManage(ctx, id); err != nil {
		return nil, err
	}
	customerID, err := parseID(id, "customer")
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.AdminUpdateCustomer(ctx, customerID, customer.AdminUpdateCustomerInput{
		Email:                    input.Email,
		FirstName:                input.FirstName,
		LastName:                 input.LastName,
		Phone:                    input.Phone,
		IsActive:                 input.IsActive,
		IsEmailVerified:          input.IsEmailVerified,
		Note:                     input.Note,
		CustomerGroupSlug:        input.CustomerGroupSlug,
		Metadata:                 customerMetadataFromInput(input.Metadata),
		DefaultBillingAddressID:  parseOptionalUUID(input.DefaultBillingAddressID),
		DefaultShippingAddressID: parseOptionalUUID(input.DefaultShippingAddressID),
	})
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

func (r *mutationResolver) AdminAddAddress(ctx context.Context, customerID string, input model.AddressInput) (*model.Address, error) {
	if err := r.requireCustomerManage(ctx, customerID); err != nil {
		return nil, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return nil, err
	}
	addr, err := r.CustomerSvc.AddAddress(ctx, cid, toAddressInput(input))
	if err != nil {
		return nil, err
	}
	return toAddressModel(addr), nil
}

// The address resolves to its owner first, so the channel check has a subject to scope against
func (r *mutationResolver) AdminDeleteAddress(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return false, err
	}
	addressID, err := parseID(id, "address")
	if err != nil {
		return false, err
	}
	addr, err := r.CustomerSvc.GetAddressByID(ctx, addressID)
	if err != nil {
		return false, auth.ErrForbidden
	}
	if err := r.requireCustomerManage(ctx, addr.CustomerID.String()); err != nil {
		return false, err
	}
	if err := r.CustomerSvc.AdminDeleteAddress(ctx, addressID); err != nil {
		return false, err
	}
	return true, nil
}

// A hard delete, refused once the customer has ever placed an order - GDPR anonymize is its own flow, and addresses plus carts cascade via FK
func (r *mutationResolver) DeleteCustomer(ctx context.Context, id string) (bool, error) {
	if err := r.requireCustomerManage(ctx, id); err != nil {
		return false, err
	}
	customerID, err := parseID(id, "customer")
	if err != nil {
		return false, err
	}
	if err := r.CustomerSvc.Delete(ctx, customerID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetCustomerActive(ctx context.Context, id string, active bool) (*model.Customer, error) {
	if err := r.requireCustomerManage(ctx, id); err != nil {
		return nil, err
	}
	customerID, err := parseID(id, "customer")
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.SetCustomerActive(ctx, customerID, active)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// Stored with source="admin" and the acting admin, so the audit trail can tell an override from a registration, profile or checkout action and name who made it
func (r *mutationResolver) AdminSetCustomerConsent(ctx context.Context, customerID string, kind string, optedIn bool) (*model.CustomerConsent, error) {
	if err := r.requireCustomerManage(ctx, customerID); err != nil {
		return nil, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return nil, err
	}
	stored, err := r.ConsentSvc.SetConsentByAdmin(ctx, cid, kind, optedIn, actingAdminID(ctx))
	if err != nil {
		return nil, err
	}
	return toCustomerConsentModel(stored), nil
}

func (r *mutationResolver) AnonymizeCustomer(ctx context.Context, customerID string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return false, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return false, err
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.manage", c.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.CustomerSvc.AnonymizeInPlace(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

// Signed: positive grants, negative deducts and is capped so the balance never goes below zero, in the customer's channel currency
func (r *mutationResolver) AdjustStoreCredit(ctx context.Context, customerID string, amountCents int32, reason string) (*model.StoreCreditSummary, error) {
	c, currency, err := r.storeCreditContext(ctx, customerID, "customers.manage")
	if err != nil {
		return nil, err
	}
	by, err := auth.RequireActor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := r.StoreCreditSvc.Adjust(ctx, c.ID, int64(amountCents), currency, strings.TrimSpace(reason), by); err != nil {
		return nil, err
	}
	return r.storeCreditSummary(ctx, c.ID, currency)
}

// Signed like AdjustStoreCredit, and a deduction is capped at the balance
func (r *mutationResolver) AdjustLoyaltyPoints(ctx context.Context, customerID string, points int32, reason string) (*model.LoyaltySummary, error) {
	c, _, err := r.storeCreditContext(ctx, customerID, "customers.manage")
	if err != nil {
		return nil, err
	}
	if _, err := r.LoyaltySvc.Adjust(ctx, c.ID, int64(points), strings.TrimSpace(reason)); err != nil {
		return nil, err
	}
	return r.loyaltySummary(ctx, c.ID)
}

// A short-lived storefront session so an operator sees the store as that customer does - a support
// and sales tool, not a B2B one, which is why it stays in core
func (r *mutationResolver) MasqueradeCustomer(ctx context.Context, customerID string) (*model.MasqueradeSession, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return nil, err
	}
	cust, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, errNotFound("customer")
	}
	// Scoped on the CUSTOMER's channel, not the caller's - this mints a real storefront session, so an
	// unscoped grant would let an operator on one store sign in as any customer on any other
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.manage", cust.ChannelID, nil); err != nil {
		return nil, err
	}
	token, err := auth.GenerateCustomerToken(cust.ID, cust.ChannelID, cust.Email, r.CustomerKeys, masqueradeSessionTTL)
	if err != nil {
		return nil, err
	}
	// Loud trail: masquerade hands an operator the customer's storefront powers, and the order
	// timeline is not enough on its own - anything they do arrives looking like the customer did it
	log.Printf("[masquerade] admin %s (%s) minted a session for customer %s", claims.AdminID, claims.Email, cust.ID)
	path := "/masquerade?token=" + token
	fullURL := ""
	if ch, chErr := r.ChannelSvc.GetByID(ctx, cust.ChannelID); chErr == nil && ch.PublicURL != "" {
		fullURL = strings.TrimRight(ch.PublicURL, "/") + path
	}
	return &model.MasqueradeSession{
		AccessToken:      token,
		StorefrontPath:   path,
		StorefrontURL:    fullURL,
		ExpiresInSeconds: int32(masqueradeSessionTTL.Seconds()),
	}, nil
}

// An explicit channelId can only narrow the role-granted scope, never widen it
func (r *queryResolver) Customers(ctx context.Context, channelID *string) ([]*model.Customer, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "customers.view")
	if err != nil {
		return nil, err
	}
	explicit, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	ids := perm.IDs()
	if explicit != nil {
		ids = []uuid.UUID{}
		if perm.Allows(*explicit) {
			ids = []uuid.UUID{*explicit}
		}
	}
	customers, err := r.CustomerSvc.ListInChannels(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Customer, 0, len(customers))
	for _, c := range customers {
		result = append(result, toCustomerModel(c))
	}
	return result, nil
}

func (r *queryResolver) CustomersByIds(ctx context.Context, ids []string) ([]*model.Customer, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "customers.view")
	if err != nil {
		return nil, err
	}
	want := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, errInvalidID("ids", err)
		}
		want = append(want, id)
	}
	byID, err := r.CustomerSvc.GetByIDs(ctx, want)
	if err != nil {
		return nil, err
	}
	found := make([]customer.Customer, 0, len(byID))
	for _, c := range byID {
		found = append(found, c)
	}
	// Same channel filter the list resolver applies - an id the caller cannot see resolves to nothing rather than an error
	visible := auth.FilterByChannel(found, perm, func(c customer.Customer) uuid.UUID { return c.ChannelID })
	out := make([]*model.Customer, 0, len(visible))
	for _, c := range visible {
		out = append(out, toCustomerModel(c))
	}
	return out, nil
}

func (r *queryResolver) CustomersPaged(ctx context.Context, channelID *string, search *string, groupSlug *string, isActive *bool, limit *int32, offset *int32) (*model.CustomerPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	// Scope resolves to a concrete id list BEFORE the SQL page, so LIMIT is applied to rows the caller may actually see
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "customers.view")
	if err != nil {
		return nil, err
	}
	explicit, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var channels []uuid.UUID
	switch {
	case explicit != nil:
		if !perm.Allows(*explicit) {
			return nil, auth.ErrForbidden
		}
		channels = []uuid.UUID{*explicit}
	case !perm.Bypass():
		channels = perm.IDs()
		if len(channels) == 0 {
			return &model.CustomerPage{TotalCount: 0, Items: []*model.Customer{}}, nil
		}
	}
	f := customer.ListPageFilter{
		ChannelIDs: channels,
		IsActive:   isActive,
		Limit:      intDeref(limit, 50),
		Offset:     intDeref(offset, 0),
	}
	if search != nil {
		f.Search = *search
	}
	if groupSlug != nil {
		f.GroupSlug = *groupSlug
	}
	rows, total, err := r.CustomerSvc.ListPage(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]*model.Customer, 0, len(rows))
	for _, c := range rows {
		items = append(items, toCustomerModel(c))
	}
	return &model.CustomerPage{TotalCount: int32(total), Items: items}, nil
}

// A scoped admin asking for a customer in a channel they can't see gets FORBIDDEN, not NOT_FOUND
func (r *queryResolver) Customer(ctx context.Context, id string) (*model.Customer, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	customerID, err := parseID(id, "customer")
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.GetByIDWithAddresses(ctx, customerID)
	if err != nil {
		return nil, errNotFound("customer")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.view", c.ChannelID, nil); err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

func (r *queryResolver) CustomerGroups(ctx context.Context) ([]*model.CustomerGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	groups, err := r.CustomerSvc.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.CustomerGroup, len(groups))
	for i, g := range groups {
		result[i] = toCustomerGroupModel(g)
	}
	return result, nil
}

func (r *queryResolver) CustomerGroup(ctx context.Context, id string) (*model.CustomerGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	groupID, err := parseID(id, "group")
	if err != nil {
		return nil, err
	}
	g, err := r.CustomerSvc.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return toCustomerGroupModel(g), nil
}

func (r *queryResolver) CustomerConsentHistory(ctx context.Context, customerID string, limit *int32) ([]*model.CustomerConsentEvent, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, auth.ErrForbidden
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.view", c.ChannelID, nil); err != nil {
		return nil, err
	}
	var lim int32
	if limit != nil {
		lim = *limit
	}
	events, err := r.ConsentSvc.History(ctx, cid, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CustomerConsentEvent, len(events))
	for i, e := range events {
		out[i] = toCustomerConsentEventModel(e)
	}
	return out, nil
}

func (r *queryResolver) ExportCustomerData(ctx context.Context, customerID string) (string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return "", err
	}
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return "", err
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return "", err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.manage", c.ChannelID, nil); err != nil {
		return "", err
	}
	bundle, err := r.DSAR.Build(ctx, cid)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (r *queryResolver) StoreCredit(ctx context.Context, customerID string) (*model.StoreCreditSummary, error) {
	c, currency, err := r.storeCreditContext(ctx, customerID, "customers.view")
	if err != nil {
		return nil, err
	}
	return r.storeCreditSummary(ctx, c.ID, currency)
}

func (r *queryResolver) Loyalty(ctx context.Context, customerID string) (*model.LoyaltySummary, error) {
	c, _, err := r.storeCreditContext(ctx, customerID, "customers.view")
	if err != nil {
		return nil, err
	}
	return r.loyaltySummary(ctx, c.ID)
}

func (r *Resolver) Customer() CustomerResolver { return &customerResolver{r} }

type customerResolver struct{ *Resolver }
