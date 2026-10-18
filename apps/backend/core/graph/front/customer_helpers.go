package front

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
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
	accessToken, err := auth.GenerateCustomerToken(c.ID, c.ChannelID, c.Email, r.CustomerKeys, r.JWTTTL)
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

// No session and no account - the one answer registration gives both when the channel wants a verified address first and when the address was already taken
func sessionWithheldPayload() *model.CustomerAuthPayload {
	return &model.CustomerAuthPayload{VerificationRequired: true}
}
