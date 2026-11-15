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

func (s *Service) SignInVouched(ctx context.Context, c Customer) error {
	ip, ua := authAttribution(ctx)
	if s.bus != nil {
		attempting := &auth.LoginAttempting{Subject: auth.SubjectCustomer, Email: c.Email, IP: ip, UserAgent: ua, ChannelID: c.ChannelID}
		if err := s.bus.PublishSync(ctx, attempting); err != nil {
			s.publishLoginFailed(ctx, c.Email, ip, ua, c.ChannelID, "rejected_by_subscriber")
			return fmt.Errorf("login rejected: %w", err)
		}
	}
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
