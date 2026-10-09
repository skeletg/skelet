package customer

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// The actions customer.address_changed carries
const (
	AddressAdded           = "add"
	AddressUpdated         = "update"
	AddressDeleted         = "delete"
	AddressDefaultBilling  = "default_billing"
	AddressDefaultShipping = "default_shipping"
)

// Country codes are stored upper-case, so a pasted "de " validates and lands the same as "DE"
func normalizeAddress(in AddressInput) AddressInput {
	in.CountryCode = strings.ToUpper(strings.TrimSpace(in.CountryCode))
	return in
}

func (s *Service) AddAddress(ctx context.Context, customerID uuid.UUID, input AddressInput) (Address, error) {
	input = normalizeAddress(input)
	if err := validateAddressInput(input); err != nil {
		return Address{}, err
	}
	if n, err := s.repo.CountAddresses(ctx, customerID); err != nil {
		return Address{}, err
	} else if n >= MaxAddressesPerCustomer {
		return Address{}, ErrTooManyAddresses
	}
	if err := s.validateAddressCountry(ctx, customerID, input); err != nil {
		return Address{}, err
	}
	addr, err := s.repo.CreateAddress(ctx, customerID, input)
	if err != nil {
		return Address{}, err
	}
	s.publishAddressChanged(ctx, customerID, addr.ID, AddressAdded)
	return addr, nil
}

// Best-effort - a channel lookup failure just drops the analytics-only event
func (s *Service) publishAddressChanged(ctx context.Context, customerID, addressID uuid.UUID, action string) {
	if s.bus == nil {
		return
	}
	cust, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return
	}
	s.bus.Publish(ctx, &CustomerAddressChanged{CustomerID: customerID, ChannelID: cust.ChannelID, AddressID: addressID, Action: action})
}

// Another customer's address reads as not found - the write is scoped to the owner in SQL as well
func (s *Service) UpdateAddress(ctx context.Context, customerID, addressID uuid.UUID, input AddressInput) (Address, error) {
	input = normalizeAddress(input)
	if err := validateAddressInput(input); err != nil {
		return Address{}, err
	}
	if err := s.validateAddressCountry(ctx, customerID, input); err != nil {
		return Address{}, err
	}
	updated, err := s.repo.UpdateAddress(ctx, customerID, addressID, input)
	if err != nil {
		return Address{}, err
	}
	s.publishAddressChanged(ctx, customerID, addressID, AddressUpdated)
	return updated, nil
}

// Scoped to the customer's own channel - no-op when no validator is wired
func (s *Service) validateAddressCountry(ctx context.Context, customerID uuid.UUID, input AddressInput) error {
	if s.countries == nil {
		return nil
	}
	cust, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	return s.countries.ValidateAddressForChannel(ctx, cust.ChannelID, input.CountryCode, input.State)
}

func (s *Service) DeleteAddress(ctx context.Context, customerID, addressID uuid.UUID) error {
	if err := s.repo.DeleteAddress(ctx, customerID, addressID); err != nil {
		return err
	}
	s.publishAddressChanged(ctx, customerID, addressID, AddressDeleted)
	return nil
}

// Returns the row regardless of owner - admin resolvers need it to find the owning customer before running the channel-scoped permission check
func (s *Service) GetAddressByID(ctx context.Context, id uuid.UUID) (Address, error) {
	return s.repo.GetAddress(ctx, id)
}

// Admin only - the resolver has checked the owner's channel, and the delete itself is still scoped to that owner
func (s *Service) AdminDeleteAddress(ctx context.Context, id uuid.UUID) error {
	addr, err := s.repo.GetAddress(ctx, id)
	if err != nil {
		return err
	}
	return s.DeleteAddress(ctx, addr.CustomerID, id)
}

func (s *Service) SetDefaultBillingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	c, err := s.repo.SetDefaultBillingAddress(ctx, customerID, addressID)
	if err != nil {
		return Customer{}, err
	}
	s.publishAddressChanged(ctx, customerID, addressID, AddressDefaultBilling)
	return c, nil
}

func (s *Service) SetDefaultShippingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	c, err := s.repo.SetDefaultShippingAddress(ctx, customerID, addressID)
	if err != nil {
		return Customer{}, err
	}
	s.publishAddressChanged(ctx, customerID, addressID, AddressDefaultShipping)
	return c, nil
}
