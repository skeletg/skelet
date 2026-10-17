// Package wishlist stores the signed-in customer's saved products - One implicit list per customer - guests keep a
// client-side list the storefront merges after login
package wishlist

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

const (
	EventItemAdded   = "wishlist.item_added"
	EventItemRemoved = "wishlist.item_removed"
)

// ItemAdded / ItemRemoved fire after a customer saves or unsaves a product - Async
type ItemAdded struct {
	CustomerID uuid.UUID
	ProductID  uuid.UUID
}

func (*ItemAdded) Type() string { return EventItemAdded }

type ItemRemoved struct {
	CustomerID uuid.UUID
	ProductID  uuid.UUID
}

func (*ItemRemoved) Type() string { return EventItemRemoved }

type Service struct {
	queries *db.Queries
	bus     *events.Bus
}

func NewService(queries *db.Queries) *Service { return &Service{queries: queries} }

// SetEventBus is optional and nil-safe
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

func (s *Service) publish(ctx context.Context, e events.Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

// MaxItems bounds one list - the account page renders every saved product, so an unbounded list is an unbounded page
const MaxItems = 500

var (
	ErrProductNotFound = errors.New("that product doesn't exist")
	ErrFull            = fmt.Errorf("a wishlist holds at most %d products - remove some to save more", MaxItems)
)

func (s *Service) Add(ctx context.Context, customerID, productID uuid.UUID) error {
	n, err := s.queries.AddWishlistItem(ctx, db.AddWishlistItemParams{CustomerID: customerID, ProductID: productID, MaxItems: MaxItems})
	if pgdb.IsForeignKeyViolation(err) {
		return ErrProductNotFound
	}
	if err != nil {
		return fmt.Errorf("add wishlist item: %w", err)
	}
	if n == 0 {
		saved, err := s.queries.WishlistHasItem(ctx, db.WishlistHasItemParams{CustomerID: customerID, ProductID: productID})
		if err != nil {
			return fmt.Errorf("check wishlist item: %w", err)
		}
		if !saved {
			return ErrFull
		}
	}
	s.publish(ctx, &ItemAdded{CustomerID: customerID, ProductID: productID})
	return nil
}

func (s *Service) Remove(ctx context.Context, customerID, productID uuid.UUID) error {
	if err := s.queries.RemoveWishlistItem(ctx, db.RemoveWishlistItemParams{CustomerID: customerID, ProductID: productID}); err != nil {
		return fmt.Errorf("remove wishlist item: %w", err)
	}
	s.publish(ctx, &ItemRemoved{CustomerID: customerID, ProductID: productID})
	return nil
}

func (s *Service) ListIDs(ctx context.Context, customerID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.queries.ListWishlistProductIDs(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list wishlist: %w", err)
	}
	return ids, nil
}

// shareAlphabet omits easily-confused symbols for hand-typed links
const shareAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// ShareToken returns the customer's public wishlist token, minting one
// on first use - Tokens are stable so shared links keep working
func (s *Service) ShareToken(ctx context.Context, customerID uuid.UUID) (string, error) {
	if existing, err := s.queries.GetWishlistShareByCustomer(ctx, customerID); err == nil {
		return existing.Token, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("get wishlist share: %w", err)
	}
	// rand.Int draws each character uniformly - a byte modulo 31 favours the first eight
	token := make([]byte, 16)
	for i := range token {
		n, err := crand.Int(crand.Reader, big.NewInt(int64(len(shareAlphabet))))
		if err != nil {
			return "", fmt.Errorf("wishlist share: read random: %w", err)
		}
		token[i] = shareAlphabet[n.Int64()]
	}
	row, err := s.queries.UpsertWishlistShare(ctx, db.UpsertWishlistShareParams{
		CustomerID: customerID,
		Token:      string(token),
	})
	if err != nil {
		return "", fmt.Errorf("upsert wishlist share: %w", err)
	}
	return row.Token, nil
}

// RevokeShare stops the customer's current share link working - the next ShareToken mints a fresh one
func (s *Service) RevokeShare(ctx context.Context, customerID uuid.UUID) error {
	if err := s.queries.DeleteWishlistShare(ctx, customerID); err != nil {
		return fmt.Errorf("revoke wishlist share: %w", err)
	}
	return nil
}

// ResolveShare maps a public token to the owning customer's wishlist
// product ids - Unknown tokens return an empty list, not an error
func (s *Service) ResolveShare(ctx context.Context, token string) ([]uuid.UUID, error) {
	share, err := s.queries.GetWishlistShareByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve wishlist share: %w", err)
	}
	return s.ListIDs(ctx, share.CustomerID)
}

func init() {
	apperr.RegisterSentinel(ErrProductNotFound, "WISHLIST_PRODUCT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrFull, "WISHLIST_FULL", apperr.ClassValidation)
}
