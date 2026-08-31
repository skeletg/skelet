package admin

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/b2b"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// B2B_ENABLED is server-wide, and this is the first check in every b2b resolver
func (r *Resolver) requireB2B() error {
	if !r.B2BSvc.Enabled() {
		return b2b.ErrDisabled
	}
	return nil
}

// The company has to be loaded before the grant can be checked - its channel is the scope
func (r *Resolver) b2bCompanyTarget(ctx context.Context, id, perm string) (b2b.Company, error) {
	cid, err := parseID(id, "company")
	if err != nil {
		return b2b.Company{}, err
	}
	co, err := r.B2BSvc.GetCompany(ctx, cid)
	if err != nil {
		return b2b.Company{}, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, perm, co.ChannelID, nil); err != nil {
		return b2b.Company{}, err
	}
	return co, nil
}

// bypass = every channel, otherwise an empty id list means no access at all rather than no filter
// The grant check comes first because LoadPermittedChannels hands a caller with no claims Bypass, which the callers read as every channel
func (r *Resolver) b2bScope(ctx context.Context, perm string) (ids []uuid.UUID, bypass bool, err error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
		return nil, false, err
	}
	p, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, perm)
	if err != nil {
		return nil, false, err
	}
	if p.Bypass() {
		return nil, true, nil
	}
	return p.IDs(), false, nil
}

func toCompanyModel(c b2b.Company) *model.Company {
	return &model.Company{
		ID:               c.ID.String(),
		ChannelID:        c.ChannelID.String(),
		Name:             c.Name,
		Status:           c.Status,
		CreditLimitCents: int32(c.CreditLimitCents),
		CreditUsedCents:  int32(c.CreditUsedCents),
		PaymentTermsDays: c.PaymentTermsDays,
		CreatedAt:        c.CreatedAt,
	}
}

func (r *Resolver) companyModelWithGroup(ctx context.Context, c b2b.Company) *model.Company {
	m := toCompanyModel(c)
	if c.DefaultCustomerGroupID != nil {
		if slug, err := r.B2BSvc.CustomerGroupSlug(ctx, *c.DefaultCustomerGroupID); err == nil {
			m.DefaultCustomerGroupSlug = slug
		}
	}
	return m
}

func toB2BQuoteModel(q b2b.Quote) *model.B2BQuote {
	out := &model.B2BQuote{
		ID:         q.ID.String(),
		CustomerID: q.CustomerID.String(),
		ChannelID:  q.ChannelID.String(),
		Status:     q.Status,
		Note:       q.Note,
		SellerNote: q.SellerNote,
		TotalCents: int32(q.TotalCents),
		ValidUntil: q.ValidUntil,
		CreatedAt:  q.CreatedAt,
		Items:      make([]*model.B2BQuoteItem, 0, len(q.Items)),
	}
	if q.CompanyID != nil {
		id := q.CompanyID.String()
		out.CompanyID = &id
	}
	for _, it := range q.Items {
		out.Items = append(out.Items, &model.B2BQuoteItem{
			Sku:            it.SKU,
			Name:           it.Name,
			Quantity:       it.Quantity,
			UnitPriceCents: int32(it.UnitPriceCents),
			OptionsSummary: it.OptionsSummary,
		})
	}
	return out
}

func customerAddressToOrderAddress(a customer.Address) order.OrderAddress {
	return order.OrderAddress{
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     a.Company,
		Street1:     a.Street1,
		Street2:     a.Street2,
		City:        a.City,
		State:       a.State,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

func toB2BInvoiceModel(inv b2b.Invoice) *model.B2BInvoice {
	overdue := inv.Status == "open" && inv.DueAt != nil && inv.DueAt.Before(time.Now())
	return &model.B2BInvoice{
		ID:          inv.ID.String(),
		CompanyID:   inv.CompanyID.String(),
		OrderID:     inv.OrderID.String(),
		OrderNumber: inv.OrderNumber,
		Number:      inv.Number,
		AmountCents: int32(inv.AmountCents),
		Status:      inv.Status,
		Overdue:     overdue,
		DueAt:       inv.DueAt,
		PaidAt:      inv.PaidAt,
		CreatedAt:   inv.CreatedAt,
	}
}
