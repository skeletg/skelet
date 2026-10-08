package customer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Declared locally to avoid importing channel and cycling with auth
type StoreLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (StoreSnapshot, error)
}

// The channel projection outbound emails render from - main.go adapts channel.Service onto it
type StoreSnapshot struct {
	ID               uuid.UUID
	PublicURL        string
	EmailFromAddress string
	EmailFromName    string
}

// customerRefresh RevokeAll's sessions after a reset, bus fires CustomerRegistering (sync, pre-commit) and CustomerRegistered (async, post-commit)
// stores is per-store so email links and "from" addresses come from the customer's store rather than a global env var
type Service struct {
	repo            *postgresRepository
	verifyStore     *auth.VerificationStore
	resetStore      *auth.PasswordResetStore
	customerRefresh *auth.RefreshStore
	emailer         email.Sender
	stores          StoreLookup
	consentSvc      *consent.Service
	bus             *events.Bus
	cfg             *config.Service
	countries       CountryValidator
	authFlows       authEmailDispatcher
	authFallback    authEmailFallback
	// The roster events go durable through these when wired - see outbox.go
	outbox OutboxEnqueuer
	tx     *txrunner.Manager
	mail   sync.WaitGroup
}

// Routes account emails through Pulse flows - this service hands off the recipient and one-time link, the flow engine renders and sends synchronously
// A structural interface so customer doesn't import flow (flow imports customer) - nil, or a zero return, falls through to authEmailFallback
type authEmailDispatcher interface {
	// Both return the number of flows that took delivery - zero means nobody sent the mail
	OnEmailVerification(ctx context.Context, channelID, customerID uuid.UUID, verifyLink, storeName, firstName string) int
	OnPasswordReset(ctx context.Context, channelID, customerID uuid.UUID, resetLink, expiresIn, storeName, firstName string) int
	OnSignupConfirmation(ctx context.Context, channelID uuid.UUID, to, confirmLink, expiresIn, storeName, firstName string) int
	OnAccountExists(ctx context.Context, channelID, customerID uuid.UUID, signInLink, forgotLink, storeName, firstName string) int
}

func (s *Service) SetAuthEmailDispatcher(d authEmailDispatcher) { s.authFlows = d }

// Sends account mail when no flow did, so verification and reset never depend on a flow being active - satisfied by *emailtemplate.Service
// Nil leaves the pure-flow behaviour: no flow means no email
type authEmailFallback interface {
	SendVerification(ctx context.Context, channelID uuid.UUID, from, to, storefrontURL string, d email.VerificationEmailData) error
	SendPasswordReset(ctx context.Context, channelID uuid.UUID, from, to, storefrontURL string, d email.PasswordResetEmailData) error
	SendSignupConfirmation(ctx context.Context, channelID uuid.UUID, from, to, storefrontURL string, d email.SignupConfirmationEmailData) error
	SendAccountExists(ctx context.Context, channelID uuid.UUID, from, to, storefrontURL string, d email.AccountExistsEmailData) error
}

func (s *Service) SetAuthEmailFallback(f authEmailFallback) { s.authFallback = f }

// The From header for account mail - mirrors the flow engine's, so a fallback send is indistinguishable from a flow one
func (st StoreSnapshot) fromHeader() string {
	addr := strings.TrimSpace(st.EmailFromAddress)
	if addr == "" {
		addr = "no-reply@skelet.local"
	}
	if name := strings.TrimSpace(st.EmailFromName); name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

func (s *Service) storeName(ctx context.Context, st StoreSnapshot) string {
	if s.cfg != nil {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &st.ID}
		if name, err := s.cfg.GetString(ctx, "general/branding/store_name", scope); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return st.EmailFromName
}

// Optional - nil skips per-channel country/region enforcement on address writes
type CountryValidator interface {
	ValidateAddressForChannel(ctx context.Context, channelID uuid.UUID, countryCode string, region *string) error
}

func (s *Service) SetCountryValidator(v CountryValidator) { s.countries = v }

// Optional - when nil, Authenticate runs without the email-verification gate, preserving what CLI and test paths rely on
func (s *Service) SetConfigService(c *config.Service) {
	s.cfg = c
}

// NormalizeEmail is the single definition of a customer's login identity - addresses are case-insensitive in practice and users paste them with stray whitespace, so without this "Alice@x.com" and "alice@x.com " register as separate accounts and neither can log in as the other
// Applied at every write AND every lookup, or the two disagree and a legitimately-registered address stops matching
func NormalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}

// RequiresEmailVerification exposes the channel's gate so the registration resolver can withhold the session - minting one before the address is proven would make the whole setting a no-op
func (s *Service) RequiresEmailVerification(ctx context.Context, channelID uuid.UUID) bool {
	return s.requireEmailVerification(ctx, channelID)
}

// Falls back to the registry default (false) on any miss - best-effort, since a config-read error returning false keeps login open rather than locking everyone out
func (s *Service) requireEmailVerification(ctx context.Context, channelID uuid.UUID) bool {
	if s.cfg == nil {
		return false
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	v, err := s.cfg.GetBool(ctx, pathRequireEmailVerification, scope)
	if err != nil {
		return false
	}
	return v
}

// Email deps are injected by main.go and the CLI can pass no-ops, since read paths don't touch them
// bus may be nil - call sites check and skip event dispatch, which keeps CLI paths simple
func NewService(
	repo *postgresRepository,
	verifyStore *auth.VerificationStore,
	resetStore *auth.PasswordResetStore,
	customerRefresh *auth.RefreshStore,
	emailer email.Sender,
	stores StoreLookup,
	consentSvc *consent.Service,
	bus *events.Bus,
) *Service {
	return &Service{
		repo:            repo,
		verifyStore:     verifyStore,
		resetStore:      resetStore,
		customerRefresh: customerRefresh,
		emailer:         emailer,
		stores:          stores,
		consentSvc:      consentSvc,
		bus:             bus,
	}
}

// The group a customer.registering plugin routed the account into, or nil for the default - a group deleted since the
// plugin was configured falls back to the default rather than refusing every signup it would have routed
func (s *Service) registeringGroup(ctx context.Context, ing *CustomerRegistering) *uuid.UUID {
	if ing.GroupID == nil {
		return nil
	}
	if _, err := s.repo.GetGroupByID(ctx, *ing.GroupID); err != nil {
		log.Printf("[registration:warn] customer group %s picked by a customer.registering subscriber is missing, using the default: %v", *ing.GroupID, err)
		return nil
	}
	return ing.GroupID
}

// The verification email and the consent write are both non-fatal - a failing SMTP server or Valkey blip shouldn't block account creation
// marketingOptIn is three-state: nil means no checkbox was shown (leave consent at the registry default, opt-out for GDPR), *true is an explicit opt-in with source=registration, *false records an audit-visible "no"
// A taken address mails its owner and still returns ErrEmailTaken - the storefront resolver answers it exactly as it answers a new registration
func (s *Service) Register(ctx context.Context, channelID uuid.UUID, emailAddr, password, firstName, lastName string, marketingOptIn *bool) (Customer, error) {
	emailAddr = NormalizeEmail(emailAddr)
	if err := validateEmail(emailAddr); err != nil {
		return Customer{}, err
	}
	if err := validateNames(firstName, lastName); err != nil {
		return Customer{}, err
	}
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return Customer{}, err
	}
	// Pre-commit hook - subscribers can reject (fraud, blocklist) or enrich by tagging the account
	ing := &CustomerRegistering{
		ChannelID: channelID,
		Email:     emailAddr,
		FirstName: firstName,
		LastName:  lastName,
	}
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Customer{}, fmt.Errorf("registration blocked: %w", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.HashCost)
	if err != nil {
		return Customer{}, fmt.Errorf("hash password: %w", err)
	}
	c, err := s.repo.Create(ctx, CreateCustomerInput{
		ChannelID:    channelID,
		Email:        emailAddr,
		PasswordHash: string(hash),
		FirstName:    firstName,
		LastName:     lastName,
		GroupID:      s.registeringGroup(ctx, ing),
	})
	if errors.Is(err, ErrEmailTaken) {
		s.sendAsync(ctx, "account exists", func(ctx context.Context) error {
			return s.sendAccountExists(ctx, channelID, emailAddr)
		})
		return Customer{}, ErrEmailTaken
	}
	if err != nil {
		return Customer{}, err
	}
	s.sendAsync(ctx, "verification", func(ctx context.Context) error { return s.sendVerificationEmail(ctx, c) })
	// The shopper's own answer wins - a plugin only decides where no checkbox was shown, since it can't consent on anyone's behalf
	optIn, source := marketingOptIn, consent.SourceRegistration
	if optIn == nil && ing.MarketingConsent != nil {
		optIn, source = ing.MarketingConsent, consent.SourcePlugin
	}
	if optIn != nil {
		if _, cerr := s.consentSvc.SetConsent(ctx, c.ID, consentkind.MarketingEmail, *optIn, source); cerr != nil {
			// Non-fatal - the account exists, consent stays at the registry default
			log.Printf("[consent:error] record marketing opt-in for customer %s: %v", c.ID, cerr)
		}
	}

	// Post-commit - tags from the pre-commit chain carry forward so enrichers can influence async subscribers
	s.publishDurable(ctx, &CustomerRegistered{
		CustomerID: c.ID,
		ChannelID:  c.ChannelID,
		Email:      c.Email,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		Source:     "self",
		Tags:       ing.Tags,
	})
	return c, nil
}

// Email is unique per (store, email), so channelID is required to disambiguate - logging into the wheels store with a tires-store account fails as invalid credentials
// Auth events bracket the check: sync auth.login.attempting before any DB read, async auth.login.succeeded / .failed after
func (s *Service) Authenticate(ctx context.Context, channelID uuid.UUID, email, password string) (Customer, error) {
	email = NormalizeEmail(email)
	ip, ua := authAttribution(ctx)
	if s.bus != nil {
		attempting := &auth.LoginAttempting{
			Subject:   auth.SubjectCustomer,
			Email:     email,
			IP:        ip,
			UserAgent: ua,
			ChannelID: channelID,
		}
		if err := s.bus.PublishSync(ctx, attempting); err != nil {
			s.bus.Publish(ctx, &auth.LoginFailed{
				Subject:   auth.SubjectCustomer,
				Email:     email,
				IP:        ip,
				UserAgent: ua,
				ChannelID: channelID,
				Reason:    "rejected_by_subscriber",
			})
			return Customer{}, fmt.Errorf("login rejected: %w", err)
		}
	}
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, email)
	if errors.Is(err, ErrCustomerNotFound) {
		// Equalize timing with the found-but-wrong-password path so response time doesn't reveal whether the email is registered
		auth.CompareDummyPassword(password)
		s.publishLoginFailed(ctx, email, ip, ua, channelID, "invalid_credentials")
		return Customer{}, ErrInvalidCredentials
	}
	if err != nil {
		return Customer{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(password)); err != nil {
		s.publishLoginFailed(ctx, email, ip, ua, channelID, "invalid_credentials")
		return Customer{}, ErrInvalidCredentials
	}
	if !c.IsActive {
		s.publishLoginFailed(ctx, email, ip, ua, channelID, "account_disabled")
		return Customer{}, ErrAccountDisabled
	}
	if !c.IsEmailVerified && s.requireEmailVerification(ctx, channelID) {
		s.publishLoginFailed(ctx, email, ip, ua, channelID, "email_not_verified")
		return Customer{}, ErrEmailNotVerified
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &auth.LoginSucceeded{
			Subject:   auth.SubjectCustomer,
			SubjectID: c.ID,
			Email:     email,
			IP:        ip,
			UserAgent: ua,
			ChannelID: channelID,
		})
	}
	return c, nil
}

func (s *Service) publishLoginFailed(ctx context.Context, email, ip, ua string, channelID uuid.UUID, reason string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &auth.LoginFailed{
		Subject:   auth.SubjectCustomer,
		Email:     email,
		IP:        ip,
		UserAgent: ua,
		ChannelID: channelID,
		Reason:    reason,
	})
}

func authAttribution(ctx context.Context) (ip, ua string) {
	r, ok := auth.RequestFromContext(ctx)
	if !ok || r == nil {
		return "", ""
	}
	return auth.ClientIP(r), r.UserAgent()
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.repo.GetByID(ctx, id)
}

// found=false for guests with no account, and no error - back-in-stock and form flows use it to route notifications through the flow engine when the address is a known customer
func (s *Service) CustomerIDByEmail(ctx context.Context, channelID uuid.UUID, email string) (uuid.UUID, bool) {
	email = NormalizeEmail(email)
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, email)
	if err != nil {
		return uuid.Nil, false
	}
	return c.ID, true
}

// The "email = profile" model signup forms need - a newsletter subscriber becomes targetable (segments, flows, consent) without an account created is true only on a fresh record, so the caller fires CustomerRegistered exactly once A random unusable password hash is stored so the row can never be logged into until a reset sets a real one, which is how the contact later claims the account
// verified marks a fresh contact's address as proven, which only a confirmed double opt-in may claim
func (s *Service) EnsureContact(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool) (Customer, bool, error) {
	email = NormalizeEmail(email)
	if err := validateEmail(email); err != nil {
		return Customer{}, false, err
	}
	if err := checkLen("first name", firstName, MaxNameRunes); err != nil {
		return Customer{}, false, err
	}
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, email)
	if err == nil {
		return c, false, nil
	}
	if !errors.Is(err, ErrCustomerNotFound) {
		return Customer{}, false, err
	}
	// Unusable password - bcrypt of 32 random bytes, never shared
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return Customer{}, false, err
	}
	hash, err := bcrypt.GenerateFromPassword(buf[:], auth.HashCost)
	if err != nil {
		return Customer{}, false, err
	}
	c, err = s.repo.Create(ctx, CreateCustomerInput{
		ChannelID: channelID, Email: email, PasswordHash: string(hash), FirstName: firstName, EmailVerified: verified,
	})
	if err != nil {
		return Customer{}, false, err
	}
	s.publishDurable(ctx, &CustomerRegistered{
		CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email,
		FirstName: c.FirstName, LastName: c.LastName, Source: "signup_form",
	})
	if verified {
		s.publishDurable(ctx, &CustomerEmailVerified{CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email})
	}
	return c, true, nil
}

// Batched lookup for the storefront DataLoader - empty input skips the DB, and missing ids come back as absent keys rather than errors
func (s *Service) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Customer, error) {
	return s.repo.GetByIDs(ctx, ids)
}

func (s *Service) GetByIDWithAddresses(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.repo.GetByIDWithAddresses(ctx, id)
}

// ListPage is the server-paged admin list - see ListPageFilter
func (s *Service) ListPage(ctx context.Context, f ListPageFilter) ([]Customer, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.repo.ListPage(ctx, f)
}

func (s *Service) List(ctx context.Context) ([]Customer, error) {
	return s.repo.List(ctx)
}

// nil channelIDs is every channel, an empty slice none
func (s *Service) ListInChannels(ctx context.Context, channelIDs []uuid.UUID) ([]Customer, error) {
	if channelIDs != nil && len(channelIDs) == 0 {
		return nil, nil
	}
	return s.repo.ListInChannels(ctx, channelIDs)
}

func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, input UpdateCustomerInput) (Customer, error) {
	if err := validateNames(input.FirstName, input.LastName); err != nil {
		return Customer{}, err
	}
	if err := checkOptLen("phone", input.Phone, MaxPhoneRunes); err != nil {
		return Customer{}, err
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	merged := UpdateCustomerInput{
		FirstName: existing.FirstName,
		LastName:  existing.LastName,
		Phone:     existing.Phone,
	}
	if input.FirstName != "" {
		merged.FirstName = input.FirstName
	}
	if input.LastName != "" {
		merged.LastName = input.LastName
	}
	if input.Phone != nil {
		merged.Phone = input.Phone
	}
	var changed []string
	if input.FirstName != "" && input.FirstName != existing.FirstName {
		changed = append(changed, "first_name")
	}
	if input.LastName != "" && input.LastName != existing.LastName {
		changed = append(changed, "last_name")
	}
	if input.Phone != nil {
		changed = append(changed, "phone")
	}
	updated, err := s.repo.Update(ctx, id, merged)
	if err != nil {
		return Customer{}, err
	}
	s.publishDurable(ctx, &CustomerProfileUpdated{CustomerID: id, ChannelID: existing.ChannelID, ChangedFields: changed})
	return updated, nil
}

func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, currentPassword, newPassword string) error {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrIncorrectCurrentPassword
	}
	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), auth.HashCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.repo.UpdatePassword(ctx, id, string(hash)); err != nil {
		return err
	}
	// Same as ResetPassword - changing the password is how someone evicts an attacker, so every other session has to die with it or a stolen 30-day refresh token just keeps rotating
	if err := s.customerRefresh.RevokeAll(ctx, id); err != nil {
		log.Printf("[auth:warn] revoke customer %s sessions after password change: %v", id, err)
	}
	s.revokeResetLinks(ctx, id)
	if s.bus != nil {
		s.bus.Publish(ctx, &CustomerPasswordChanged{CustomerID: id, ChannelID: c.ChannelID})
	}
	return nil
}

func (s *Service) AddAddress(ctx context.Context, customerID uuid.UUID, input AddressInput) (Address, error) {
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
	s.publishAddressChanged(ctx, customerID, addr.ID, "add")
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

func (s *Service) UpdateAddress(ctx context.Context, customerID, addressID uuid.UUID, input AddressInput) (Address, error) {
	addr, err := s.repo.GetAddress(ctx, addressID)
	if err != nil {
		return Address{}, fmt.Errorf("address not found")
	}
	if addr.CustomerID != customerID {
		return Address{}, fmt.Errorf("forbidden")
	}
	if err := validateAddressInput(input); err != nil {
		return Address{}, err
	}
	if err := s.validateAddressCountry(ctx, customerID, input); err != nil {
		return Address{}, err
	}
	updated, err := s.repo.UpdateAddress(ctx, addressID, input)
	if err != nil {
		return Address{}, err
	}
	s.publishAddressChanged(ctx, customerID, addressID, "update")
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
	addr, err := s.repo.GetAddress(ctx, addressID)
	if err != nil {
		return fmt.Errorf("address not found")
	}
	if addr.CustomerID != customerID {
		return fmt.Errorf("forbidden")
	}
	if err := s.repo.DeleteAddress(ctx, addressID); err != nil {
		return err
	}
	s.publishAddressChanged(ctx, customerID, addressID, "delete")
	return nil
}

// Returns the row regardless of owner - admin resolvers need it to find the owning customer before running the channel-scoped permission check
func (s *Service) GetAddressByID(ctx context.Context, id uuid.UUID) (Address, error) {
	return s.repo.GetAddress(ctx, id)
}

// No ownership check - admin only
func (s *Service) AdminDeleteAddress(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetAddress(ctx, id); err != nil {
		return fmt.Errorf("address not found")
	}
	return s.repo.DeleteAddress(ctx, id)
}

func (s *Service) SetDefaultBillingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	addr, err := s.repo.GetAddress(ctx, addressID)
	if err != nil {
		return Customer{}, fmt.Errorf("address not found")
	}
	if addr.CustomerID != customerID {
		return Customer{}, fmt.Errorf("forbidden")
	}
	return s.repo.SetDefaultBillingAddress(ctx, customerID, addressID)
}

func (s *Service) SetDefaultShippingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	addr, err := s.repo.GetAddress(ctx, addressID)
	if err != nil {
		return Customer{}, fmt.Errorf("address not found")
	}
	if addr.CustomerID != customerID {
		return Customer{}, fmt.Errorf("forbidden")
	}
	return s.repo.SetDefaultShippingAddress(ctx, customerID, addressID)
}

func (s *Service) CreateGroup(ctx context.Context, input CreateCustomerGroupInput) (CustomerGroup, error) {
	if err := validateGroupSlug(input.Slug); err != nil {
		return CustomerGroup{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return CustomerGroup{}, fmt.Errorf("group name is required")
	}
	return s.repo.CreateGroup(ctx, input)
}

func (s *Service) UpdateGroup(ctx context.Context, id uuid.UUID, input UpdateCustomerGroupInput) (CustomerGroup, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return CustomerGroup{}, fmt.Errorf("group name cannot be empty")
	}
	return s.repo.UpdateGroup(ctx, id, input)
}

// ErrGroupInUse if it has members, ErrCannotDeleteDefaultGroup if it's the current default
func (s *Service) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteGroup(ctx, id)
}

func (s *Service) GetGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	return s.repo.GetGroupByID(ctx, id)
}

func (s *Service) GetGroupBySlug(ctx context.Context, slug string) (CustomerGroup, error) {
	return s.repo.GetGroupBySlug(ctx, slug)
}

func (s *Service) GetDefaultGroup(ctx context.Context) (CustomerGroup, error) {
	return s.repo.GetDefaultGroup(ctx)
}

// Populates each group's member count
func (s *Service) ListGroups(ctx context.Context) ([]CustomerGroup, error) {
	return s.repo.ListGroups(ctx)
}

// No-op when it's already the default, otherwise an atomic swap
func (s *Service) SetDefaultGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	current, err := s.repo.GetGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, err
	}
	if current.IsDefault {
		return current, nil
	}
	return s.repo.SetDefaultGroup(ctx, id)
}

// Verifies the group exists first so a bogus id gets a clean error
func (s *Service) SetCustomerGroup(ctx context.Context, customerID, groupID uuid.UUID) (Customer, error) {
	if _, err := s.repo.GetGroupByID(ctx, groupID); err != nil {
		return Customer{}, err
	}
	c, err := s.repo.SetCustomerGroup(ctx, customerID, groupID)
	if err != nil {
		return Customer{}, err
	}
	s.publishUpdated(ctx, c, "customerGroup")
	return c, nil
}

func (s *Service) publishUpdated(ctx context.Context, c Customer, field string) {
	s.publishDurable(ctx, &CustomerUpdated{
		CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, LastName: c.LastName,
		IsActive: c.IsActive, ChangedFields: []string{field}, Source: "admin",
	})
}

// How long a detached account-mail send may run once the request that started it has returned
const mailSendTimeout = 30 * time.Second

// Account mail leaves the request path, so whether an address has an account never shows in the response time
func (s *Service) sendAsync(ctx context.Context, what string, send func(context.Context) error) {
	s.mail.Go(func() {
		mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mailSendTimeout)
		defer cancel()
		if err := send(mctx); err != nil {
			slog.Error("account mail failed", "mail", what, "err", err)
		}
	})
}

// FlushMail waits for every account mail started so far - for tests and shutdown
func (s *Service) FlushMail() { s.mail.Wait() }

// Where the account-exists notice sends the owner, on the shop's own origin
const (
	SignInPath         = "/account/login"
	ForgotPasswordPath = "/account/forgot"
)

func (s *Service) sendAccountExists(ctx context.Context, channelID uuid.UUID, addr string) error {
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, addr)
	if err != nil {
		return fmt.Errorf("load account owner: %w", err)
	}
	store, err := s.stores.GetByID(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load store for email: %w", err)
	}
	storefront := strings.TrimRight(store.PublicURL, "/")
	signIn, forgot := storefront+SignInPath, storefront+ForgotPasswordPath
	name := s.storeName(ctx, store)
	if s.authFlows != nil && s.authFlows.OnAccountExists(ctx, channelID, c.ID, signIn, forgot, name, c.FirstName) > 0 {
		return nil
	}
	if s.authFallback == nil {
		log.Printf("[email:warn] account exists: no flow handled it and no fallback wired; not sent to customer %s", c.ID)
		return nil
	}
	return s.authFallback.SendAccountExists(ctx, channelID, store.fromHeader(), c.Email, storefront, email.AccountExistsEmailData{
		FirstName: c.FirstName, SignInLink: signIn, ResetLink: forgot, StoreName: name,
	})
}

// Best-effort - a failure leaves links that still expire on their own
func (s *Service) revokeResetLinks(ctx context.Context, id uuid.UUID) {
	if err := s.resetStore.RevokeAll(ctx, id); err != nil {
		log.Printf("[auth:warn] revoke customer %s reset links: %v", id, err)
	}
}

// A changed address makes every link mailed to the old one stale
func (s *Service) revokeEmailLinks(ctx context.Context, id uuid.UUID) {
	if err := s.verifyStore.RevokeAll(ctx, id); err != nil {
		log.Printf("[auth:warn] revoke customer %s verification links: %v", id, err)
	}
	s.revokeResetLinks(ctx, id)
}

// Shared by Register and ResendVerificationEmail
func (s *Service) sendVerificationEmail(ctx context.Context, c Customer) error {
	token, err := s.verifyStore.Create(ctx, c.ID, c.Email)
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	store, err := s.stores.GetByID(ctx, c.ChannelID)
	if err != nil {
		return fmt.Errorf("load store for email: %w", err)
	}
	storefront := strings.TrimRight(store.PublicURL, "/")
	link := storefront + "/verify-email?token=" + token
	// A flow on the email_verification trigger delivers this when one is active, so operators can edit the journey
	name := s.storeName(ctx, store)
	if s.authFlows != nil && s.authFlows.OnEmailVerification(ctx, c.ChannelID, c.ID, link, name, c.FirstName) > 0 {
		return nil
	}
	// Nothing handled it - send directly rather than let a missing or paused flow silently swallow a verification
	if s.authFallback == nil {
		log.Printf("[email:warn] verification: no flow handled it and no fallback wired; not sent to customer %s", c.ID)
		return nil
	}
	return s.authFallback.SendVerification(ctx, c.ChannelID, store.fromHeader(), c.Email, storefront, email.VerificationEmailData{
		FirstName: c.FirstName, Link: link, StoreName: name,
	})
}

// The signup form's double opt-in link, addressed to someone who may have no account yet - delivered like verification, by a flow when one takes it and from core's own template otherwise
func (s *Service) SendSignupConfirmation(ctx context.Context, channelID uuid.UUID, to, firstName, token, expiresIn string) error {
	store, err := s.stores.GetByID(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load store for email: %w", err)
	}
	storefront := strings.TrimRight(store.PublicURL, "/")
	link := storefront + SignupConfirmPath + token
	name := s.storeName(ctx, store)
	if s.authFlows != nil && s.authFlows.OnSignupConfirmation(ctx, channelID, to, link, expiresIn, name, firstName) > 0 {
		return nil
	}
	if s.authFallback == nil {
		return fmt.Errorf("signup confirmation: no flow handled it and no fallback is wired")
	}
	return s.authFallback.SendSignupConfirmation(ctx, channelID, store.fromHeader(), to, storefront, email.SignupConfirmationEmailData{
		FirstName: firstName, Link: link, StoreName: name, ExpiresIn: expiresIn,
	})
}

// Where a confirmation link lands, on the shop's own origin - the storefront proxies it to core
const SignupConfirmPath = "/subscribe/confirm/"

// Single-use - the token is revoked right after the customer is marked verified, and only proves the address it was mailed to
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	claim, err := s.verifyStore.Validate(ctx, token)
	if err != nil {
		return err
	}
	c, err := s.repo.GetByID(ctx, claim.CustomerID)
	if errors.Is(err, ErrCustomerNotFound) {
		return ErrVerificationLinkInvalid
	}
	if err != nil {
		return err
	}
	if !claim.BoundTo(c.Email) {
		s.revokeVerifyToken(ctx, token)
		return ErrVerificationEmailMismatch
	}
	if err := s.markEmailVerified(ctx, c); err != nil {
		return err
	}
	s.revokeVerifyToken(ctx, token)
	return nil
}

func (s *Service) revokeVerifyToken(ctx context.Context, token string) {
	if err := s.verifyStore.Revoke(ctx, token); err != nil {
		// A live token does no harm here and the main key TTLs out in 24h, so log rather than fail
		log.Printf("[email:warn] revoke verification token after use: %v", err)
	}
}

// The flag and the event commit together, and the write only lands while the row still has the address that was proven -
// the guest-order claim hangs off the event, so a lost one would leave the customer's history behind
func (s *Service) markEmailVerified(ctx context.Context, c Customer) error {
	evt := &CustomerEmailVerified{CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email}
	var enqueue func(*sql.Tx) error
	if s.outbox != nil {
		enqueue = func(t *sql.Tx) error { return s.outbox.Enqueue(ctx, t, evt) }
	}
	if err := s.repo.MarkEmailVerified(ctx, c.ID, c.Email, enqueue); err != nil {
		return err
	}
	if enqueue == nil && s.bus != nil {
		s.bus.Publish(ctx, evt)
	}
	return nil
}

// The resolver enforces authenticated customer context - refuses to send when they're already verified, which would just confuse them
func (s *Service) ResendVerificationEmail(ctx context.Context, customerID uuid.UUID) error {
	c, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	if c.IsEmailVerified {
		return ErrEmailAlreadyVerified
	}
	return s.sendVerificationEmail(ctx, c)
}

// The signed-out resend, for a customer the verification gate keeps from signing in - it says nothing about whether the address has an account
func (s *Service) ResendVerificationByEmail(ctx context.Context, channelID uuid.UUID, emailAddr string) error {
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, NormalizeEmail(emailAddr))
	if errors.Is(err, ErrCustomerNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.IsEmailVerified || !c.IsActive {
		return nil
	}
	s.sendAsync(ctx, "verification", func(ctx context.Context) error { return s.sendVerificationEmail(ctx, c) })
	return nil
}

// Returns nil whether or not the address has an account (enumeration defense) - only a storage failure, which says nothing about the address, comes back
// The token and the mail run detached, so both paths answer in about the same time
func (s *Service) RequestPasswordReset(ctx context.Context, channelID uuid.UUID, emailAddr string) error {
	emailAddr = NormalizeEmail(emailAddr)
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, emailAddr)
	if errors.Is(err, ErrCustomerNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Sync pre-mint hook - a subscriber may veto the reset, e.g - SSO-only accounts that mustn't reset by email
	// A veto returns the same nil as an unknown email, so the enumeration defense above still holds
	if s.bus != nil {
		ip, _ := authAttribution(ctx)
		if err := s.bus.PublishSync(ctx, &auth.PasswordResetRequesting{
			Subject:   auth.SubjectCustomer,
			Email:     c.Email,
			IP:        ip,
			ChannelID: c.ChannelID,
		}); err != nil {
			log.Printf("[auth] password reset vetoed for customer %s: %v", c.ID, err)
			return nil
		}
	}
	s.sendAsync(ctx, "password reset", func(ctx context.Context) error { return s.sendPasswordReset(ctx, c) })
	return nil
}

func (s *Service) sendPasswordReset(ctx context.Context, c Customer) error {
	token, err := s.resetStore.Create(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("create reset token: %w", err)
	}
	store, err := s.stores.GetByID(ctx, c.ChannelID)
	if err != nil {
		return fmt.Errorf("load store for email: %w", err)
	}
	storefront := strings.TrimRight(store.PublicURL, "/")
	link := storefront + "/reset-password?token=" + token
	expiresIn := humanDuration(s.resetStore.TTL())
	// A flow on the password_reset trigger delivers this when one is active, sent synchronously
	name := s.storeName(ctx, store)
	if s.authFlows != nil && s.authFlows.OnPasswordReset(ctx, c.ChannelID, c.ID, link, expiresIn, name, c.FirstName) > 0 {
		return nil
	}
	// Nothing handled it - send directly, or a paused flow locks the customer out of their own account
	if s.authFallback == nil {
		log.Printf("[email:warn] password reset: no flow handled it and no fallback wired; not sent to customer %s", c.ID)
		return nil
	}
	return s.authFallback.SendPasswordReset(ctx, c.ChannelID, store.fromHeader(), c.Email, storefront, email.PasswordResetEmailData{
		FirstName: c.FirstName, Link: link, StoreName: name, ExpiresIn: expiresIn,
	})
}

// Revokes every live refresh token - pre-reset access JWTs survive their 15-min TTL, the standard JWT tradeoff The channelID store-match is enforced here rather than only at the resolver, so a password write can never cross stores if a future caller forgets the check A mismatch returns the same opaque error as an invalid token, so the response never confirms the customer exists elsewhere
func (s *Service) ResetPassword(ctx context.Context, channelID uuid.UUID, token, newPassword string) error {
	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	// Peeked first, so a link presented on the wrong store is refused without being spent
	customerID, err := s.resetStore.Validate(ctx, token)
	if err != nil {
		return err
	}
	c, err := s.repo.GetByID(ctx, customerID)
	if errors.Is(err, ErrCustomerNotFound) || (err == nil && c.ChannelID != channelID) {
		return ErrResetLinkInvalid
	}
	if err != nil {
		return err
	}
	// Spent before the write, so two concurrent redemptions can't both set a password
	if spent, err := s.resetStore.Consume(ctx, token); err != nil {
		return err
	} else if spent != customerID {
		return ErrResetLinkInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), auth.HashCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.repo.UpdatePassword(ctx, customerID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if err := s.customerRefresh.RevokeAll(ctx, customerID); err != nil {
		// Log but don't fail - the password is already changed
		log.Printf("[auth:warn] revoke customer %s sessions after password reset: %v", customerID, err)
	}
	s.revokeResetLinks(ctx, customerID)
	// The link reached the inbox, which is all a verification link proves
	if !c.IsEmailVerified {
		if err := s.markEmailVerified(ctx, c); err != nil {
			log.Printf("[auth:warn] mark customer %s verified after password reset: %v", customerID, err)
		}
	}
	// Distinct from customer.password_changed, which is the logged-in self-service change - subscribers here send "your password was changed" notices and revoke downstream sessions
	if s.bus != nil {
		s.bus.Publish(ctx, &auth.PasswordResetCompleted{
			Subject:   auth.SubjectCustomer,
			SubjectID: customerID,
			Email:     c.Email,
			ChannelID: c.ChannelID,
		})
	}
	return nil
}

// Short English for email copy - exact phrasing isn't load-bearing
func humanDuration(d time.Duration) string {
	if d >= time.Hour {
		h := int(d / time.Hour)
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	m := int(d / time.Minute)
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
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

	var groupID uuid.UUID
	if input.CustomerGroupSlug == "" {
		g, err := s.repo.GetDefaultGroup(ctx)
		if err != nil {
			return Customer{}, err
		}
		groupID = g.ID
	} else {
		g, err := s.repo.GetGroupBySlug(ctx, input.CustomerGroupSlug)
		if err != nil {
			return Customer{}, err
		}
		groupID = g.ID
	}

	// Normalized copy so we don't mutate the caller's input
	normalized := input
	if normalized.IsActive == nil {
		t := true
		normalized.IsActive = &t
	}
	if normalized.IsEmailVerified == nil {
		t := true
		normalized.IsEmailVerified = &t
	}

	c, err := s.repo.AdminCreate(ctx, normalized, string(hash), groupID)
	if err != nil {
		return Customer{}, err
	}

	// Post-commit only - admin-created accounts skip the pre-commit fraud chain because the admin vouched for the address
	s.publishDurable(ctx, &CustomerRegistered{
		CustomerID: c.ID,
		ChannelID:  c.ChannelID,
		Email:      c.Email,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		Source:     "admin",
	})
	return c, nil
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
	// Build ChangedFields before any mutation so subscribers know what changed
	var changed []string
	if input.Email != nil {
		changed = append(changed, "email")
	}
	if input.FirstName != nil {
		changed = append(changed, "firstName")
	}
	if input.LastName != nil {
		changed = append(changed, "lastName")
	}
	if input.Phone != nil {
		changed = append(changed, "phone")
	}
	if input.IsActive != nil {
		changed = append(changed, "isActive")
	}
	if input.IsEmailVerified != nil {
		changed = append(changed, "isEmailVerified")
	}
	if input.Note != nil {
		changed = append(changed, "note")
	}
	if input.CustomerGroupSlug != nil {
		changed = append(changed, "customerGroup")
	}
	if input.Metadata != nil {
		changed = append(changed, "metadata")
	}
	if input.DefaultBillingAddressID != nil {
		changed = append(changed, "defaultBillingAddress")
	}
	if input.DefaultShippingAddressID != nil {
		changed = append(changed, "defaultShippingAddress")
	}

	// Load the customer first so events carry its store - subscribers filter on that
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Customer{}, err
	}

	// Sync pre-commit hook - handlers can abort or enrich with tags
	updatingEvt := &CustomerUpdating{
		CustomerID:    id,
		ChannelID:     existing.ChannelID,
		ChangedFields: changed,
	}
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, updatingEvt); err != nil {
			return Customer{}, err
		}
	}

	var groupID *uuid.UUID
	if input.CustomerGroupSlug != nil {
		slug := *input.CustomerGroupSlug
		if slug == "" {
			g, err := s.repo.GetDefaultGroup(ctx)
			if err != nil {
				return Customer{}, err
			}
			gid := g.ID
			groupID = &gid
		} else {
			g, err := s.repo.GetGroupBySlug(ctx, slug)
			if err != nil {
				return Customer{}, err
			}
			gid := g.ID
			groupID = &gid
		}
	}

	c, err := s.repo.AdminUpdate(ctx, id, input, groupID)
	if err != nil {
		return Customer{}, err
	}
	s.afterAdminUpdate(ctx, existing, c)

	// Async post-commit - cannot abort
	s.publishDurable(ctx, &CustomerUpdated{
		CustomerID:    c.ID,
		ChannelID:     c.ChannelID,
		Email:         c.Email,
		FirstName:     c.FirstName,
		LastName:      c.LastName,
		IsActive:      c.IsActive,
		ChangedFields: changed,
		Source:        "admin",
		Tags:          updatingEvt.Tags,
	})

	return c, nil
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
// The FK on orders.customer_id blocks it anyway; this check exists for the nicer error message
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.CountOrders(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrCustomerHasOrders
	}
	// Snapshot identity for the post-commit event and the address-keyed cleanup before the row is gone
	pre, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// Sync pre-commit hook - external systems can refuse the delete
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, &CustomerDeleting{
			CustomerID: pre.ID,
			ChannelID:  pre.ChannelID,
			Email:      pre.Email,
		}); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, pre); err != nil {
		return err
	}
	// The row is gone but its refresh tokens live in a separate store with their own 30-day TTL - leaving them would let a deleted account keep minting access tokens
	if rerr := s.customerRefresh.RevokeAll(ctx, id); rerr != nil {
		log.Printf("[auth:warn] revoke deleted customer %s sessions: %v", id, rerr)
	}
	// Signals is a separate database with no FK cascade, so a hard delete leaves the email and identity mappings behind forever - the anonymize path already does this, the delete path never did
	if serr := s.repo.PurgeSignalsIdentity(ctx, id, pre.Email); serr != nil {
		log.Printf("[gdpr:warn] purge signals identity for deleted customer %s: %v", id, serr)
	}
	s.publishDurable(ctx, &CustomerDeleted{
		CustomerID: pre.ID,
		ChannelID:  pre.ChannelID,
		Email:      pre.Email,
	})
	return nil
}

// A disabled account can't authenticate - Authenticate checks is_active, CustomerSessionGuard drops already-issued access tokens, and revoking here kills the 30-day refresh chain so the account can't mint a fresh session
func (s *Service) SetCustomerActive(ctx context.Context, id uuid.UUID, active bool) (Customer, error) {
	c, err := s.repo.SetActive(ctx, id, active)
	if err != nil {
		return Customer{}, err
	}
	if !active {
		if rerr := s.customerRefresh.RevokeAll(ctx, id); rerr != nil {
			log.Printf("[auth:warn] revoke customer %s sessions after disable: %v", id, rerr)
		}
	}
	s.publishUpdated(ctx, c, "isActive")
	return c, nil
}

// IsActive backs auth.CustomerSessionGuard - a disabled or deleted account stops authenticating on the next request instead of riding out its access-token TTL
func (s *Service) IsActive(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.repo.IsActive(ctx, id)
}

// Keeps group slugs usable as stable identifiers in GraphQL inputs and future pricing rules
func validateGroupSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("group slug is required")
	}
	if slug != strings.ToLower(slug) {
		return fmt.Errorf("group slug must be lowercase")
	}
	if strings.ContainsAny(slug, " \t\n") {
		return fmt.Errorf("group slug must not contain whitespace")
	}
	return nil
}

// The note is the exemption certificate reference shown to admins
func (s *Service) SetTaxStatus(ctx context.Context, id uuid.UUID, exempt bool, note string) (Customer, error) {
	return s.repo.SetTaxStatus(ctx, id, exempt, note)
}

// An empty vatID resets the status to unverified
func (s *Service) SetVatID(ctx context.Context, id uuid.UUID, vatID, status string) (Customer, error) {
	if vatID == "" {
		status = "unverified"
	}
	return s.repo.SetVatID(ctx, id, vatID, status)
}
