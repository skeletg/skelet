package customer

// GDPR data-subject tooling - full data export for a DSAR, plus anonymize-in-place for customers with order history, since orders are retained for tax-law reasons with their PII columns scrubbed

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

// The DSAR bundle - orders are appended by the caller, which owns order access, so this struct just frames the JSON
type DataExport struct {
	ExportedAt time.Time       `json:"exportedAt"`
	Profile    Customer        `json:"profile"`
	Addresses  []Address       `json:"addresses"`
	Consents   json.RawMessage `json:"consents,omitempty"`
	Orders     json.RawMessage `json:"orders,omitempty"`
	Extra      map[string]any  `json:"extra,omitempty"`
}

// Assembles the customer-owned slice of the bundle, profile and addresses - callers append orders and consents from their own services and marshal the result
func (s *Service) ExportData(ctx context.Context, customerID uuid.UUID) (DataExport, error) {
	c, err := s.repo.GetByIDWithAddresses(ctx, customerID)
	if err != nil {
		return DataExport{}, err
	}
	// Never export the hash
	c.PasswordHash = ""
	return DataExport{
		ExportedAt: time.Now().UTC(),
		Profile:    c,
		Addresses:  c.Addresses,
	}, nil
}

// Scrubs every personal field while keeping the customer row as an FK anchor and the financial order records - irreversible, and every session is revoked so the account can never sign in again
func (s *Service) AnonymizeInPlace(ctx context.Context, customerID uuid.UUID) error {
	c, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword(buf[:], bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.Anonymize(ctx, customerID, c.Email, string(hash)); err != nil {
		return err
	}
	if s.customerRefresh != nil {
		if err := s.customerRefresh.RevokeAll(ctx, customerID); err != nil {
			return fmt.Errorf("anonymized but session revocation failed: %w", err)
		}
	}
	return nil
}

// PurgeSignalsIdentity clears the analytics side for a HARD delete - the commerce row cascades but Signals lives in its own database with no FK, so without this a deleted customer's email and identity mappings survive indefinitely
func (r *postgresRepository) PurgeSignalsIdentity(ctx context.Context, customerID uuid.UUID, email string) error {
	if r.signals == nil {
		return nil
	}
	if err := r.signals.ScrubCustomerEmailSends(ctx, customerID, email); err != nil {
		return fmt.Errorf("scrub email sends: %w", err)
	}
	if err := r.signals.DeleteCustomerIdentities(ctx, customerID); err != nil {
		return fmt.Errorf("delete analytics identities: %w", err)
	}
	return nil
}

// Signals can't ride the commerce tx - the send scrub goes first (its guest-era match needs the pre-scrub address), the identity delete after (a leftover only re-links until the retry)
func (r *postgresRepository) Anonymize(ctx context.Context, customerID uuid.UUID, email, passwordHash string) error {
	if r.signals != nil {
		if err := r.signals.ScrubCustomerEmailSends(ctx, customerID, email); err != nil {
			return fmt.Errorf("scrub email sends: %w", err)
		}
	}
	if err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.AnonymizeCustomerRow(ctx, db.AnonymizeCustomerRowParams{ID: customerID, PasswordHash: passwordHash}); err != nil {
			return fmt.Errorf("anonymize customer row: %w", err)
		}
		if err := q.DeleteCustomerAddresses(ctx, customerID); err != nil {
			return fmt.Errorf("delete addresses: %w", err)
		}
		if _, err := q.ScrubCustomerOrders(ctx, uuid.NullUUID{UUID: customerID, Valid: true}); err != nil {
			return fmt.Errorf("scrub orders: %w", err)
		}
		if err := q.DeleteCustomerWishlist(ctx, customerID); err != nil {
			return fmt.Errorf("delete wishlist: %w", err)
		}
		// The three places an erased customer's address kept living: the cart
		// recovery flow, the back-in-stock notifier and the year-long audit log
		if _, err := q.ScrubCustomerCarts(ctx, uuid.NullUUID{UUID: customerID, Valid: true}); err != nil {
			return fmt.Errorf("scrub carts: %w", err)
		}
		if _, err := q.DeleteCustomerBackInStockRequests(ctx, email); err != nil {
			return fmt.Errorf("delete back-in-stock requests: %w", err)
		}
		if _, err := q.ScrubCustomerAuditDetail(ctx, email); err != nil {
			return fmt.Errorf("scrub audit detail: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	if r.signals != nil {
		if err := r.signals.DeleteCustomerIdentities(ctx, customerID); err != nil {
			return fmt.Errorf("delete analytics identities: %w", err)
		}
	}
	return nil
}
