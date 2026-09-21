// Package referral is a give-get program - every customer can mint one share code, a new customer registering with it becomes a pending referral, and when that referee places their first order both sides are rewarded with store credit
// Rewards ride the existing store-credit tender rails, so there are no new money paths
package referral

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "referrals", Label: "Referral Program", SortOrder: 5, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: "sales/referrals/enabled", Type: config.TypeBool, Default: "false", Label: "Referral program enabled", Public: true})
	config.Register(config.Definition{Path: "sales/referrals/referrer_reward_cents", Type: config.TypeInt, Default: "1000", Label: "Store credit for the referrer when the referee first orders (cents)", Public: true})
	config.Register(config.Definition{Path: "sales/referrals/referee_reward_cents", Type: config.TypeInt, Default: "500", Label: "Store credit for the referee on their first order (cents)", Public: true})
}

// Omits easily-confused symbols - 0/O, 1/I/L
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

type Program struct {
	Enabled             bool
	ReferrerRewardCents int64
	RefereeRewardCents  int64
}

type Stats struct {
	Referred    int64
	Rewarded    int64
	EarnedCents int64
}

// OrderLookup loads an order so the cancellation reversal can read the currency the reward was granted in - satisfied by order.Service
type OrderLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
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

// Optional - without it a cancelled order can't be reversed, since the reward currency is unknown
func (s *Service) SetOrderLookup(o OrderLookup) { s.orders = o }

func (s *Service) ProgramFor(ctx context.Context, channelID uuid.UUID) (Program, error) {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	enabled, err := s.cfg.GetBool(ctx, "sales/referrals/enabled", scope)
	if err != nil {
		return Program{}, fmt.Errorf("referrals enabled config: %w", err)
	}
	referrer, err := s.cfg.GetInt(ctx, "sales/referrals/referrer_reward_cents", scope)
	if err != nil {
		return Program{}, fmt.Errorf("referrer reward config: %w", err)
	}
	referee, err := s.cfg.GetInt(ctx, "sales/referrals/referee_reward_cents", scope)
	if err != nil {
		return Program{}, fmt.Errorf("referee reward config: %w", err)
	}
	return Program{Enabled: enabled, ReferrerRewardCents: int64(referrer), RefereeRewardCents: int64(referee)}, nil
}

// The customer's share code, minting one on first use
func (s *Service) CodeFor(ctx context.Context, customerID uuid.UUID) (string, error) {
	existing, err := s.queries.GetReferralCodeByCustomer(ctx, customerID)
	if err == nil {
		return existing.Code, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("get referral code: %w", err)
	}
	for range 5 {
		code, err := newCode()
		if err != nil {
			return "", err
		}
		row, err := s.queries.InsertReferralCode(ctx, db.InsertReferralCodeParams{
			CustomerID: customerID,
			Code:       code,
		})
		if err == nil {
			return row.Code, nil
		}
		// Retry only on the astronomically rare code collision - an upsert conflict on customer_id returns the existing row instead
	}
	return "", fmt.Errorf("referral: could not generate a unique code")
}

// Attach links a just-registered referee to the referrer who owns the presented code - Best-effort semantics for the register flow: invalid
// or self-referencing codes report an error the caller may log and ignore - a referee can only ever be attached once
func (s *Service) Attach(ctx context.Context, refereeID uuid.UUID, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil
	}
	owner, err := s.queries.GetReferralCodeByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unknown referral code")
		}
		return fmt.Errorf("look up referral code: %w", err)
	}
	if owner.CustomerID == refereeID {
		return fmt.Errorf("self-referral ignored")
	}
	if _, err := s.queries.InsertReferral(ctx, db.InsertReferralParams{
		ReferrerCustomerID: owner.CustomerID,
		RefereeCustomerID:  refereeID,
		Code:               code,
	}); err != nil {
		return fmt.Errorf("insert referral: %w", err)
	}
	return nil
}

// StatsFor summarises the customer's referrals
func (s *Service) StatsFor(ctx context.Context, customerID uuid.UUID) (Stats, error) {
	row, err := s.queries.ReferralStats(ctx, customerID)
	if err != nil {
		return Stats{}, fmt.Errorf("referral stats: %w", err)
	}
	return Stats{Referred: row.Referred, Rewarded: row.Rewarded, EarnedCents: row.EarnedCents}, nil
}

func newCode() (string, error) {
	raw := make([]byte, 8)
	if _, err := crand.Read(raw); err != nil {
		return "", fmt.Errorf("referral: read random: %w", err)
	}
	out := make([]byte, 0, 8)
	for _, b := range raw {
		out = append(out, codeAlphabet[int(b)%len(codeAlphabet)])
	}
	return string(out), nil
}
