package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Unknown customer and forbidden both answer FORBIDDEN - saying which would leak the existence of a row the caller can't see
func (r *Resolver) requireCustomerManage(ctx context.Context, customerID string) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.manage"); err != nil {
		return err
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return errInvalidID("customer id", nil)
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return auth.ErrForbidden
	}
	return auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.manage", c.ChannelID, nil)
}

func toCustomerModel(c customer.Customer) *model.Customer {
	addrs := make([]*model.Address, len(c.Addresses))
	for i, a := range c.Addresses {
		addrs[i] = toAddressModel(a)
	}
	m := &model.Customer{
		ID:               c.ID.String(),
		ChannelID:        c.ChannelID.String(),
		Email:            c.Email,
		FirstName:        c.FirstName,
		LastName:         c.LastName,
		Phone:            c.Phone,
		IsActive:         c.IsActive,
		IsEmailVerified:  c.IsEmailVerified,
		Note:             c.Note,
		Metadata:         customerMetadataItems(c.Metadata),
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
		Addresses:        addrs,
		TaxExempt:        c.TaxExempt,
		TaxExemptionNote: c.TaxExemptionNote,
		VatID:            c.VatID,
		VatIDStatus:      c.VatIDStatus,
		VatIDCheckedAt:   c.VatIDCheckedAt,
	}
	// Group is non-null on the schema so a missing one is a bug, but panicking turned one bad row into a 500 - log loud and let gqlgen emit the field-level null error instead
	if c.Group != nil {
		m.Group = toCustomerGroupModel(*c.Group)
	} else {
		slog.Error("customer returned without Group loaded - check repo loading path", "customer", c.ID)
	}
	if c.DefaultBillingAddressID != nil {
		for i := range c.Addresses {
			if c.Addresses[i].ID == *c.DefaultBillingAddressID {
				m.DefaultBillingAddress = addrs[i]
				break
			}
		}
	}
	if c.DefaultShippingAddressID != nil {
		for i := range c.Addresses {
			if c.Addresses[i].ID == *c.DefaultShippingAddressID {
				m.DefaultShippingAddress = addrs[i]
				break
			}
		}
	}
	return m
}

// nil input means no change, empty slice means clear
func customerMetadataFromInput(in []*model.MetadataInput) *customer.Metadata {
	if in == nil {
		return nil
	}
	m := make(customer.Metadata, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		m[item.Key] = item.Value
	}
	return &m
}

// SYNC: mirrors the storefront's DSAR assembly in graph/front - an admin-served bundle must carry the same categories the customer could pull themselves
// Consents fail the export rather than drop out of it - a DSAR answered without them hides the record the subject is most entitled to see
func (r *Resolver) enrichDataExport(ctx context.Context, customerID uuid.UUID, currency string, bundle *customer.DataExport) error {
	extra := map[string]any{}
	if r.ConsentSvc != nil {
		list, err := r.ConsentSvc.ListForCustomer(ctx, customerID)
		if err != nil {
			return fmt.Errorf("export consents: %w", err)
		}
		if bundle.Consents, err = json.Marshal(list); err != nil {
			return fmt.Errorf("export consents: %w", err)
		}
		history, err := r.ConsentSvc.History(ctx, customerID, 0)
		if err != nil {
			return fmt.Errorf("export consent history: %w", err)
		}
		if len(history) > 0 {
			extra["consentHistory"] = history
		}
	}
	if r.StoreCreditSvc != nil {
		if entries, err := r.StoreCreditSvc.Entries(ctx, customerID, 1000); err == nil && len(entries) > 0 {
			extra["storeCredit"] = entries
		}
		if currency != "" {
			if bal, err := r.StoreCreditSvc.Balance(ctx, customerID, currency); err == nil {
				extra["storeCreditBalance"] = map[string]any{"amountCents": bal, "currency": currency}
			}
		}
	}
	if r.LoyaltySvc != nil {
		if entries, err := r.LoyaltySvc.Entries(ctx, customerID, 1000); err == nil && len(entries) > 0 {
			extra["loyaltyPoints"] = entries
		}
	}
	if len(extra) > 0 {
		bundle.Extra = extra
	}
	return nil
}

func actingAdminID(ctx context.Context) *uuid.UUID {
	if claims, ok := auth.AdminFromContext(ctx); ok && claims.AdminID != uuid.Nil {
		id := claims.AdminID
		return &id
	}
	return nil
}

func toCustomerConsentEventModel(e consent.Event) *model.CustomerConsentEvent {
	m := &model.CustomerConsentEvent{
		ID: e.ID.String(), Kind: e.Kind, OptedIn: e.OptedIn, Source: e.Source, CreatedAt: e.CreatedAt,
	}
	if e.AdminID != nil {
		id := e.AdminID.String()
		m.AdminID = &id
	}
	return m
}
