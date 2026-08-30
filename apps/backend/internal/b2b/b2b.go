// Package b2b is company accounts for wholesale buyers - customers group under a company with shared payment terms and a credit limit, pay on account via the offline purchase-order method (placement reserves credit, recording the payment releases it),
// and negotiate prices through a quote workflow (request -> admin prices -> accept)
package b2b

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/apperr"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/permission"
	"github.com/skeletg/skelet/internal/tx"
)

const (
	QuoteRequested = "requested"
	QuoteQuoted    = "quoted"
	QuoteAccepted  = "accepted"
	QuoteDeclined  = "declined"
	QuoteExpired   = "expired"
)

var (
	ErrCreditExceeded    = apperr.New("B2B_CREDIT_EXCEEDED", apperr.ClassValidation, "this order exceeds your company's available credit")
	ErrNoCompany         = apperr.New("B2B_NO_COMPANY", apperr.ClassValidation, "your account is not linked to a company")
	ErrCompanySuspended  = apperr.New("B2B_COMPANY_SUSPENDED", apperr.ClassValidation, "your company account is suspended - contact the store")
	ErrMemberOrderLimit  = apperr.New("B2B_MEMBER_ORDER_LIMIT", apperr.ClassValidation, "this order exceeds your per-order purchasing limit")
	ErrNotFound          = apperr.New("B2B_NOT_FOUND", apperr.ClassNotFound, "not found")
	ErrDisabled          = apperr.New("B2B_DISABLED", apperr.ClassValidation, "B2B features are disabled on this server") // every B2B operation rejects with this when B2B_ENABLED=false
	ErrPriceNotInCompany = apperr.New("B2B_PRICE_NOT_IN_COMPANY", apperr.ClassNotFound, "that price does not belong to this company")
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "b2b",
		Name:      "B2B",
		SortOrder: 78,
	})
	permission.Register(permission.Definition{
		Group: "b2b", Slug: "b2b.view", Name: "View Companies & Quotes", SortOrder: 1,
	})
	permission.Register(permission.Definition{
		Group: "b2b", Slug: "b2b.manage", Name: "Manage Companies, Credit & Quotes", SortOrder: 2,
	})
}

type Company struct {
	ID                     uuid.UUID
	ChannelID              uuid.UUID
	Name                   string
	Status                 string
	CreditLimitCents       int64
	CreditUsedCents        int64
	PaymentTermsDays       int32
	DefaultCustomerGroupID *uuid.UUID // when set, the wholesale group members are moved into on attach - drives group/tier pricing
	CreatedAt              time.Time
}

type Member struct {
	CompanyID       uuid.UUID
	CustomerID      uuid.UUID
	Role            string
	Email           string
	FirstName       string
	LastName        string
	OrderLimitCents int64
}

type Membership struct { // the customer-side view - their company plus credit headroom
	CompanyID        uuid.UUID
	CompanyName      string
	CompanyStatus    string
	Role             string
	CreditLimitCents int64
	CreditUsedCents  int64
	PaymentTermsDays int32
	OrderLimitCents  int64 // caps a single on-account order for this member, 0 = unlimited
}

type QuoteItem struct {
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Quantity       float64 `json:"quantity"`
	UnitPriceCents int64   `json:"unit_price_cents"`
	OptionsSummary string  `json:"options_summary,omitempty"` // the requesting cart line's custom-option picks as display text ("Engraving: ALICE; Size: XL"), so the personalization survives the quote -> draft-order path
}

type Quote struct {
	ID         uuid.UUID
	CompanyID  *uuid.UUID
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Status     string
	Items      []QuoteItem
	Note       string
	SellerNote string
	TotalCents int64
	ValidUntil *time.Time
	CreatedAt  time.Time
}

type Service struct {
	queries *db.Queries
	tx      *tx.Manager
	bus     *events.Bus
	enabled bool // the server-wide B2B_ENABLED switch - when false the API rejects with ErrDisabled and the engine hooks (contract pricing, on-account credit, quote expiry) no-op
}

func NewService(queries *db.Queries, txMgr *tx.Manager) *Service {
	return &Service{queries: queries, tx: txMgr, enabled: true}
}

func (s *Service) SetEnabled(v bool) { s.enabled = v } // called once at startup from the B2B_ENABLED env var

func (s *Service) Enabled() bool { return s.enabled }

// --- companies ---

func (s *Service) CreateCompany(ctx context.Context, channelID uuid.UUID, name string, creditLimitCents int64, termsDays int32) (Company, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Company{}, apperr.New("B2B_NAME_REQUIRED", apperr.ClassValidation, "company name is required")
	}
	row, err := s.queries.CreateCompany(ctx, db.CreateCompanyParams{
		ChannelID:        channelID,
		Name:             name,
		CreditLimitCents: creditLimitCents,
		PaymentTermsDays: termsDays,
	})
	if err != nil {
		return Company{}, fmt.Errorf("create company: %w", err)
	}
	co := toCompany(row)
	s.publish(ctx, &CompanyCreated{
		CompanyID:        co.ID,
		ChannelID:        co.ChannelID,
		Name:             co.Name,
		CreditLimitCents: co.CreditLimitCents,
		PaymentTermsDays: co.PaymentTermsDays,
	})
	return co, nil
}

func (s *Service) GetCompany(ctx context.Context, id uuid.UUID) (Company, error) {
	row, err := s.queries.GetCompany(ctx, id)
	if err != nil {
		return Company{}, ErrNotFound
	}
	return toCompany(row), nil
}

func (s *Service) ListCompanies(ctx context.Context, channelIDs []uuid.UUID) ([]Company, error) {
	rows, err := s.queries.ListCompanies(ctx, db.ListCompaniesParams{
		AllChannels: len(channelIDs) == 0,
		ChannelIds:  channelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("list companies: %w", err)
	}
	out := make([]Company, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCompany(r))
	}
	return out, nil
}

func (s *Service) UpdateCompany(ctx context.Context, id uuid.UUID, name, status string, creditLimitCents int64, termsDays int32) (Company, error) {
	if status != "active" && status != "suspended" {
		return Company{}, apperr.New("B2B_BAD_STATUS", apperr.ClassValidation, "status must be active or suspended")
	}
	row, err := s.queries.UpdateCompany(ctx, db.UpdateCompanyParams{
		ID:               id,
		Name:             strings.TrimSpace(name),
		Status:           status,
		CreditLimitCents: creditLimitCents,
		PaymentTermsDays: termsDays,
	})
	if err != nil {
		return Company{}, ErrNotFound
	}
	co := toCompany(row)
	s.publish(ctx, &CompanyUpdated{
		CompanyID:        co.ID,
		ChannelID:        co.ChannelID,
		Name:             co.Name,
		Status:           co.Status,
		CreditLimitCents: co.CreditLimitCents,
		PaymentTermsDays: co.PaymentTermsDays,
	})
	return co, nil
}

func (s *Service) SetMember(ctx context.Context, companyID, customerID uuid.UUID, role string, orderLimitCents int64) error {
	if role != "admin" && role != "buyer" {
		role = "buyer"
	}
	if err := s.queries.UpsertCompanyMember(ctx, db.UpsertCompanyMemberParams{
		CompanyID:  companyID,
		CustomerID: customerID,
		Role:       role,
	}); err != nil {
		return fmt.Errorf("set company member: %w", err)
	}
	if orderLimitCents >= 0 {
		if err := s.queries.SetCompanyMemberOrderLimit(ctx, db.SetCompanyMemberOrderLimitParams{
			CustomerID:      customerID,
			OrderLimitCents: orderLimitCents,
		}); err != nil {
			return fmt.Errorf("set member order limit: %w", err)
		}
	}
	co, err := s.queries.GetCompany(ctx, companyID) // wholesale pricing - the member moves into the company's default customer group, so group/tier prices apply on their next request
	if err == nil && co.DefaultCustomerGroupID.Valid {
		if err := s.queries.AssignCustomerGroup(ctx, db.AssignCustomerGroupParams{
			ID:              customerID,
			CustomerGroupID: co.DefaultCustomerGroupID.UUID,
		}); err != nil {
			return fmt.Errorf("assign customer group: %w", err)
		}
	}
	ev := &CompanyMemberChanged{CompanyID: companyID, CustomerID: customerID, Action: "added", Role: role}
	if err == nil {
		ev.ChannelID = co.ChannelID
	}
	s.publish(ctx, ev)
	return nil
}

func (s *Service) SetDefaultCustomerGroup(ctx context.Context, companyID uuid.UUID, groupSlug string) (Company, error) { // "" clears the group - existing members are re-assigned immediately
	groupID := uuid.NullUUID{}
	if strings.TrimSpace(groupSlug) != "" {
		id, err := s.queries.GetCustomerGroupIDBySlug(ctx, strings.TrimSpace(groupSlug))
		if err != nil {
			return Company{}, apperr.New("B2B_GROUP_UNKNOWN", apperr.ClassValidation, "unknown customer group slug")
		}
		groupID = uuid.NullUUID{UUID: id, Valid: true}
	}
	row, err := s.queries.SetCompanyDefaultGroup(ctx, db.SetCompanyDefaultGroupParams{
		ID:      companyID,
		GroupID: groupID,
	})
	if err != nil {
		return Company{}, ErrNotFound
	}
	if groupID.Valid {
		members, err := s.queries.ListCompanyMembers(ctx, companyID)
		if err != nil {
			return Company{}, fmt.Errorf("list members: %w", err)
		}
		for _, m := range members {
			if err := s.queries.AssignCustomerGroup(ctx, db.AssignCustomerGroupParams{
				ID:              m.CustomerID,
				CustomerGroupID: groupID.UUID,
			}); err != nil {
				return Company{}, fmt.Errorf("assign customer group: %w", err)
			}
		}
	}
	return toCompany(row), nil
}

func (s *Service) RemoveMember(ctx context.Context, customerID uuid.UUID) error {
	var companyID uuid.UUID // best-effort lookup before the row goes away - the event should say WHICH company lost the member
	if m, err := s.MembershipFor(ctx, customerID); err == nil {
		companyID = m.CompanyID
	}
	if err := s.queries.RemoveCompanyMember(ctx, customerID); err != nil {
		return err
	}
	s.publish(ctx, &CompanyMemberChanged{CompanyID: companyID, CustomerID: customerID, Action: "removed"})
	return nil
}

func (s *Service) Members(ctx context.Context, companyID uuid.UUID) ([]Member, error) {
	rows, err := s.queries.ListCompanyMembers(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, Member{
			CompanyID:       r.CompanyID,
			CustomerID:      r.CustomerID,
			Role:            r.Role,
			Email:           r.Email,
			FirstName:       r.FirstName,
			LastName:        r.LastName,
			OrderLimitCents: r.OrderLimitCents,
		})
	}
	return out, nil
}

func (s *Service) MembershipFor(ctx context.Context, customerID uuid.UUID) (Membership, error) {
	row, err := s.queries.GetMembershipByCustomer(ctx, customerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Membership{}, ErrNoCompany
		}
		return Membership{}, fmt.Errorf("membership: %w", err)
	}
	return Membership{
		CompanyID:        row.CompanyID,
		CompanyName:      row.CompanyName,
		CompanyStatus:    row.CompanyStatus,
		Role:             row.Role,
		CreditLimitCents: row.CreditLimitCents,
		CreditUsedCents:  row.CreditUsedCents,
		PaymentTermsDays: row.PaymentTermsDays,
		OrderLimitCents:  row.OrderLimitCents,
	}, nil
}

func (s *Service) CustomerGroupSlug(ctx context.Context, id uuid.UUID) (string, error) { // for display
	return s.queries.GetCustomerGroupSlugByID(ctx, id)
}

// --- credit ---

func (s *Service) ReserveCreditTx(ctx context.Context, sqlTx *sql.Tx, customerID, orderID uuid.UUID, orderNumber string, amountCents int64) error { // ReserveCredit joining the caller's order-placement transaction, so the reservation commits or rolls back with the order itself
	if !s.enabled {
		return ErrDisabled
	}
	q := s.queries.WithTx(sqlTx)
	row, err := q.GetMembershipByCustomer(ctx, customerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoCompany
		}
		return fmt.Errorf("membership: %w", err)
	}
	if row.OrderLimitCents > 0 && amountCents > row.OrderLimitCents {
		return ErrMemberOrderLimit
	}
	co, err := q.GetCompanyForUpdate(ctx, row.CompanyID)
	if err != nil {
		return ErrNotFound
	}
	if co.Status != "active" {
		return ErrCompanySuspended
	}
	if co.CreditUsedCents+amountCents > co.CreditLimitCents {
		return ErrCreditExceeded
	}
	if _, err := q.AdjustCompanyCreditUsed(ctx, db.AdjustCompanyCreditUsedParams{
		ID:              row.CompanyID,
		CreditUsedCents: amountCents,
	}); err != nil {
		return fmt.Errorf("reserve credit: %w", err)
	}
	if _, err := q.InsertB2BCreditEntry(ctx, db.InsertB2BCreditEntryParams{
		CompanyID:   row.CompanyID,
		OrderID:     uuid.NullUUID{UUID: orderID, Valid: true},
		AmountCents: amountCents,
		Reason:      "order placed on account",
	}); err != nil {
		return fmt.Errorf("record credit reserve: %w", err)
	}
	due := sql.NullTime{} // the receivable rides the same transaction - due per the company's payment terms, numbered off the order for easy matching
	if co.PaymentTermsDays > 0 {
		due = sql.NullTime{Time: time.Now().UTC().AddDate(0, 0, int(co.PaymentTermsDays)), Valid: true}
	}
	if _, err := q.CreateB2BInvoice(ctx, db.CreateB2BInvoiceParams{
		CompanyID:   row.CompanyID,
		OrderID:     orderID,
		Number:      "INV-" + strings.TrimPrefix(orderNumber, "ORD-"),
		AmountCents: amountCents,
		DueAt:       due,
	}); err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}
	return nil
}

func (s *Service) ReleaseCredit(ctx context.Context, companyID uuid.UUID, amountCents int64, adminID *uuid.UUID, reason string) error { // headroom back on a settled invoice - floored at zero in SQL, and the manual release lands in the audit ledger
	if _, err := s.queries.AdjustCompanyCreditUsed(ctx, db.AdjustCompanyCreditUsedParams{
		ID:              companyID,
		CreditUsedCents: -amountCents,
	}); err != nil {
		return fmt.Errorf("release credit: %w", err)
	}
	if reason == "" {
		reason = "manual credit release"
	}
	if _, err := s.queries.InsertB2BCreditEntry(ctx, db.InsertB2BCreditEntryParams{
		CompanyID:   companyID,
		AmountCents: -amountCents,
		Reason:      reason,
		AdminID:     nullableUUID(adminID),
	}); err != nil {
		return fmt.Errorf("record credit release: %w", err)
	}
	s.publish(ctx, &CompanyCreditReleased{CompanyID: companyID, AmountCents: amountCents, Reason: reason})
	return nil
}

type CreditEntry struct { // one row of the company credit audit ledger
	ID          uuid.UUID
	OrderID     *uuid.UUID
	AmountCents int64
	Reason      string
	CreatedAt   time.Time
}

func (s *Service) CreditEntries(ctx context.Context, companyID uuid.UUID, limit int32) ([]CreditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListB2BCreditEntries(ctx, db.ListB2BCreditEntriesParams{
		CompanyID: companyID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list credit entries: %w", err)
	}
	out := make([]CreditEntry, 0, len(rows))
	for _, r := range rows {
		e := CreditEntry{ID: r.ID, AmountCents: r.AmountCents, Reason: r.Reason, CreatedAt: r.CreatedAt}
		if r.OrderID.Valid {
			id := r.OrderID.UUID
			e.OrderID = &id
		}
		out = append(out, e)
	}
	return out, nil
}

type CompanyOrder struct { // one member order in the company-wide history
	ID            uuid.UUID
	Number        string
	Status        string
	PaymentStatus string
	TotalCents    int64
	CurrencyCode  string
	MemberEmail   string
	CreatedAt     time.Time
}

func (s *Service) OrdersForCompany(ctx context.Context, companyID uuid.UUID, limit int32) ([]CompanyOrder, error) { // every member's orders, the company-admin view
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.queries.ListOrdersByCompany(ctx, db.ListOrdersByCompanyParams{
		CompanyID: companyID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list company orders: %w", err)
	}
	out := make([]CompanyOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompanyOrder{
			ID:            r.ID,
			Number:        r.Number,
			Status:        r.Status,
			PaymentStatus: r.PaymentStatus,
			TotalCents:    r.TotalAmount,
			CurrencyCode:  r.CurrencyCode,
			MemberEmail:   r.MemberEmail,
			CreatedAt:     r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) MarkQuoteConverted(ctx context.Context, id uuid.UUID) (Quote, error) { // stamps an accepted quote as turned into an order
	row, err := s.queries.SetB2BQuoteStatusAdmin(ctx, db.SetB2BQuoteStatusAdminParams{
		ID:     id,
		Status: "converted",
	})
	if err != nil {
		return Quote{}, ErrNotFound
	}
	return toQuote(row)
}

// --- quotes ---

func (s *Service) RequestQuote(ctx context.Context, customerID, channelID uuid.UUID, items []QuoteItem, note string) (Quote, error) { // snapshots the requested lines into a negotiation
	if len(items) == 0 {
		return Quote{}, apperr.New("B2B_EMPTY_QUOTE", apperr.ClassValidation, "a quote request needs at least one line")
	}
	var companyID uuid.NullUUID
	if m, err := s.MembershipFor(ctx, customerID); err == nil {
		companyID = uuid.NullUUID{UUID: m.CompanyID, Valid: true}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return Quote{}, fmt.Errorf("marshal quote items: %w", err)
	}
	row, err := s.queries.InsertB2BQuote(ctx, db.InsertB2BQuoteParams{
		CompanyID:  companyID,
		CustomerID: customerID,
		ChannelID:  channelID,
		Items:      raw,
		Note:       strings.TrimSpace(note),
	})
	if err != nil {
		return Quote{}, fmt.Errorf("insert quote: %w", err)
	}
	q, qerr := toQuote(row)
	if qerr == nil {
		s.publish(ctx, &QuoteRequestedEvent{
			QuoteID:    q.ID,
			CompanyID:  q.CompanyID,
			CustomerID: q.CustomerID,
			ChannelID:  q.ChannelID,
			Lines:      len(q.Items),
			Note:       q.Note,
		})
	}
	return q, qerr
}

func (s *Service) QuotesForCustomer(ctx context.Context, customerID uuid.UUID) ([]Quote, error) {
	rows, err := s.queries.ListB2BQuotesForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list quotes: %w", err)
	}
	return toQuotes(rows)
}

func (s *Service) QuotesForAdmin(ctx context.Context, channelIDs []uuid.UUID, status string, limit, offset int32) ([]Quote, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListB2BQuotesForAdmin(ctx, db.ListB2BQuotesForAdminParams{
		Limit:       limit,
		Offset:      offset,
		AllChannels: len(channelIDs) == 0,
		ChannelIds:  channelIDs,
		Status:      status,
	})
	if err != nil {
		return nil, fmt.Errorf("list quotes: %w", err)
	}
	return toQuotes(rows)
}

func (s *Service) GetQuote(ctx context.Context, id uuid.UUID) (Quote, error) {
	row, err := s.queries.GetB2BQuote(ctx, id)
	if err != nil {
		return Quote{}, ErrNotFound
	}
	return toQuote(row)
}

func (s *Service) PriceQuote(ctx context.Context, id uuid.UUID, items []QuoteItem, sellerNote string, validUntil *time.Time) (Quote, error) { // the admin action - reprice lines, set validity, publish
	if len(items) == 0 {
		return Quote{}, apperr.New("B2B_EMPTY_QUOTE", apperr.ClassValidation, "a quote needs at least one line")
	}
	var total int64
	for _, it := range items {
		total += int64(float64(it.UnitPriceCents) * it.Quantity)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return Quote{}, fmt.Errorf("marshal quote items: %w", err)
	}
	vu := sql.NullTime{}
	if validUntil != nil {
		vu = sql.NullTime{Time: *validUntil, Valid: true}
	}
	row, err := s.queries.UpdateB2BQuoteAdmin(ctx, db.UpdateB2BQuoteAdminParams{
		ID:         id,
		Status:     QuoteQuoted,
		Items:      raw,
		TotalCents: total,
		SellerNote: strings.TrimSpace(sellerNote),
		ValidUntil: vu,
	})
	if err != nil {
		return Quote{}, ErrNotFound
	}
	q, qerr := toQuote(row)
	if qerr == nil {
		s.publish(ctx, &QuotePriced{
			QuoteID:    q.ID,
			CompanyID:  q.CompanyID,
			CustomerID: q.CustomerID,
			ChannelID:  q.ChannelID,
			TotalCents: q.TotalCents,
		})
	}
	return q, qerr
}

func (s *Service) RespondToQuote(ctx context.Context, customerID, id uuid.UUID, accept bool) (Quote, error) { // the customer action - accept or decline a quoted offer
	q, err := s.GetQuote(ctx, id)
	if err != nil {
		return Quote{}, err
	}
	if q.CustomerID != customerID {
		return Quote{}, ErrNotFound
	}
	if q.Status != QuoteQuoted {
		return Quote{}, apperr.New("B2B_QUOTE_NOT_OPEN", apperr.ClassValidation, "this quote is not awaiting a response")
	}
	status := QuoteDeclined
	if accept {
		if q.ValidUntil != nil && time.Now().After(*q.ValidUntil) {
			return Quote{}, apperr.New("B2B_QUOTE_EXPIRED", apperr.ClassValidation, "this quote has expired")
		}
		status = QuoteAccepted
	}
	row, err := s.queries.SetB2BQuoteStatus(ctx, db.SetB2BQuoteStatusParams{
		ID:         id,
		Status:     status,
		CustomerID: customerID,
	})
	if err != nil {
		return Quote{}, ErrNotFound
	}
	res, qerr := toQuote(row)
	if qerr == nil {
		s.publish(ctx, &QuoteResponded{
			QuoteID:    res.ID,
			CompanyID:  res.CompanyID,
			CustomerID: res.CustomerID,
			ChannelID:  res.ChannelID,
			Status:     res.Status,
			TotalCents: res.TotalCents,
		})
	}
	return res, qerr
}

// --- mapping ---

func toCompany(r db.Company) Company {
	c := Company{
		ID:               r.ID,
		ChannelID:        r.ChannelID,
		Name:             r.Name,
		Status:           r.Status,
		CreditLimitCents: r.CreditLimitCents,
		CreditUsedCents:  r.CreditUsedCents,
		PaymentTermsDays: r.PaymentTermsDays,
		CreatedAt:        r.CreatedAt,
	}
	if r.DefaultCustomerGroupID.Valid {
		id := r.DefaultCustomerGroupID.UUID
		c.DefaultCustomerGroupID = &id
	}
	return c
}

func toQuotes(rows []db.B2bQuote) ([]Quote, error) {
	out := make([]Quote, 0, len(rows))
	for _, r := range rows {
		q, err := toQuote(r)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

func toQuote(r db.B2bQuote) (Quote, error) {
	q := Quote{
		ID:         r.ID,
		CustomerID: r.CustomerID,
		ChannelID:  r.ChannelID,
		Status:     r.Status,
		Note:       r.Note,
		SellerNote: r.SellerNote,
		TotalCents: r.TotalCents,
		CreatedAt:  r.CreatedAt,
	}
	if r.CompanyID.Valid {
		id := r.CompanyID.UUID
		q.CompanyID = &id
	}
	if r.ValidUntil.Valid {
		t := r.ValidUntil.Time
		q.ValidUntil = &t
	}
	if len(r.Items) > 0 {
		if err := json.Unmarshal(r.Items, &q.Items); err != nil {
			return Quote{}, fmt.Errorf("decode quote items: %w", err)
		}
	}
	return q, nil
}

func nullableUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

type CompanyPrice struct { // one negotiated per-product unit price for a company
	ID         uuid.UUID
	CompanyID  uuid.UUID
	ProductID  uuid.UUID
	ProductSKU string
	PriceCents int64
}

func (s *Service) SetCompanyPrice(ctx context.Context, companyID, productID uuid.UUID, priceCents int64) (CompanyPrice, error) {
	if priceCents < 0 {
		return CompanyPrice{}, fmt.Errorf("price must be >= 0")
	}
	row, err := s.queries.UpsertCompanyPrice(ctx, db.UpsertCompanyPriceParams{CompanyID: companyID, ProductID: productID, PriceCents: priceCents})
	if err != nil {
		return CompanyPrice{}, fmt.Errorf("set company price: %w", err)
	}
	return CompanyPrice{ID: row.ID, CompanyID: row.CompanyID, ProductID: row.ProductID, PriceCents: row.PriceCents}, nil
}

func (s *Service) DeleteCompanyPrice(ctx context.Context, companyID, id uuid.UUID) error { // deletes only when the row belongs to companyID - a cross-company id is ErrPriceNotInCompany rather than a silent no-op
	n, err := s.queries.DeleteCompanyPrice(ctx, db.DeleteCompanyPriceParams{ID: id, CompanyID: companyID})
	if err != nil {
		return fmt.Errorf("delete company price: %w", err)
	}
	if n == 0 {
		return ErrPriceNotInCompany
	}
	return nil
}

func (s *Service) CompanyPrices(ctx context.Context, companyID uuid.UUID) ([]CompanyPrice, error) {
	rows, err := s.queries.ListCompanyPrices(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list company prices: %w", err)
	}
	out := make([]CompanyPrice, len(rows))
	for i, r := range rows {
		out[i] = CompanyPrice{ID: r.ID, CompanyID: r.CompanyID, ProductID: r.ProductID, ProductSKU: r.ProductSku, PriceCents: r.PriceCents}
	}
	return out, nil
}

func (s *Service) ContractPriceFor(ctx context.Context, customerID, productID uuid.UUID) (int64, bool) { // the price the customer gets via their active company, else (0, false) - fail-soft, so a lookup error reads as "no contract price" and pricing never breaks on a B2B hiccup
	if !s.enabled {
		return 0, false
	}
	cents, err := s.queries.ContractPriceForCustomer(ctx, db.ContractPriceForCustomerParams{CustomerID: customerID, ProductID: productID})
	if err != nil {
		return 0, false
	}
	return cents, true
}

func (s *Service) ExpireQuotes(ctx context.Context) (int64, error) { // sweeps quotes past their validity window into 'expired', rather than leaving it to a lazy check at accept time
	if !s.enabled {
		return 0, nil
	}
	return s.queries.ExpireB2BQuotes(ctx)
}

// --- invoices ---

type Invoice struct { // one on-account receivable, created at placement inside the credit-reservation transaction - settling it (paid/void) releases the reserved credit in the same transaction
	ID          uuid.UUID
	CompanyID   uuid.UUID
	OrderID     uuid.UUID
	OrderNumber string // hydrated on list reads
	Number      string
	AmountCents int64
	Status      string // open | paid | void
	DueAt       *time.Time
	PaidAt      *time.Time
	CreatedAt   time.Time
}

func toInvoice(row db.B2bInvoice) Invoice {
	inv := Invoice{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		OrderID:     row.OrderID,
		Number:      row.Number,
		AmountCents: row.AmountCents,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
	}
	if row.DueAt.Valid {
		t := row.DueAt.Time
		inv.DueAt = &t
	}
	if row.PaidAt.Valid {
		t := row.PaidAt.Time
		inv.PaidAt = &t
	}
	return inv
}

func (s *Service) InvoicesForCompany(ctx context.Context, companyID uuid.UUID, limit int32) ([]Invoice, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListB2BInvoicesByCompany(ctx, db.ListB2BInvoicesByCompanyParams{
		CompanyID: companyID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	out := make([]Invoice, len(rows))
	for i, r := range rows {
		inv := toInvoice(db.B2bInvoice{
			ID: r.ID, CompanyID: r.CompanyID, OrderID: r.OrderID, Number: r.Number,
			AmountCents: r.AmountCents, Status: r.Status, DueAt: r.DueAt, PaidAt: r.PaidAt,
			CreatedAt: r.CreatedAt,
		})
		inv.OrderNumber = r.OrderNumber
		out[i] = inv
	}
	return out, nil
}

func (s *Service) GetInvoice(ctx context.Context, id uuid.UUID) (Invoice, error) {
	row, err := s.queries.GetB2BInvoice(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Invoice{}, ErrNotFound
		}
		return Invoice{}, fmt.Errorf("get invoice: %w", err)
	}
	return toInvoice(row), nil
}

var ErrInvoiceNotOpen = errors.New("b2b: invoice is not open") // guards double settlement - paid/void are terminal, their credit already released once

func (s *Service) SettleInvoice(ctx context.Context, id uuid.UUID, toStatus string, adminID *uuid.UUID) (Invoice, error) { // flip plus credit release in one transaction, so a crash can't release twice or leave a settled invoice with held credit - the conditional UPDATE on status = 'open' is the double-settlement guard
	if toStatus != "paid" && toStatus != "void" {
		return Invoice{}, fmt.Errorf("b2b: settle status must be paid or void")
	}
	var settled Invoice
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		q := s.queries.WithTx(sqlTx)
		row, err := q.SettleB2BInvoice(ctx, db.SettleB2BInvoiceParams{ID: id, Status: toStatus})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvoiceNotOpen
			}
			return fmt.Errorf("settle invoice: %w", err)
		}
		settled = toInvoice(row)
		reason := fmt.Sprintf("invoice %s %s", settled.Number, toStatus)
		if _, err := q.AdjustCompanyCreditUsed(ctx, db.AdjustCompanyCreditUsedParams{
			ID:              settled.CompanyID,
			CreditUsedCents: -settled.AmountCents,
		}); err != nil {
			return fmt.Errorf("release credit: %w", err)
		}
		if _, err := q.InsertB2BCreditEntry(ctx, db.InsertB2BCreditEntryParams{
			CompanyID:   settled.CompanyID,
			OrderID:     uuid.NullUUID{UUID: settled.OrderID, Valid: true},
			AmountCents: -settled.AmountCents,
			Reason:      reason,
			AdminID:     nullableUUID(adminID),
		}); err != nil {
			return fmt.Errorf("record credit release: %w", err)
		}
		return nil
	})
	if err != nil {
		return Invoice{}, err
	}
	s.publish(ctx, &InvoiceSettled{
		InvoiceID:   settled.ID,
		CompanyID:   settled.CompanyID,
		OrderID:     settled.OrderID,
		Number:      settled.Number,
		Status:      settled.Status,
		AmountCents: settled.AmountCents,
	})
	return settled, nil
}
