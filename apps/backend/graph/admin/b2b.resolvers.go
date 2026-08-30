package admin

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/b2b"
	"github.com/skeletg/skelet/internal/customer"
	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/ptr"
)

func (r *companyResolver) Prices(ctx context.Context, obj *model.Company) ([]*model.CompanyPrice, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	cid, err := parseID(obj.ID, "company")
	if err != nil {
		return nil, err
	}
	rows, err := r.B2BSvc.CompanyPrices(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CompanyPrice, len(rows))
	for i, cp := range rows {
		out[i] = &model.CompanyPrice{ID: cp.ID.String(), ProductID: cp.ProductID.String(), ProductSku: cp.ProductSKU, PriceCents: int32(cp.PriceCents)}
	}
	return out, nil
}

// CreateCompany opens a wholesale account on one channel (b2b.manage).
func (r *mutationResolver) CreateCompany(ctx context.Context, channelID string, name string, creditLimitCents int32, paymentTermsDays int32) (*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "b2b.manage", cid, nil); err != nil {
		return nil, err
	}
	co, err := r.B2BSvc.CreateCompany(ctx, cid, name, int64(creditLimitCents), paymentTermsDays)
	if err != nil {
		return nil, err
	}
	return r.companyModelWithGroup(ctx, co), nil
}

// UpdateCompany edits name/status/credit/terms (b2b.manage).
func (r *mutationResolver) UpdateCompany(ctx context.Context, id string, name string, status string, creditLimitCents int32, paymentTermsDays int32) (*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, id, "b2b.manage")
	if err != nil {
		return nil, err
	}
	updated, err := r.B2BSvc.UpdateCompany(ctx, co.ID, name, status, int64(creditLimitCents), paymentTermsDays)
	if err != nil {
		return nil, err
	}
	return r.companyModelWithGroup(ctx, updated), nil
}

// SetCompanyMember attaches a customer to the company (b2b.manage).
func (r *mutationResolver) SetCompanyMember(ctx context.Context, companyID string, customerID string, role string, orderLimitCents *int32) (bool, error) {
	if err := r.requireB2B(); err != nil {
		return false, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.manage")
	if err != nil {
		return false, err
	}
	custID, err := uuid.Parse(customerID)
	if err != nil {
		return false, errInvalidID("customer id", nil)
	}
	limit := int64(0)
	if orderLimitCents != nil {
		limit = int64(*orderLimitCents)
	}
	if err := r.B2BSvc.SetMember(ctx, co.ID, custID, role, limit); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveCompanyMember detaches a customer from any company (b2b.manage).
func (r *mutationResolver) RemoveCompanyMember(ctx context.Context, customerID string) (bool, error) {
	if err := r.requireB2B(); err != nil {
		return false, err
	}
	custID, err := uuid.Parse(customerID)
	if err != nil {
		return false, errInvalidID("customer id", nil)
	}
	// Scope on the company the member actually belongs to - the bare permission let an admin on one channel detach a buyer from a company on another, killing their contract pricing
	mem, err := r.B2BSvc.MembershipFor(ctx, custID)
	if err != nil {
		return false, err
	}
	if _, err := r.b2bCompanyTarget(ctx, mem.CompanyID.String(), "b2b.manage"); err != nil {
		return false, err
	}
	if err := r.B2BSvc.RemoveMember(ctx, custID); err != nil {
		return false, err
	}
	return true, nil
}

// ReleaseCompanyCredit returns headroom when an on-account invoice
// settles (b2b.manage).
func (r *mutationResolver) ReleaseCompanyCredit(ctx context.Context, companyID string, amountCents int32, reason *string) (*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.manage")
	if err != nil {
		return nil, err
	}
	if amountCents <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	var adminID *uuid.UUID
	if claims, aerr := auth.RequireAuth(ctx); aerr == nil {
		id := claims.AdminID
		adminID = &id
	}
	if err := r.B2BSvc.ReleaseCredit(ctx, co.ID, int64(amountCents), adminID, ptr.Deref(reason)); err != nil {
		return nil, err
	}
	updated, err := r.B2BSvc.GetCompany(ctx, co.ID)
	if err != nil {
		return nil, err
	}
	return r.companyModelWithGroup(ctx, updated), nil
}

func (r *mutationResolver) SettleB2bInvoice(ctx context.Context, id string, status string) (*model.B2BInvoice, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	invID, err := parseID(id, "invoice")
	if err != nil {
		return nil, err
	}
	inv, err := r.B2BSvc.GetInvoice(ctx, invID)
	if err != nil {
		return nil, err
	}
	// Channel-scope through the owning company, like every b2b mutation.
	if _, err := r.b2bCompanyTarget(ctx, inv.CompanyID.String(), "b2b.manage"); err != nil {
		return nil, err
	}
	var adminID *uuid.UUID
	if claims, aerr := auth.RequireAuth(ctx); aerr == nil {
		aid := claims.AdminID
		adminID = &aid
	}
	settled, err := r.B2BSvc.SettleInvoice(ctx, invID, status, adminID)
	if err != nil {
		return nil, err
	}
	// Hydrate the order number for the response (list reads join it).
	if o, oerr := r.OrderSvc.GetByID(ctx, settled.OrderID); oerr == nil {
		settled.OrderNumber = o.Number
	}
	return toB2BInvoiceModel(settled), nil
}

func (r *mutationResolver) SetCompanyPrice(ctx context.Context, companyID string, productID string, priceCents int32) (*model.CompanyPrice, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.manage")
	if err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	cp, err := r.B2BSvc.SetCompanyPrice(ctx, co.ID, pid, int64(priceCents))
	if err != nil {
		return nil, err
	}
	return &model.CompanyPrice{ID: cp.ID.String(), ProductID: cp.ProductID.String(), ProductSku: cp.ProductSKU, PriceCents: int32(cp.PriceCents)}, nil
}

func (r *mutationResolver) MasqueradeCustomer(ctx context.Context, customerID string) (*model.MasqueradeSession, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	claims, err := auth.RequireAuth(ctx)
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
	// Scope on the CUSTOMER's channel - masquerade mints a real storefront session, so an unscoped b2b.manage let a rep on one store sign in as any customer on any other
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "b2b.manage", cust.ChannelID, nil); err != nil {
		return nil, err
	}
	const ttl = 15 * time.Minute
	token, err := auth.GenerateCustomerToken(cust.ID, cust.ChannelID, cust.Email, r.JWTSecret, ttl)
	if err != nil {
		return nil, err
	}
	// Loud trail: masquerade grants a rep the customer's storefront
	// powers - the order timeline isn't enough, so log it explicitly.
	log.Printf("[b2b] admin %s (%s) minted masquerade session for customer %s (%s)", claims.AdminID, claims.Email, cust.ID, cust.Email)
	path := "/masquerade?token=" + token
	fullURL := ""
	if ch, chErr := r.ChannelSvc.GetByID(ctx, cust.ChannelID); chErr == nil && ch.PublicURL != "" {
		fullURL = strings.TrimRight(ch.PublicURL, "/") + path
	}
	return &model.MasqueradeSession{
		AccessToken:      token,
		StorefrontPath:   path,
		StorefrontURL:    fullURL,
		ExpiresInSeconds: int32(ttl.Seconds()),
	}, nil
}

func (r *mutationResolver) DeleteCompanyPrice(ctx context.Context, companyID string, priceID string) (bool, error) {
	if err := r.requireB2B(); err != nil {
		return false, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.manage")
	if err != nil {
		return false, err
	}
	id, err := parseID(priceID, "price")
	if err != nil {
		return false, err
	}
	// Keyed on the scoped company - the permission was checked against companyID, so the delete must be too or any company's price row could be removed through a company the caller does manage
	if err := r.B2BSvc.DeleteCompanyPrice(ctx, co.ID, id); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetCompanyCustomerGroup(ctx context.Context, companyID string, groupSlug string) (*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.manage")
	if err != nil {
		return nil, err
	}
	updated, err := r.B2BSvc.SetDefaultCustomerGroup(ctx, co.ID, groupSlug)
	if err != nil {
		return nil, err
	}
	return r.companyModelWithGroup(ctx, updated), nil
}

// PriceB2BQuote publishes priced lines back to the requester (b2b.manage).
func (r *mutationResolver) PriceB2BQuote(ctx context.Context, id string, items []*model.B2BQuoteItemInput, sellerNote *string, validUntil *time.Time) (*model.B2BQuote, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	qid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("quote id", nil)
	}
	q, err := r.B2BSvc.GetQuote(ctx, qid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "b2b.manage", q.ChannelID, nil); err != nil {
		return nil, err
	}
	lines := make([]b2b.QuoteItem, 0, len(items))
	for _, it := range items {
		summary := ""
		if it.OptionsSummary != nil {
			summary = *it.OptionsSummary
		}
		lines = append(lines, b2b.QuoteItem{
			SKU:            it.Sku,
			Name:           it.Name,
			Quantity:       it.Quantity,
			UnitPriceCents: int64(it.UnitPriceCents),
			OptionsSummary: summary,
		})
	}
	updated, err := r.B2BSvc.PriceQuote(ctx, qid, lines, ptr.Deref(sellerNote), validUntil)
	if err != nil {
		return nil, err
	}
	return toB2BQuoteModel(updated), nil
}

// CreateDraftOrderFromQuote turns an accepted quote into a draft order at the negotiated prices, using the customer's default addresses.
// The quote flips to 'converted'; the draft goes through the normal review + place flow (b2b.manage + orders draft permissions apply)
func (r *mutationResolver) CreateDraftOrderFromQuote(ctx context.Context, quoteID string) (*model.Order, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	qid, err := uuid.Parse(quoteID)
	if err != nil {
		return nil, errInvalidID("quote id", nil)
	}
	q, err := r.B2BSvc.GetQuote(ctx, qid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "b2b.manage", q.ChannelID, nil); err != nil {
		return nil, err
	}
	if q.Status != b2b.QuoteAccepted {
		return nil, fmt.Errorf("only accepted quotes can be converted (status is %q)", q.Status)
	}
	cust, err := r.CustomerSvc.GetByIDWithAddresses(ctx, q.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("load customer: %w", err)
	}
	findAddr := func(id *uuid.UUID) *customer.Address {
		if id == nil {
			return nil
		}
		for i := range cust.Addresses {
			if cust.Addresses[i].ID == *id {
				return &cust.Addresses[i]
			}
		}
		return nil
	}
	billing := findAddr(cust.DefaultBillingAddressID)
	shipping := findAddr(cust.DefaultShippingAddressID)
	if shipping == nil && len(cust.Addresses) > 0 {
		shipping = &cust.Addresses[0]
	}
	if billing == nil {
		billing = shipping
	}
	if billing == nil || shipping == nil {
		return nil, fmt.Errorf("the customer has no saved addresses - add one before converting")
	}
	lines := make([]order.CreateDraftLine, 0, len(q.Items))
	for _, it := range q.Items {
		price := it.UnitPriceCents
		lines = append(lines, order.CreateDraftLine{
			ProductSku:     it.SKU,
			Quantity:       it.Quantity,
			UnitPriceCents: &price,
		})
	}
	custID := q.CustomerID
	var noteB strings.Builder
	noteB.WriteString("Created from quote " + q.ID.String())
	for _, it := range q.Items {
		if it.OptionsSummary != "" {
			noteB.WriteString("\n" + it.SKU + ": " + it.OptionsSummary)
		}
	}
	note := noteB.String()
	draft, err := r.OrderSvc.CreateDraftOrder(ctx, order.CreateDraftOrderInput{
		ChannelID:       q.ChannelID,
		CustomerID:      &custID,
		Email:           cust.Email,
		BillingAddress:  customerAddressToOrderAddress(*billing),
		ShippingAddress: customerAddressToOrderAddress(*shipping),
		Lines:           lines,
		CustomerNote:    note,
		// The quote's optionsSummary rides the note - don't re-reject
		// required-option SKUs here.
		SkipRequiredOptionCheck: true,
	})
	if err != nil {
		return nil, err
	}
	if _, err := r.B2BSvc.MarkQuoteConverted(ctx, q.ID); err != nil {
		return nil, err
	}
	return toOrderModel(draft), nil
}

func (r *queryResolver) CompanyInvoices(ctx context.Context, companyID string, limit *int32) ([]*model.B2BInvoice, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.view")
	if err != nil {
		return nil, err
	}
	lim := int32(50)
	if limit != nil {
		lim = *limit
	}
	list, err := r.B2BSvc.InvoicesForCompany(ctx, co.ID, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.B2BInvoice, len(list))
	for i, inv := range list {
		out[i] = toB2BInvoiceModel(inv)
	}
	return out, nil
}

// Companies lists wholesale accounts in the admin's b2b.view scope.
func (r *queryResolver) Companies(ctx context.Context) ([]*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	channelIDs, bypass, err := r.b2bScope(ctx, "b2b.view")
	if err != nil {
		return nil, err
	}
	if !bypass && len(channelIDs) == 0 {
		return []*model.Company{}, nil
	}
	list, err := r.B2BSvc.ListCompanies(ctx, channelIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Company, 0, len(list))
	for _, c := range list {
		out = append(out, r.companyModelWithGroup(ctx, c))
	}
	return out, nil
}

// Company returns one account (b2b.view, channel-gated).
func (r *queryResolver) Company(ctx context.Context, id string) (*model.Company, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, id, "b2b.view")
	if err != nil {
		return nil, err
	}
	return r.companyModelWithGroup(ctx, co), nil
}

// CompanyMembers lists the company's customers (b2b.view).
func (r *queryResolver) CompanyMembers(ctx context.Context, companyID string) ([]*model.CompanyMember, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.view")
	if err != nil {
		return nil, err
	}
	members, err := r.B2BSvc.Members(ctx, co.ID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CompanyMember, 0, len(members))
	for _, m := range members {
		out = append(out, &model.CompanyMember{
			CustomerID:      m.CustomerID.String(),
			Role:            m.Role,
			Email:           m.Email,
			FirstName:       m.FirstName,
			LastName:        m.LastName,
			OrderLimitCents: int32(m.OrderLimitCents),
		})
	}
	return out, nil
}

func (r *queryResolver) CompanyCreditEntries(ctx context.Context, companyID string) ([]*model.CompanyCreditEntry, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	co, err := r.b2bCompanyTarget(ctx, companyID, "b2b.view")
	if err != nil {
		return nil, err
	}
	entries, err := r.B2BSvc.CreditEntries(ctx, co.ID, 100)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CompanyCreditEntry, 0, len(entries))
	for _, e := range entries {
		m := &model.CompanyCreditEntry{
			ID:          e.ID.String(),
			AmountCents: int32(e.AmountCents),
			Reason:      e.Reason,
			CreatedAt:   e.CreatedAt,
		}
		if e.OrderID != nil {
			id := e.OrderID.String()
			m.OrderID = &id
		}
		out = append(out, m)
	}
	return out, nil
}

// B2bQuotes is the negotiation queue (b2b.view scope).
func (r *queryResolver) B2bQuotes(ctx context.Context, status *string, limit *int32, offset *int32) ([]*model.B2BQuote, error) {
	if err := r.requireB2B(); err != nil {
		return nil, err
	}
	channelIDs, bypass, err := r.b2bScope(ctx, "b2b.view")
	if err != nil {
		return nil, err
	}
	if !bypass && len(channelIDs) == 0 {
		return []*model.B2BQuote{}, nil
	}
	st := ""
	if status != nil {
		st = *status
	}
	lim, off := int32(50), int32(0)
	if limit != nil {
		lim = *limit
	}
	if offset != nil {
		off = *offset
	}
	quotes, err := r.B2BSvc.QuotesForAdmin(ctx, channelIDs, st, lim, off)
	if err != nil {
		return nil, err
	}
	out := make([]*model.B2BQuote, 0, len(quotes))
	for _, q := range quotes {
		out = append(out, toB2BQuoteModel(q))
	}
	return out, nil
}

func (r *Resolver) Company() CompanyResolver { return &companyResolver{r} }

type companyResolver struct{ *Resolver }
