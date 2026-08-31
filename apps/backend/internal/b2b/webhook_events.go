package b2b

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/events"
)

// B2B lifecycle events, published post-commit for outbound webhooks, with ERP sync the classic consumer, and any plugin subscriber - async and best-effort

const (
	EventCompanyCreated        = "company.created"
	EventCompanyUpdated        = "company.updated"
	EventCompanyMemberChanged  = "company.member.changed"
	EventCompanyCreditReleased = "company.credit.released"
	EventQuoteRequested        = "b2b.quote.requested"
	EventQuotePriced           = "b2b.quote.priced"
	EventQuoteResponded        = "b2b.quote.responded"
	EventInvoiceSettled        = "b2b.invoice.settled"
)

type CompanyCreated struct {
	CompanyID        uuid.UUID
	ChannelID        uuid.UUID
	Name             string
	CreditLimitCents int64
	PaymentTermsDays int32
}

func (*CompanyCreated) Type() string                { return EventCompanyCreated }
func (e *CompanyCreated) EventChannelID() uuid.UUID { return e.ChannelID }

type CompanyUpdated struct {
	CompanyID        uuid.UUID
	ChannelID        uuid.UUID
	Name             string
	Status           string
	CreditLimitCents int64
	PaymentTermsDays int32
}

func (*CompanyUpdated) Type() string                { return EventCompanyUpdated }
func (e *CompanyUpdated) EventChannelID() uuid.UUID { return e.ChannelID }

type CompanyMemberChanged struct { // fires on attach, update and detach - Action is added or removed, and ChannelID is uuid.Nil on remove, since membership is looked up by customer and the company may already be gone
	CompanyID  uuid.UUID
	ChannelID  uuid.UUID
	CustomerID uuid.UUID
	Action     string
	Role       string
}

func (*CompanyMemberChanged) Type() string { return EventCompanyMemberChanged }

type CompanyCreditReleased struct {
	CompanyID   uuid.UUID
	AmountCents int64
	Reason      string
}

func (*CompanyCreditReleased) Type() string { return EventCompanyCreditReleased }

type QuoteRequestedEvent struct {
	QuoteID    uuid.UUID
	CompanyID  *uuid.UUID
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Lines      int
	Note       string
}

func (*QuoteRequestedEvent) Type() string                { return EventQuoteRequested }
func (e *QuoteRequestedEvent) EventChannelID() uuid.UUID { return e.ChannelID }

type QuotePriced struct {
	QuoteID    uuid.UUID
	CompanyID  *uuid.UUID
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	TotalCents int64
}

func (*QuotePriced) Type() string                { return EventQuotePriced }
func (e *QuotePriced) EventChannelID() uuid.UUID { return e.ChannelID }

type QuoteResponded struct { // fires on the customer's decision - Status is accepted or declined
	QuoteID    uuid.UUID
	CompanyID  *uuid.UUID
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Status     string
	TotalCents int64
}

func (*QuoteResponded) Type() string                { return EventQuoteResponded }
func (e *QuoteResponded) EventChannelID() uuid.UUID { return e.ChannelID }

type InvoiceSettled struct { // fires when an open on-account invoice is marked paid or void - the reserved company credit was released in the same transaction
	InvoiceID   uuid.UUID
	CompanyID   uuid.UUID
	OrderID     uuid.UUID
	Number      string
	Status      string
	AmountCents int64
}

func (*InvoiceSettled) Type() string { return EventInvoiceSettled }

func (s *Service) SetEventBus(bus *events.Bus) { s.bus = bus } // separate from SubscribeToOrderEvents, the credit-release consumer, so publishing works even if the subscriber wiring changes

func (s *Service) publish(ctx context.Context, e events.Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}
