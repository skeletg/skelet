package customer

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

var (
	ErrGroupInUse = errors.New("customer group has members; reassign them before deleting")

	ErrGroupSlugTaken = errors.New("customer group slug already exists")

	ErrCannotDeleteDefaultGroup = errors.New("cannot delete the default customer group")

	// Should never happen - the partial unique index plus seed data keeps exactly one default at all times
	ErrNoDefaultGroup = errors.New("no default customer group configured")

	// GDPR-style anonymize-in-place is a separate flow - admin delete is only for fresh accounts
	ErrCustomerHasOrders = errors.New("customer has orders; cannot delete")

	ErrEmailTaken = errors.New("email already registered")

	// Deliberately undistinguished from unknown-email so a caller can't probe registered addresses - ClassAuth, so the front presenter forwards the message verbatim instead of the generic internal-error string
	ErrInvalidCredentials = errors.New("invalid credentials")

	ErrAccountDisabled = errors.New("account is disabled")

	// Password matched and the account is active, but the store requires verification and the token isn't redeemed - the storefront surfaces a "resend verification email" affordance off this code
	ErrEmailNotVerified = errors.New("email not verified")

	// Distinct from ErrInvalidCredentials so admin callers see the real cause, while the storefront login path still folds it into invalid-credentials
	ErrCustomerNotFound = errors.New("customer not found")

	ErrResetLinkInvalid        = auth.ErrResetLinkInvalid
	ErrVerificationLinkInvalid = auth.ErrVerificationLinkInvalid

	// The link was mailed to an address the account no longer has, so it proves nothing about the current one
	ErrVerificationEmailMismatch = errors.New("this verification link was sent to a different email address")

	ErrIncorrectCurrentPassword = errors.New("current password is incorrect")

	ErrEmailAlreadyVerified = errors.New("email is already verified")

	ErrInvalidEmail = errors.New("enter a valid email address")

	ErrFieldTooLong = errors.New("value is too long")

	ErrTooManyAddresses = fmt.Errorf("an account can hold at most %d addresses", MaxAddressesPerCustomer)

	// Also what another customer's address reads as, so an id never confirms whose it is
	ErrAddressNotFound = errors.New("address not found")

	ErrUnknownCountry = errors.New("unknown country code")

	ErrGroupNotFound = errors.New("customer group not found")

	ErrGroupNameRequired = errors.New("customer group name is required")

	ErrInvalidGroupSlug = errors.New("invalid customer group slug")

	// Two default-group swaps raced and this one lost - the other's choice stands
	ErrDefaultGroupConflict = errors.New("the default customer group was changed at the same time; reload and try again")
)

// Input caps - the columns are unbounded text, so these keep one request from storing megabytes
const (
	MaxEmailBytes           = 254
	MaxNameRunes            = 100
	MaxPhoneRunes           = 40
	MaxCompanyRunes         = 200
	MaxStreetRunes          = 200
	MaxCityRunes            = 100
	MaxStateRunes           = 100
	MaxPostalCodeRunes      = 20
	MaxAddressesPerCustomer = 50
)

// The same type the catalog domain uses, so helpers and patterns stay consistent
type Metadata map[string]string

// Customers are not shared across stores - the same email registering on the wheels store and the tires store creates two unrelated rows
type Customer struct {
	ID                       uuid.UUID
	ChannelID                uuid.UUID
	Email                    string
	PasswordHash             string // internal - never expose via GraphQL
	FirstName                string
	LastName                 string
	Phone                    *string
	IsActive                 bool
	IsEmailVerified          bool
	Note                     *string // admin-only internal note
	Metadata                 Metadata
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DefaultBillingAddressID  *uuid.UUID
	DefaultShippingAddressID *uuid.UUID
	Addresses                []Address
	CustomerGroupID          uuid.UUID
	Group                    *CustomerGroup // populated on single-fetch + List; nil means not loaded
	// TaxExempt zeroes the core tax engine for this customer's orders and the note records the certificate or reason - VatID is stored normalized, VatIDStatus is the latest VIES answer
	TaxExempt        bool
	TaxExemptionNote string
	VatID            string
	VatIDStatus      string
	VatIDCheckedAt   *time.Time
}

type Address struct {
	ID          uuid.UUID
	CustomerID  uuid.UUID
	FirstName   string
	LastName    string
	Company     *string
	Street1     string
	Street2     *string
	City        string
	State       *string
	PostalCode  string
	CountryCode string
	Phone       *string
	CreatedAt   time.Time
}

// Its slug feeds the promotion engine's core.customer_group_in, so rules can target VIP or wholesale
type CustomerGroup struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Description string
	IsDefault   bool
	CreatedAt   time.Time
	MemberCount int64 // populated only by ListGroups
}

// The customer's group is auto-assigned to the current default by the SQL
type CreateCustomerInput struct {
	ChannelID    uuid.UUID
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	GroupID      *uuid.UUID // nil takes the default group
	// Only a path that has proven the address sets this - a confirmed double opt-in
	EmailVerified bool
}

type UpdateCustomerInput struct {
	FirstName string
	LastName  string
	Phone     *string
}

type CreateCustomerGroupInput struct {
	Name        string
	Slug        string
	Description string
}

type UpdateCustomerGroupInput struct {
	Name        *string
	Description *string
}

// IsActive and IsEmailVerified default to true when unset, since the admin vouches - an empty CustomerGroupSlug routes to the default group
type AdminCreateCustomerInput struct {
	ChannelID         uuid.UUID
	Email             string
	Password          string
	FirstName         string
	LastName          string
	Phone             *string
	IsActive          *bool  // nil → true (default active)
	IsEmailVerified   *bool  // nil → true (admin vouches)
	CustomerGroupSlug string // "" → default group
}

// Every field is a pointer and nil means no change
// CustomerGroupSlug non-nil empty resets to the default group, Metadata non-nil even when empty replaces all entries, and the default address ids must belong to this customer
type AdminUpdateCustomerInput struct {
	Email                    *string
	FirstName                *string
	LastName                 *string
	Phone                    *string
	IsActive                 *bool
	IsEmailVerified          *bool
	Note                     *string
	CustomerGroupSlug        *string
	Metadata                 *Metadata
	DefaultBillingAddressID  *uuid.UUID
	DefaultShippingAddressID *uuid.UUID
}

type AddressInput struct {
	FirstName   string
	LastName    string
	Company     *string
	Street1     string
	Street2     *string
	City        string
	State       *string
	PostalCode  string
	CountryCode string
	Phone       *string
}
