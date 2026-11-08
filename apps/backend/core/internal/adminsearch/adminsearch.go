// Package adminsearch is the dashboard's ⌘K lookup across orders, customers and products - a handful of light rows per
// kind, matched on a substring of what an admin types, without the counting and loading a list page does
package adminsearch

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

const (
	// MinTermRunes is the shortest term searched - pg_trgm needs three characters before its indexes can serve a pattern
	MinTermRunes = 3
	// maxTermRunes caps a pasted wall of text; nothing that long names one record
	maxTermRunes = 100
	// PerKind is how many rows of each kind come back
	PerKind = 5
)

type Order struct {
	ID     uuid.UUID
	Number string
	Email  string
	Name   string // the name it ships to
}

type Customer struct {
	ID    uuid.UUID
	Email string
	Name  string
}

type Product struct {
	ID   uuid.UUID
	SKU  string
	Name string
}

type Result struct {
	Orders    []Order
	Customers []Customer
	Products  []Product
}

// Scope is what the caller may read: which kinds, and for the channel-owned ones which channels - nil channels is every
// channel. A kind left off is not searched at all
type Scope struct {
	Orders           bool
	OrderChannels    []uuid.UUID
	Customers        bool
	CustomerChannels []uuid.UUID
	Products         bool
}

type Querier interface {
	AdminSearchOrders(ctx context.Context, arg db.AdminSearchOrdersParams) ([]db.AdminSearchOrdersRow, error)
	AdminSearchCustomers(ctx context.Context, arg db.AdminSearchCustomersParams) ([]db.AdminSearchCustomersRow, error)
	AdminSearchProducts(ctx context.Context, arg db.AdminSearchProductsParams) ([]db.AdminSearchProductsRow, error)
}

type Service struct{ q Querier }

func New(q Querier) *Service { return &Service{q: q} }

// Search finds up to PerKind of each kind the scope allows, newest first, and nothing for a term shorter than
// MinTermRunes or longer than maxTermRunes. Product names are in locale where a translation exists
func (s *Service) Search(ctx context.Context, term string, scope Scope, locale string) (Result, error) {
	out := Result{Orders: []Order{}, Customers: []Customer{}, Products: []Product{}}
	term = strings.TrimSpace(term)
	if n := utf8.RuneCountInString(term); n < MinTermRunes || n > maxTermRunes {
		return out, nil
	}
	if scope.Orders {
		rows, err := s.q.AdminSearchOrders(ctx, db.AdminSearchOrdersParams{ChannelIds: scope.OrderChannels, Term: term, RowLimit: PerKind})
		if err != nil {
			return Result{}, fmt.Errorf("search orders: %w", err)
		}
		for _, r := range rows {
			out.Orders = append(out.Orders, Order{ID: r.ID, Number: r.Number, Email: r.Email, Name: fullName(r.ShippingFirstName, r.ShippingLastName)})
		}
	}
	if scope.Customers {
		rows, err := s.q.AdminSearchCustomers(ctx, db.AdminSearchCustomersParams{ChannelIds: scope.CustomerChannels, Term: term, RowLimit: PerKind})
		if err != nil {
			return Result{}, fmt.Errorf("search customers: %w", err)
		}
		for _, r := range rows {
			out.Customers = append(out.Customers, Customer{ID: r.ID, Email: r.Email, Name: fullName(r.FirstName, r.LastName)})
		}
	}
	if scope.Products {
		rows, err := s.q.AdminSearchProducts(ctx, db.AdminSearchProductsParams{Locale: locale, Term: term, RowLimit: PerKind})
		if err != nil {
			return Result{}, fmt.Errorf("search products: %w", err)
		}
		for _, r := range rows {
			out.Products = append(out.Products, Product{ID: r.ID, SKU: r.Sku, Name: r.Name})
		}
	}
	return out, nil
}

func fullName(first, last string) string {
	return strings.TrimSpace(first + " " + last)
}
