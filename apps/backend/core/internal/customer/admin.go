package customer

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The admin list's page size when the caller names none, and the most it may ask for
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// ListPage is the server-paged admin list - see ListPageFilter
func (s *Service) ListPage(ctx context.Context, f ListPageFilter) ([]Customer, int64, error) {
	if f.ChannelIDs != nil && len(f.ChannelIDs) == 0 {
		return nil, 0, nil
	}
	if f.Limit <= 0 || f.Limit > MaxListLimit {
		f.Limit = DefaultListLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.Search = escapeLike(strings.TrimSpace(f.Search))
	return s.repo.ListPage(ctx, f)
}

// The search is matched with ILIKE, where a typed % or _ would otherwise match anything
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// nil channelIDs is every channel, an empty slice none
func (s *Service) ListInChannels(ctx context.Context, channelIDs []uuid.UUID) ([]Customer, error) {
	if channelIDs != nil && len(channelIDs) == 0 {
		return nil, nil
	}
	return s.repo.ListInChannels(ctx, channelIDs)
}

// Password is admin-supplied and bcrypted here, and the customer should change it after first login
// No verification email - the admin vouched for the address by setting is_email_verified=true, which is the default
func (s *Service) AdminCreateCustomer(ctx context.Context, input AdminCreateCustomerInput) (Customer, error) {
	input.Email = NormalizeEmail(input.Email)
	if err := validateEmail(input.Email); err != nil {
		return Customer{}, err
	}
	if err := validateNames(input.FirstName, input.LastName); err != nil {
		return Customer{}, err
	}
	if err := checkOptLen("phone", input.Phone, MaxPhoneRunes); err != nil {
		return Customer{}, err
	}
	if err := auth.ValidatePasswordStrength(input.Password); err != nil {
		return Customer{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), auth.HashCost)
	if err != nil {
		return Customer{}, fmt.Errorf("hash password: %w", err)
	}
	group, err := s.groupForSlug(ctx, input.CustomerGroupSlug)
	if err != nil {
		return Customer{}, err
	}

	// Normalized copy so we don't mutate the caller's input
	normalized := input
	if normalized.Phone, err = s.NormalizeProfilePhone(ctx, input.ChannelID, input.Phone); err != nil {
		return Customer{}, err
	}
	if normalized.IsActive == nil {
		t := true
		normalized.IsActive = &t
	}
	if normalized.IsEmailVerified == nil {
		t := true
		normalized.IsEmailVerified = &t
	}

	// Admin-created accounts skip the pre-commit fraud chain because the admin vouched for the address
	var c Customer
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		if c, err = r.AdminCreate(ctx, normalized, string(hash), group.ID); err != nil {
			return nil, err
		}
		return []events.Event{&CustomerRegistered{
			CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, LastName: c.LastName, Source: SourceAdmin,
		}}, nil
	})
	return c, err
}

// An empty slug is the default group
func (s *Service) groupForSlug(ctx context.Context, slug string) (CustomerGroup, error) {
	if slug == "" {
		return s.repo.GetDefaultGroup(ctx)
	}
	return s.repo.GetGroupBySlug(ctx, slug)
}

// Nil input fields are left unchanged, and email uniqueness comes from the DB with the repo surfacing ErrEmailTaken
// Changing email does NOT flip is_email_verified - the admin passes isEmailVerified: false in the same call to force a re-verify
func (s *Service) AdminUpdateCustomer(ctx context.Context, id uuid.UUID, input AdminUpdateCustomerInput) (Customer, error) {
	if input.Email != nil {
		// Normalized like every other write - an admin pasting " Jane@Acme.com " from a ticket would otherwise store an address that can never match at login
		n := NormalizeEmail(*input.Email)
		input.Email = &n
		if err := validateEmail(n); err != nil {
			return Customer{}, err
		}
	}
	if err := validateAdminUpdate(input); err != nil {
		return Customer{}, err
	}
	changed := adminChangedFields(input)

	// Load the customer first so events carry its store - subscribers filter on that
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	if input.Phone, err = s.NormalizeProfilePhone(ctx, existing.ChannelID, input.Phone); err != nil {
		return Customer{}, err
	}
	updatingEvt, err := s.vetoUpdate(ctx, existing, changed)
	if err != nil {
		return Customer{}, err
	}

	var groupID *uuid.UUID
	if input.CustomerGroupSlug != nil {
		g, err := s.groupForSlug(ctx, *input.CustomerGroupSlug)
		if err != nil {
			return Customer{}, err
		}
		groupID = &g.ID
	}

	var before, c Customer
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		if before, c, err = r.AdminUpdate(ctx, id, input, groupID); err != nil {
			return nil, err
		}
		return []events.Event{updatedEvent(c, changed, updatingEvt.Tags)}, nil
	})
	if err != nil {
		return Customer{}, err
	}
	s.afterAdminUpdate(ctx, before, c)
	return c, nil
}

// Built before any mutation so subscribers know what the admin touched
func adminChangedFields(in AdminUpdateCustomerInput) []string {
	var changed []string
	add := func(set bool, name string) {
		if set {
			changed = append(changed, name)
		}
	}
	add(in.Email != nil, FieldEmail)
	add(in.FirstName != nil, FieldFirstName)
	add(in.LastName != nil, FieldLastName)
	add(in.Phone != nil, FieldPhone)
	add(in.IsActive != nil, FieldIsActive)
	add(in.IsEmailVerified != nil, FieldIsEmailVerified)
	add(in.Note != nil, FieldNote)
	add(in.CustomerGroupSlug != nil, FieldCustomerGroup)
	add(in.Metadata != nil, FieldMetadata)
	add(in.DefaultBillingAddressID != nil, FieldDefaultBillingAddress)
	add(in.DefaultShippingAddressID != nil, FieldDefaultShippingAddress)
	return changed
}

// The sync customer.updating hook - handlers can abort or enrich with tags
func (s *Service) vetoUpdate(ctx context.Context, c Customer, changed []string) (*CustomerUpdating, error) {
	ing := &CustomerUpdating{CustomerID: c.ID, ChannelID: c.ChannelID, ChangedFields: changed}
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return nil, err
		}
	}
	return ing, nil
}

func updatedEvent(c Customer, changed, tags []string) *CustomerUpdated {
	return &CustomerUpdated{
		CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, LastName: c.LastName,
		IsActive: c.IsActive, ChangedFields: changed, Source: SourceAdmin, Tags: tags,
	}
}

// One admin field change: the customer.updating veto, then the write and customer.updated together
func (s *Service) adminSet(ctx context.Context, id uuid.UUID, changed []string, write func(r *postgresRepository) error) (Customer, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	ing, err := s.vetoUpdate(ctx, existing, changed)
	if err != nil {
		return Customer{}, err
	}
	var c Customer
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		if err := write(r); err != nil {
			return nil, err
		}
		var err error
		if c, err = r.GetByIDWithAddresses(ctx, id); err != nil {
			return nil, err
		}
		return []events.Event{updatedEvent(c, changed, ing.Tags)}, nil
	})
	return c, err
}

// A new address voids every link mailed to the old one and every session signed in under it, and an unverified one gets its own link -
// a deactivation ends the sessions the way SetCustomerActive does
func (s *Service) afterAdminUpdate(ctx context.Context, before, after Customer) {
	emailChanged := before.Email != after.Email
	if emailChanged {
		s.revokeEmailLinks(ctx, after.ID)
	}
	if emailChanged || (before.IsActive && !after.IsActive) {
		if err := s.customerRefresh.RevokeAll(ctx, after.ID); err != nil {
			log.Printf("[auth:warn] revoke customer %s sessions after admin update: %v", after.ID, err)
		}
	}
	if emailChanged && !after.IsEmailVerified && after.IsActive {
		s.sendAsync(ctx, "verification", func(ctx context.Context) error { return s.sendVerificationEmail(ctx, after) })
	}
}

func validateAdminUpdate(in AdminUpdateCustomerInput) error {
	if err := checkOptLen("first name", in.FirstName, MaxNameRunes); err != nil {
		return err
	}
	if err := checkOptLen("last name", in.LastName, MaxNameRunes); err != nil {
		return err
	}
	return checkOptLen("phone", in.Phone, MaxPhoneRunes)
}

// Refuses when the customer has any orders - that history is immutable, and anonymize-in-place is a separate GDPR flow - Addresses and carts cascade via FK
// The count gives the nicer error up front; an order placed after it still trips the FK, which reads the same
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.CountOrders(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrCustomerHasOrders
	}
	// Snapshot identity for the event and the address-keyed cleanup before the row is gone
	pre, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// Sync pre-commit hook - external systems can refuse the delete
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, &CustomerDeleting{CustomerID: pre.ID, ChannelID: pre.ChannelID, Email: pre.Email}); err != nil {
			return err
		}
	}
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		if err := r.Delete(ctx, pre); err != nil {
			return nil, err
		}
		return []events.Event{&CustomerDeleted{CustomerID: pre.ID, ChannelID: pre.ChannelID, Email: pre.Email}}, nil
	})
	if err != nil {
		return err
	}
	// The row is gone but its refresh tokens live in a separate store with their own 30-day TTL - leaving them would let a deleted account keep minting access tokens
	if rerr := s.customerRefresh.RevokeAll(ctx, id); rerr != nil {
		log.Printf("[auth:warn] revoke deleted customer %s sessions: %v", id, rerr)
	}
	s.revokeEmailLinks(ctx, id)
	// Signals is a separate database with no FK cascade, so a hard delete leaves the email and identity mappings behind forever
	if serr := s.repo.PurgeSignalsIdentity(ctx, id, pre.Email); serr != nil {
		log.Printf("[gdpr:warn] purge signals identity for deleted customer %s: %v", id, serr)
	}
	return nil
}

// A disabled account can't authenticate - Authenticate checks is_active, CustomerSessionGuard drops already-issued access tokens, and revoking here kills the 30-day refresh chain so the account can't mint a fresh session
func (s *Service) SetCustomerActive(ctx context.Context, id uuid.UUID, active bool) (Customer, error) {
	c, err := s.adminSet(ctx, id, []string{FieldIsActive}, func(r *postgresRepository) error {
		return r.SetActive(ctx, id, active)
	})
	if err != nil {
		return Customer{}, err
	}
	if !active {
		if rerr := s.customerRefresh.RevokeAll(ctx, id); rerr != nil {
			log.Printf("[auth:warn] revoke customer %s sessions after disable: %v", id, rerr)
		}
	}
	return c, nil
}

// The note is the exemption certificate reference shown to admins
func (s *Service) SetTaxStatus(ctx context.Context, id uuid.UUID, exempt bool, note string) (Customer, error) {
	return s.adminSet(ctx, id, []string{FieldTaxExempt, FieldTaxExemptionNote}, func(r *postgresRepository) error {
		return r.SetTaxStatus(ctx, id, exempt, note)
	})
}

// An empty vatID resets the status to unverified
func (s *Service) SetVatID(ctx context.Context, id uuid.UUID, vatID, status string) (Customer, error) {
	if vatID == "" {
		status = tax.VatStatusUnverified
	}
	return s.adminSet(ctx, id, []string{FieldVatID}, func(r *postgresRepository) error {
		return r.SetVatID(ctx, id, vatID, status)
	})
}
