package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Where a cart came from - an agent cart is built by a shopper's AI agent over the storefront MCP endpoint, and the agent holds its id
const (
	SourceWeb   = "web"
	SourceAgent = "agent"
)

// StatusHandedOff closes an agent cart once its link exists - the agent can't touch it again, and the link copies it out
const StatusHandedOff = "handed_off"

// HandoffPath is the storefront route a handoff link opens, the token appended
const HandoffPath = "/handoff/"

const HandoffTTL = 7 * 24 * time.Hour

var (
	ErrHandoffInvalid = errors.New("this checkout link is not valid")
	ErrHandoffExpired = errors.New("this checkout link has expired")
	ErrHandoffEmpty   = errors.New("there's nothing in this cart to check out")
)

type agentAccessKey struct{}

// WithAgentAccess marks a call as the storefront MCP tools' - an agent cart answers nobody else, so the handoff is its only way to a checkout
func WithAgentAccess(ctx context.Context) context.Context {
	return context.WithValue(ctx, agentAccessKey{}, true)
}

func agentAccess(ctx context.Context) bool {
	v, _ := ctx.Value(agentAccessKey{}).(bool)
	return v
}

type Handoff struct {
	Token     string
	ExpiresAt time.Time
}

func (s *Service) CreateAgentCart(ctx context.Context, channelID uuid.UUID) (Cart, error) {
	c, err := s.repo.CreateAgentCart(ctx, channelID)
	if err != nil {
		return Cart{}, fmt.Errorf("create agent cart: %w", err)
	}
	return c, nil
}

// StartHandoff closes an agent cart and returns it with the link for the shopper. Asking again returns the same link, since agents retry
func (s *Service) StartHandoff(ctx context.Context, cartID, channelID uuid.UUID) (Cart, error) {
	c, err := s.agentCart(ctx, cartID, channelID)
	if err != nil {
		return Cart{}, err
	}
	if c.Handoff == nil {
		if c.Status != "active" {
			return Cart{}, ErrCartNotActive
		}
		if c.Lines, err = s.repo.ListCartLinesByCart(ctx, c.ID); err != nil {
			return Cart{}, fmt.Errorf("load cart lines: %w", err)
		}
		if len(c.Lines) == 0 {
			return Cart{}, ErrHandoffEmpty
		}
		started, err := s.repo.StartHandoff(ctx, c.ID, time.Now().Add(HandoffTTL))
		switch {
		case err == nil:
			started.Lines = c.Lines
			return started, nil
		case !errors.Is(err, ErrCartNotActive):
			return Cart{}, err
		}
		if c, err = s.agentCart(ctx, cartID, channelID); err != nil { // another call closed it first, so its link is the one to give back
			return Cart{}, err
		}
		if c.Handoff == nil {
			return Cart{}, ErrCartNotActive
		}
	}
	if time.Now().After(c.Handoff.ExpiresAt) {
		return Cart{}, ErrHandoffExpired
	}
	return loadCartWithLines(ctx, s.repo, c.ID)
}

// Any other cart answers like a missing one, so the agent surface can't be pointed at a shopper's cart
func (s *Service) agentCart(ctx context.Context, cartID, channelID uuid.UUID) (Cart, error) {
	c, err := s.repo.GetCartByID(ctx, cartID)
	if err != nil {
		if IsSQLNoRows(err) {
			return Cart{}, ErrCartForbidden
		}
		return Cart{}, fmt.Errorf("load agent cart: %w", err)
	}
	if c.Source != SourceAgent || c.Kind != KindShopper || c.CustomerID != nil || c.ChannelID != channelID {
		return Cart{}, ErrCartForbidden
	}
	return c, nil
}

// AgentCart is an agent cart with its lines, refused once it's handed off like any closed cart
func (s *Service) AgentCart(ctx context.Context, cartID, channelID uuid.UUID) (Cart, error) {
	c, err := s.agentCart(ctx, cartID, channelID)
	if err != nil {
		return Cart{}, err
	}
	if c.Status != "active" {
		return Cart{}, ErrCartNotActive
	}
	return loadCartWithLines(ctx, s.repo, c.ID)
}

// RedeemHandoff copies a handed-off agent cart into the shopper's own cart and returns it - the signed-in customer's, else the guest cart named by into, else a new one
// Redeeming the same link into the same cart again copies nothing, so a reload or a link scanner can't double the order
func (s *Service) RedeemHandoff(ctx context.Context, token string, channelID uuid.UUID, customerID, into *uuid.UUID) (Cart, error) {
	if !isLinkToken(token) {
		return Cart{}, ErrHandoffInvalid
	}
	agent, err := s.repo.GetCartByHandoffToken(ctx, token)
	if err != nil {
		if IsSQLNoRows(err) {
			return Cart{}, ErrHandoffInvalid
		}
		return Cart{}, err
	}
	if agent.ChannelID != channelID || agent.Status != StatusHandedOff || agent.Handoff == nil {
		return Cart{}, ErrHandoffInvalid
	}
	if time.Now().After(agent.Handoff.ExpiresAt) {
		return Cart{}, ErrHandoffExpired
	}
	target, err := s.handoffTarget(ctx, channelID, customerID, into)
	if err != nil {
		return Cart{}, err
	}
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		locked, err := r.LockCart(ctx, target.ID)
		if err != nil {
			return fmt.Errorf("lock the shopper's cart: %w", err)
		}
		if locked.Status != "active" { // checked out or merged since it was picked - the shopper's next try picks another
			return ErrCartNotActive
		}
		fresh, err := r.RecordHandoffRedemption(ctx, agent.ID, target.ID)
		if err != nil || !fresh {
			return err
		}
		lines, err := r.ListCartLinesByCart(ctx, agent.ID)
		if err != nil {
			return fmt.Errorf("load agent cart lines: %w", err)
		}
		if err := copyLines(ctx, r, target.ID, lines); err != nil {
			return err
		}
		return r.CarryAppliedCodes(ctx, agent.ID, target.ID)
	})
	if err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, target.ID)
}

// Outside the redemption's transaction - a customer cart created concurrently by another tab is a unique violation, which would abort it
func (s *Service) handoffTarget(ctx context.Context, channelID uuid.UUID, customerID, into *uuid.UUID) (Cart, error) {
	if customerID != nil {
		return GetOrCreateCustomerCart(ctx, s.repo, *customerID, channelID)
	}
	if into != nil {
		c, err := s.repo.GetCartByID(ctx, *into)
		if err != nil && !IsSQLNoRows(err) {
			return Cart{}, fmt.Errorf("load the shopper's cart: %w", err)
		}
		if err == nil && c.Status == "active" && c.Kind == KindShopper && c.Source == SourceWeb && c.CustomerID == nil && c.ChannelID == channelID {
			return c, nil
		}
	}
	return s.CreateGuestCart(ctx, channelID)
}
