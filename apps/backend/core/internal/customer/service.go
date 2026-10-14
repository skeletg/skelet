package customer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// Declared locally so the mail paths take a snapshot rather than the whole channel service
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
	// The roster events go durable through this when wired - see outbox.go
	outbox OutboxEnqueuer
	mail   sync.WaitGroup
}

// Routes account emails through Pulse flows - this service hands off the recipient and one-time link, the flow engine renders and sends synchronously
// A structural interface so customer doesn't import flow (flow imports customer) - nil, or a zero return, falls through to authEmailFallback
type authEmailDispatcher interface {
	// Each returns the number of flows that took delivery - zero means nobody sent the mail
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

// The From header for account mail - the same builder the flow engine uses, so a fallback send is indistinguishable from a flow one
func (st StoreSnapshot) fromHeader() string {
	return email.FromHeader(st.EmailFromName, st.EmailFromAddress)
}

func (s *Service) storeName(ctx context.Context, st StoreSnapshot) string {
	if s.cfg != nil {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &st.ID}
		if name, err := s.cfg.GetString(ctx, channel.ConfigStoreName, scope); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return st.EmailFromName
}

// Optional - nil skips per-channel country/region enforcement on address writes, and phones fall back to the
// default country alone
type CountryValidator interface {
	ValidateAddressForChannel(ctx context.Context, channelID uuid.UUID, countryCode string, region *string) error
	PhoneRulesForChannel(ctx context.Context, channelID uuid.UUID) phone.Rules
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

// The store's phone policy - the country domain's when it's wired, else only its default country to read a
// national number in
func (s *Service) phoneRules(ctx context.Context, channelID uuid.UUID) phone.Rules {
	if s.countries != nil {
		return s.countries.PhoneRulesForChannel(ctx, channelID)
	}
	var r phone.Rules
	if s.cfg != nil {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
		if v, err := s.cfg.GetString(ctx, pathDefaultCountry, scope); err == nil {
			r.Default = v
		}
	}
	return r
}

// A profile phone carries no country of its own, so one typed nationally is read in the store's default country - with none set only international input parses
// Exported so the CSV import can refuse a bad phone before it writes the contact
func (s *Service) NormalizeProfilePhone(ctx context.Context, channelID uuid.UUID, raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	return s.phoneRules(ctx, channelID).NormalizePtr(raw, "")
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

// Runs a write in one transaction with the events it produces - enqueued in that transaction when an outbox is wired,
// published on the bus once it commits otherwise, so a committed write never loses its event and a failed one never sends it
func (s *Service) atomically(ctx context.Context, write func(r *postgresRepository) ([]events.Event, error)) error {
	var evs []events.Event
	err := s.repo.tx.InTransaction(ctx, func(t *sql.Tx) error {
		var err error
		if evs, err = write(s.repo.withTx(t)); err != nil {
			return err
		}
		if s.outbox == nil {
			return nil
		}
		for _, e := range evs {
			if err := s.outbox.Enqueue(ctx, t, e); err != nil {
				return fmt.Errorf("enqueue %s: %w", e.Type(), err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.outbox == nil {
		s.publish(ctx, evs...)
	}
	return nil
}

// The plain bus, for the events that have no transaction to ride and no outbox type
func (s *Service) publish(ctx context.Context, evs ...events.Event) {
	if s.bus == nil {
		return
	}
	for _, e := range evs {
		s.bus.Publish(ctx, e)
	}
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.repo.GetByID(ctx, id)
}

// Batched lookup for the storefront DataLoader - empty input skips the DB, and missing ids come back as absent keys rather than errors
func (s *Service) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Customer, error) {
	return s.repo.GetByIDs(ctx, ids)
}

func (s *Service) GetByIDWithAddresses(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.repo.GetByIDWithAddresses(ctx, id)
}

// found=false with no error for guests with no account - a failed lookup is an error, never a miss
func (s *Service) CustomerIDByEmail(ctx context.Context, channelID uuid.UUID, email string) (uuid.UUID, bool, error) {
	c, err := s.repo.GetByStoreAndEmail(ctx, channelID, NormalizeEmail(email))
	if errors.Is(err, ErrCustomerNotFound) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return c.ID, true, nil
}

// IsActive backs auth.CustomerSessionGuard - a disabled or deleted account stops authenticating on the next request instead of riding out its access-token TTL
func (s *Service) IsActive(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.repo.IsActive(ctx, id)
}

// The shopper's own edit - every change lands with its event, and the names it lists are only the fields that changed
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, input UpdateCustomerInput) (Customer, error) {
	return s.UpdateProfileFrom(ctx, id, input, SourceSelf)
}

// UpdateProfile with the event stamped by whoever made the change, so a bulk import never reads as the shopper's edit
func (s *Service) UpdateProfileFrom(ctx context.Context, id uuid.UUID, input UpdateCustomerInput, source string) (Customer, error) {
	if err := ValidateProfile(input); err != nil {
		return Customer{}, err
	}
	if input.Phone != nil {
		existing, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return Customer{}, err
		}
		if input.Phone, err = s.NormalizeProfilePhone(ctx, existing.ChannelID, input.Phone); err != nil {
			return Customer{}, err
		}
	}
	var updated Customer
	err := s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		existing, err := r.GetByIDForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		merged := UpdateCustomerInput{FirstName: existing.FirstName, LastName: existing.LastName, Phone: existing.Phone}
		var changed []string
		if input.FirstName != "" && input.FirstName != existing.FirstName {
			merged.FirstName = input.FirstName
			changed = append(changed, FieldFirstName)
		}
		if input.LastName != "" && input.LastName != existing.LastName {
			merged.LastName = input.LastName
			changed = append(changed, FieldLastName)
		}
		if input.Phone != nil {
			merged.Phone = input.Phone
			changed = append(changed, FieldPhone)
		}
		if updated, err = r.Update(ctx, id, merged); err != nil {
			return nil, err
		}
		return []events.Event{&CustomerProfileUpdated{CustomerID: id, ChannelID: existing.ChannelID, ChangedFields: changed, Source: source}}, nil
	})
	return updated, err
}
