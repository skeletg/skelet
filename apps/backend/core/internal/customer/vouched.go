package customer

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (s *Service) FindForSignIn(ctx context.Context, channelID uuid.UUID, email string) (Customer, error) {
	return s.repo.GetByStoreAndEmail(ctx, channelID, NormalizeEmail(email))
}

// RegisterVouched opens an account with a verified address and no password, registration subscribers still having their say
func (s *Service) RegisterVouched(ctx context.Context, channelID uuid.UUID, emailAddr, firstName, lastName string) (Customer, error) {
	emailAddr = NormalizeEmail(emailAddr)
	if err := validateEmail(emailAddr); err != nil {
		return Customer{}, err
	}
	if err := validateNames(firstName, lastName); err != nil {
		return Customer{}, err
	}
	ing := &CustomerRegistering{ChannelID: channelID, Email: emailAddr, FirstName: firstName, LastName: lastName}
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Customer{}, fmt.Errorf("registration blocked: %w", err)
		}
	}
	group := s.registeringGroup(ctx, ing)
	var c Customer
	err := s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		c, err = r.Create(ctx, CreateCustomerInput{
			ChannelID: channelID, Email: emailAddr, FirstName: firstName, LastName: lastName,
			GroupID: group, Locale: locale.FromContext(ctx), EmailVerified: true,
		})
		if err != nil {
			return nil, err
		}
		return []events.Event{&CustomerRegistered{
			CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, LastName: c.LastName,
			Source: SourceSSO, Tags: ing.Tags, Shopper: auth.ShopperFromContext(ctx),
		}}, nil
	})
	return c, err
}

// AttemptVouched is a provider sign-in's turn at the login veto, raised before any account is opened or claimed for it
func (s *Service) AttemptVouched(ctx context.Context, channelID uuid.UUID, email string) error {
	if s.bus == nil {
		return nil
	}
	ip, ua := authAttribution(ctx)
	email = NormalizeEmail(email)
	attempting := &auth.LoginAttempting{Subject: auth.SubjectCustomer, Email: email, IP: ip, UserAgent: ua, ChannelID: channelID}
	if err := s.bus.PublishSync(ctx, attempting); err != nil {
		s.publishLoginFailed(ctx, email, ip, ua, channelID, "rejected_by_subscriber")
		return fmt.Errorf("login rejected: %w", err)
	}
	return nil
}

// SignInVouched finishes a provider sign-in AttemptVouched let through
func (s *Service) SignInVouched(ctx context.Context, c Customer) error {
	ip, ua := authAttribution(ctx)
	if !c.IsActive {
		s.publishLoginFailed(ctx, c.Email, ip, ua, c.ChannelID, "account_disabled")
		return ErrAccountDisabled
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &auth.LoginSucceeded{Subject: auth.SubjectCustomer, SubjectID: c.ID, Email: c.Email, IP: ip, UserAgent: ua,
			ChannelID: c.ChannelID, Shopper: auth.ShopperFromContext(ctx)})
	}
	return nil
}

// ClaimVouched hands an unverified account to whoever the provider proved owns its address: a password someone set
// without proving it goes, and every session with it
func (s *Service) ClaimVouched(ctx context.Context, c Customer) (Customer, error) {
	if c.IsEmailVerified {
		return c, nil
	}
	err := s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		if err := r.UpdatePassword(ctx, c.ID, ""); err != nil {
			return nil, err
		}
		if err := r.MarkEmailVerified(ctx, c.ID, c.Email); err != nil {
			return nil, err
		}
		return []events.Event{&CustomerEmailVerified{CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email}}, nil
	})
	if err != nil {
		return Customer{}, err
	}
	if s.customerRefresh != nil {
		if err := s.customerRefresh.RevokeAll(ctx, c.ID); err != nil {
			return Customer{}, fmt.Errorf("end the account's other sessions: %w", err)
		}
	}
	s.revokeResetLinks(ctx, c.ID)
	c.IsEmailVerified, c.PasswordHash = true, ""
	return c, nil
}
