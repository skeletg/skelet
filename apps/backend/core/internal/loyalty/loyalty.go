// Package loyalty is a per-customer points program: points are earned on placed orders (merchandise value x a configured per-channel rate), revoked if the order is cancelled, and redeemed by converting into store credit - which checkout already spends as a payment tender, so redemption needs no extra money-path plumbing - The ledger is append-only (earns positive, revokes/redemptions negative); balance is the signed sum
package loyalty

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

var (
	// ErrDisabled rejects redemption when the program is off for the channel
	ErrDisabled = apperr.New("LOYALTY_DISABLED", apperr.ClassValidation, "the loyalty program is not enabled")
	// ErrInvalidPoints rejects zero/negative redemptions and zero adjustments
	ErrInvalidPoints = apperr.New("LOYALTY_INVALID_POINTS", apperr.ClassValidation, "points must be a positive amount")
	// ErrInsufficientPoints rejects spending more points than the balance
	ErrInsufficientPoints = apperr.New("LOYALTY_INSUFFICIENT", apperr.ClassValidation, "not enough loyalty points")
)

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "loyalty", Label: "Loyalty Program", SortOrder: 4, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: "sales/loyalty/enabled", Type: config.TypeBool, Default: "false", Label: "Loyalty program enabled", Public: true})
	config.Register(config.Definition{Path: "sales/loyalty/earn_points_per_unit", Type: config.TypeInt, Default: "1", Label: "Points earned per currency unit of merchandise value", Public: true})
	config.Register(config.Definition{Path: "sales/loyalty/redeem_cents_per_point", Type: config.TypeInt, Default: "1", Label: "Cents of store credit per redeemed point", Public: true})
}

type Entry struct {
	ID         uuid.UUID
	CustomerID uuid.UUID
	OrderID    *uuid.UUID
	Points     int64
	Reason     string
	CreatedAt  time.Time
}

// Program is the channel-resolved configuration snapshot
type Program struct {
	Enabled             bool
	EarnPointsPerUnit   int64
	RedeemCentsPerPoint int64
}

type Service struct {
	queries *db.Queries
	tx      *txrunner.Manager
	cfg     *config.Service
	credit  *storecredit.Service
	orders  OrderLookup
}

func NewService(queries *db.Queries, txMgr *txrunner.Manager, cfg *config.Service, credit *storecredit.Service) *Service {
	return &Service{queries: queries, tx: txMgr, cfg: cfg, credit: credit}
}

// ProgramFor resolves the channel-scoped program configuration
func (s *Service) ProgramFor(ctx context.Context, channelID uuid.UUID) (Program, error) {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	enabled, err := s.cfg.GetBool(ctx, "sales/loyalty/enabled", scope)
	if err != nil {
		return Program{}, fmt.Errorf("loyalty enabled config: %w", err)
	}
	earn, err := s.cfg.GetInt(ctx, "sales/loyalty/earn_points_per_unit", scope)
	if err != nil {
		return Program{}, fmt.Errorf("loyalty earn config: %w", err)
	}
	redeem, err := s.cfg.GetInt(ctx, "sales/loyalty/redeem_cents_per_point", scope)
	if err != nil {
		return Program{}, fmt.Errorf("loyalty redeem config: %w", err)
	}
	return Program{Enabled: enabled, EarnPointsPerUnit: int64(earn), RedeemCentsPerPoint: int64(redeem)}, nil
}

func (s *Service) Balance(ctx context.Context, customerID uuid.UUID) (int64, error) {
	bal, err := s.queries.LoyaltyBalance(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("loyalty balance: %w", err)
	}
	return bal, nil
}

func (s *Service) Entries(ctx context.Context, customerID uuid.UUID, limit int32) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	rows, err := s.queries.ListLoyaltyEntries(ctx, db.ListLoyaltyEntriesParams{
		CustomerID: customerID,
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list loyalty entries: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEntry(r))
	}
	return out, nil
}

// Adjust applies a signed admin correction - Deductions are capped at the
// current balance
func (s *Service) Adjust(ctx context.Context, customerID uuid.UUID, points int64, reason string) (Entry, error) {
	if points == 0 {
		return Entry{}, ErrInvalidPoints
	}
	var entry Entry
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		if err := q.LockCustomerForLoyalty(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		if points < 0 {
			bal, err := q.LoyaltyBalance(ctx, customerID)
			if err != nil {
				return fmt.Errorf("loyalty balance: %w", err)
			}
			if bal+points < 0 {
				return ErrInsufficientPoints
			}
		}
		row, err := q.InsertLoyaltyEntry(ctx, db.InsertLoyaltyEntryParams{
			CustomerID: customerID,
			Points:     points,
			Reason:     reason,
		})
		if err != nil {
			return fmt.Errorf("insert loyalty entry: %w", err)
		}
		entry = toEntry(row)
		return nil
	})
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Redeem converts points into store credit at the channel's configured rate, atomically: the points debit and the
// credit grant land in one transaction under the customer lock - Returns the cents granted
func (s *Service) Redeem(ctx context.Context, customerID uuid.UUID, points int64, channelID uuid.UUID, currency string) (int64, error) {
	if points <= 0 {
		return 0, ErrInvalidPoints
	}
	prog, err := s.ProgramFor(ctx, channelID)
	if err != nil {
		return 0, err
	}
	if !prog.Enabled {
		return 0, ErrDisabled
	}
	cents := points * prog.RedeemCentsPerPoint
	if cents <= 0 {
		return 0, ErrInvalidPoints
	}
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		if err := q.LockCustomerForLoyalty(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		bal, err := q.LoyaltyBalance(ctx, customerID)
		if err != nil {
			return fmt.Errorf("loyalty balance: %w", err)
		}
		if bal < points {
			return ErrInsufficientPoints
		}
		if _, err := q.InsertLoyaltyEntry(ctx, db.InsertLoyaltyEntryParams{
			CustomerID: customerID,
			Points:     -points,
			Reason:     "redeemed for store credit",
		}); err != nil {
			return fmt.Errorf("insert redemption entry: %w", err)
		}
		return s.credit.GrantTx(ctx, t, customerID, cents, currency, fmt.Sprintf("loyalty redemption (%d points)", points))
	})
	if err != nil {
		return 0, err
	}
	return cents, nil
}

func toEntry(r db.LoyaltyPointEntry) Entry {
	e := Entry{
		ID:         r.ID,
		CustomerID: r.CustomerID,
		Points:     r.Points,
		Reason:     r.Reason,
		CreatedAt:  r.CreatedAt,
	}
	if r.OrderID.Valid {
		id := r.OrderID.UUID
		e.OrderID = &id
	}
	return e
}
