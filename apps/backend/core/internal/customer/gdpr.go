package customer

// GDPR data-subject tooling - full data export for a DSAR, plus anonymize-in-place for customers with order history, since orders are retained for tax-law reasons with their PII columns scrubbed

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
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
// Revocation runs whatever else failed, since the admin asked for the account to be unusable
func (s *Service) AnonymizeInPlace(ctx context.Context, customerID uuid.UUID) error {
	c, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	err = s.anonymize(ctx, c)
	if s.customerRefresh != nil {
		if rerr := s.customerRefresh.RevokeAll(ctx, customerID); rerr != nil {
			err = errors.Join(err, fmt.Errorf("session revocation failed: %w", rerr))
		}
	}
	return err
}

// The suite goes first, while the real address still exists to name the sends from before the account by - a failure there redacts nothing, so a retry has all it needs
func (s *Service) anonymize(ctx context.Context, c Customer) error {
	if err := s.repo.PurgeSignalsIdentity(ctx, c.ID, c.Email); err != nil {
		return err
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword(buf[:], auth.HashCost)
	if err != nil {
		return err
	}
	// The roster push hangs off this, so the suite's copy drops the address and consent now rather than at the next sweep
	updated := &CustomerUpdated{
		CustomerID: c.ID, ChannelID: c.ChannelID,
		Email: "redacted-" + c.ID.String() + "@anonymized.invalid", FirstName: "Redacted",
		ChangedFields: []string{FieldEmail, FieldFirstName, FieldLastName, FieldPhone, FieldIsActive, FieldIsEmailVerified, FieldNote, FieldMetadata},
		Source:        SourceGDPR,
	}
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		if err := r.Anonymize(ctx, c, string(hash)); err != nil {
			return nil, err
		}
		return []events.Event{updated}, nil
	})
	if err != nil {
		return err
	}
	s.revokeEmailLinks(ctx, c.ID)
	return nil
}

// PurgeSignalsIdentity clears what a HARD delete leaves behind in the suite's database - the send log
// and the identity mappings both carry the address, and neither has an FK to cascade on
func (r *postgresRepository) PurgeSignalsIdentity(ctx context.Context, customerID uuid.UUID, email string) error {
	if r.signals == nil {
		return nil
	}
	erasedAt, err := r.queries.CommerceClock(ctx)
	if err != nil {
		return fmt.Errorf("read the erase time: %w", err)
	}
	if err := r.signals.DeleteCustomerIdentities(ctx, customerID, email, erasedAt); err != nil {
		return fmt.Errorf("erase signals identity: %w", err)
	}
	return nil
}

// Several statements, so it runs on a transaction-scoped repository - see Service.atomically
func (r *postgresRepository) Anonymize(ctx context.Context, c Customer, passwordHash string) error {
	if err := scrubPersonalData(ctx, r.queries, c); err != nil {
		return err
	}
	if err := r.queries.AnonymizeCustomerRow(ctx, db.AnonymizeCustomerRowParams{ID: c.ID, PasswordHash: passwordHash}); err != nil {
		return fmt.Errorf("anonymize customer row: %w", err)
	}
	if err := r.queries.DeleteCustomerAddresses(ctx, c.ID); err != nil {
		return fmt.Errorf("delete addresses: %w", err)
	}
	return nil
}

// Everything outside the customer row that names the customer, by id or by address on their channel - shared by
// erasure and hard delete, since the rows keyed by address or SET NULL on delete outlive the account either way
func scrubPersonalData(ctx context.Context, q *db.Queries, c Customer) error {
	id, ch, addr := c.ID, c.ChannelID, NormalizeEmail(c.Email)
	nid := uuid.NullUUID{UUID: id, Valid: true}
	steps := []struct {
		what string
		run  func() error
	}{
		// First, while the guest orders still carry the address the audit rows are matched through
		{"scrub audit detail", func() error {
			_, err := q.ScrubCustomerAuditDetail(ctx, db.ScrubCustomerAuditDetailParams{Email: addr, CustomerID: id, ChannelID: ch})
			return err
		}},
		{"scrub orders", func() error { _, err := q.ScrubCustomerOrders(ctx, nid); return err }},
		{"scrub guest orders", func() error {
			_, err := q.ScrubGuestOrdersByEmail(ctx, db.ScrubGuestOrdersByEmailParams{CustomerID: id, ChannelID: ch, Email: addr})
			return err
		}},
		{"delete wishlist", func() error { return q.DeleteCustomerWishlist(ctx, id) }},
		{"delete wishlist shares", func() error { _, err := q.DeleteCustomerWishlistShares(ctx, id); return err }},
		// carts.email makes an abandoned cart addressable by the recovery flow
		{"scrub carts", func() error { _, err := q.ScrubCustomerCarts(ctx, nid); return err }},
		{"delete back-in-stock requests", func() error {
			_, err := q.DeleteCustomerBackInStockRequests(ctx, db.DeleteCustomerBackInStockRequestsParams{ChannelID: ch, CustomerID: nid, Email: addr})
			return err
		}},
		{"scrub product questions", func() error {
			_, err := q.ScrubCustomerProductQuestions(ctx, db.ScrubCustomerProductQuestionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
			return err
		}},
		{"delete signup submissions", func() error {
			_, err := q.DeleteCustomerSignupSubmissions(ctx, db.DeleteCustomerSignupSubmissionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
			return err
		}},
		{"delete web form submissions", func() error {
			_, err := q.DeleteCustomerWebFormSubmissions(ctx, db.DeleteCustomerWebFormSubmissionsParams{CustomerID: nid, ChannelID: ch, Email: addr})
			return err
		}},
		{"scrub download grants", func() error {
			_, err := q.ScrubCustomerDownloadGrants(ctx, db.ScrubCustomerDownloadGrantsParams{CustomerID: nid, ChannelID: ch, Email: addr})
			return err
		}},
		{"scrub gift cards", func() error {
			_, err := q.ScrubCustomerGiftCards(ctx, db.ScrubCustomerGiftCardsParams{CustomerID: nid, ChannelID: uuid.NullUUID{UUID: ch, Valid: true}, Email: addr})
			return err
		}},
		{"delete payment methods", func() error { _, err := q.DeleteCustomerPaymentMethods(ctx, id); return err }},
		{"delete consents", func() error { return q.DeleteCustomerConsents(ctx, id) }},
		{"delete consent history", func() error { return q.DeleteCustomerConsentEvents(ctx, id) }},
		{"delete signup confirmations", func() error {
			return q.DeleteCustomerSignupConfirmations(ctx, db.DeleteCustomerSignupConfirmationsParams{CustomerID: id, Email: c.Email})
		}},
	}
	for _, st := range steps {
		if err := st.run(); err != nil {
			return fmt.Errorf("%s: %w", st.what, err)
		}
	}
	return nil
}
