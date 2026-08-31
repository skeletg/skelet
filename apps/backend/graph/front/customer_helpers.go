package front

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	frontmodel "github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/b2b"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
)

func toCustomerModel(c customer.Customer) *model.Customer {
	addrs := make([]*model.Address, len(c.Addresses))
	for i, a := range c.Addresses {
		addrs[i] = toAddressModel(a)
	}
	m := &model.Customer{
		ID:              c.ID.String(),
		Email:           c.Email,
		FirstName:       c.FirstName,
		LastName:        c.LastName,
		Phone:           c.Phone,
		IsActive:        c.IsActive,
		IsEmailVerified: c.IsEmailVerified,
		Metadata:        customerMetadataItems(c.Metadata),
		CreatedAt:       c.CreatedAt,
		Addresses:       addrs,
	}
	// Group is non-null on the schema and every repo path should load it - a missing one is a bug, but panicking turned one bad row into a 500, so log loud and let gqlgen emit the field-level null error instead
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

func (r *mutationResolver) customerAuthPayload(ctx context.Context, c customer.Customer) (*model.CustomerAuthPayload, error) {
	refreshToken, _, err := r.CustomerRefreshStore.Create(ctx, c.ID, "", "")
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	return r.customerAuthPayloadWithRefresh(ctx, c, refreshToken)
}

// Takes an already-issued refresh token - login/register mint one via Create, the refresh path passes the rotated successor from Rotate straight through
func (r *mutationResolver) customerAuthPayloadWithRefresh(ctx context.Context, c customer.Customer, refreshToken string) (*model.CustomerAuthPayload, error) {
	accessToken, err := auth.GenerateCustomerToken(c.ID, c.ChannelID, c.Email, r.JWTSecret, r.JWTTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	cWithAddresses, err := r.CustomerSvc.GetByIDWithAddresses(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("load customer: %w", err)
	}
	return &model.CustomerAuthPayload{
		AccessToken:  &accessToken,
		RefreshToken: &refreshToken,
		Customer:     toCustomerModel(cWithAddresses),
	}, nil
}

// customerPendingVerificationPayload returns the created account with NO session - the caller registered while the channel requires a verified address, so tokens are withheld until they click the link
func (r *mutationResolver) customerPendingVerificationPayload(ctx context.Context, c customer.Customer) (*model.CustomerAuthPayload, error) {
	cWithAddresses, err := r.CustomerSvc.GetByIDWithAddresses(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("load customer: %w", err)
	}
	return &model.CustomerAuthPayload{
		Customer:             toCustomerModel(cWithAddresses),
		VerificationRequired: true,
	}, nil
}

func toMyQuoteModel(q b2b.Quote) *frontmodel.MyB2BQuote {
	out := &frontmodel.MyB2BQuote{
		ID:         q.ID.String(),
		Status:     q.Status,
		Note:       q.Note,
		SellerNote: q.SellerNote,
		TotalCents: int32(q.TotalCents),
		ValidUntil: q.ValidUntil,
		CreatedAt:  q.CreatedAt,
		Items:      make([]*frontmodel.MyB2BQuoteItem, 0, len(q.Items)),
	}
	for _, it := range q.Items {
		out.Items = append(out.Items, &frontmodel.MyB2BQuoteItem{
			Sku:            it.SKU,
			Name:           it.Name,
			Quantity:       it.Quantity,
			UnitPriceCents: int32(it.UnitPriceCents),
			OptionsSummary: it.OptionsSummary,
		})
	}
	return out
}

// Everything the controller demonstrably holds gets pulled in, not just profile + addresses + orders - omitting a category the admin UI shows one section below the export button makes the subject-access response incomplete
// Each source is best-effort, so one unavailable service degrades that section rather than failing the whole export
func (r *Resolver) enrichDataExport(ctx context.Context, customerID uuid.UUID, currency string, bundle *customer.DataExport) {
	if r.ConsentSvc != nil {
		if list, err := r.ConsentSvc.ListForCustomer(ctx, customerID); err == nil {
			if raw, jerr := json.Marshal(list); jerr == nil {
				bundle.Consents = raw
			}
		}
	}
	extra := map[string]any{}
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
	if r.WishlistSvc != nil {
		if ids, err := r.WishlistSvc.ListIDs(ctx, customerID); err == nil && len(ids) > 0 {
			extra["wishlistProductIds"] = ids
		}
	}
	if len(extra) > 0 {
		bundle.Extra = extra
	}
}
