package download

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/tx"
)

type Service struct {
	repo   Repository
	tx     *tx.Manager
	now    func() time.Time
	orders OrderDownloadLookup
	// When set through SubscribeToOrderEvents, publishes download.accessed for the Synapse activity tracker - nil means no publishing
	bus *events.Bus
}

func NewService(repo Repository, txMgr *tx.Manager) *Service {
	return &Service{repo: repo, tx: txMgr, now: time.Now}
}

// Wires the order-to-download bridge the OrderPlaced subscriber uses - optional, and without it grant-on-purchase is disabled
func (s *Service) SetOrderLookup(o OrderDownloadLookup) { s.orders = o }

// Validates the grant for token and increments its use, returning the updated grant whose URL the caller serves - row-locked, so a capped download can't be exceeded by concurrent requests
func (s *Service) Consume(ctx context.Context, token string) (Grant, error) {
	var out Grant
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		g, err := r.GetByTokenForUpdate(ctx, token)
		if err != nil {
			return err
		}
		if err := canConsume(g, s.now()); err != nil {
			return err
		}
		out, err = r.IncrementUse(ctx, g.ID)
		return err
	})
	if err != nil {
		return Grant{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Accessed{
			GrantID:    out.ID,
			CustomerID: out.CustomerID,
			OrderID:    out.OrderID,
			ProductID:  out.ProductID,
			Title:      out.Title,
		})
	}
	return out, nil
}

// A grant by token without consuming it, e.g. to render a link
func (s *Service) Get(ctx context.Context, token string) (Grant, error) {
	return s.repo.GetByToken(ctx, token)
}

func (s *Service) ListForCustomer(ctx context.Context, customerID uuid.UUID) ([]Grant, error) {
	return s.repo.ListByCustomer(ctx, customerID)
}

// The grants created for an order, for guest download access
func (s *Service) ListForOrder(ctx context.Context, orderID uuid.UUID) ([]Grant, error) {
	return s.repo.ListByOrder(ctx, orderID)
}

// Validates a grant for use - pure, with expired grants and those at their cap refused, and now as the reference time
func canConsume(g Grant, now time.Time) error {
	if g.ExpiresAt != nil && !g.ExpiresAt.After(now) {
		return ErrExpired
	}
	if g.MaxDownloads != nil && g.DownloadsUsed >= *g.MaxDownloads {
		return ErrLimitReached
	}
	return nil
}

// Mints an unguessable URL-safe access token
func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := crand.Read(b); err != nil {
		return "", fmt.Errorf("download: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
