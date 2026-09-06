package payment

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// The slim view of channel.Service Sync needs to enumerate channels - declared locally so payment -> channel stays one-way
type ChannelLookup interface {
	List(ctx context.Context) ([]channel.Channel, error)
}

// Provider registration is global (registry.go) - the Service just consults the registry by slug
type Service struct {
	repo       Repository
	tx         *tx.Manager
	channelSvc ChannelLookup
	bus        *events.Bus
	outbox     OutboxEnqueuer
}

func NewService(repo Repository, txMgr *tx.Manager, channelSvc ChannelLookup) *Service {
	return &Service{repo: repo, tx: txMgr, channelSvc: channelSvc}
}

// Optional - when nil, provider webhook results are dropped after the provider returns them
func (s *Service) SetEventBus(b *events.Bus) {
	s.bus = b
}

// OutboxEnqueuer makes provider webhook events durable - the 204 the gateway receives means "the
// event will be applied", so a fire-and-forget publish that a failed subscriber or a crash could
// drop breaks the contract with a gateway that will never resend
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error
}

// Optional like the bus - when set, DispatchWebhook enqueues instead of publishing
func (s *Service) SetOutbox(o OutboxEnqueuer) {
	s.outbox = o
}

// Every type Enqueue may be handed has to be registered, mirroring order.RegisterOutboxTypes
func RegisterOutboxTypes(r interface {
	RegisterType(eventType string, fn func() events.Event)
}) {
	r.RegisterType(EventProviderWebhook, func() events.Event { return &ProviderWebhook{} })
}

// Runs at boot once every plugin's init() has registered its providers, ensuring a payment_methods row per (channel, provider)
// New providers default to is_enabled=false so operators opt in explicitly rather than accidentally sending traffic to a new gateway
// One transaction for all upserts, since a half-finished Sync would leave orphan rows - and Sync never deletes, so dropping a provider from the binary leaves its row and its orders queryable
func (s *Service) Sync(ctx context.Context) error {
	stores, err := s.channelSvc.List(ctx)
	if err != nil {
		return fmt.Errorf("payment.Sync: list channels: %w", err)
	}
	regs := AllProviders()
	if len(stores) == 0 || len(regs) == 0 {
		return nil
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		txRepo := s.repo.WithTx(t)
		for _, store := range stores {
			for _, reg := range regs {
				if _, err := txRepo.UpsertPaymentMethod(ctx, store.ID, reg.Slug, reg.DisplayName); err != nil {
					return fmt.Errorf("payment.Sync: upsert %s on %s: %w", reg.Slug, store.Slug, err)
				}
			}
		}
		return nil
	})
}

// Every registered provider with its payment_methods row hydrated, for the admin "payment methods" page
func (s *Service) ListProvidersForChannel(ctx context.Context, channelID uuid.UUID) ([]ProviderWithMethod, error) {
	methods, err := s.repo.ListPaymentMethodsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	byProvider := make(map[string]PaymentMethod, len(methods))
	for _, m := range methods {
		if m.CountryMode == "specific" {
			countries, err := s.repo.ListCountriesForPaymentMethod(ctx, m.ID)
			if err != nil {
				return nil, err
			}
			m.AllowedCountries = countries
		}
		byProvider[m.ProviderSlug] = m
	}
	regs := AllProviders()
	out := make([]ProviderWithMethod, 0, len(regs))
	for _, reg := range regs {
		out = append(out, ProviderWithMethod{
			Slug:        reg.Slug,
			DisplayName: reg.DisplayName,
			IsOffline:   reg.IsOffline,
			Method:      byProvider[reg.Slug], // zero value if not yet synced
		})
	}
	return out, nil
}

type ProviderWithMethod struct {
	Slug        string
	DisplayName string
	// No third party behind it - cash on delivery, bank transfer, pickup, purchase order
	IsOffline bool
	Method    PaymentMethod // zero-value when sync hasn't run yet
}

// Always true in "all" mode, else membership of the per-method allow-list, case-insensitive - country gate only, callers check IsEnabled separately
func (s *Service) IsMethodAllowedForCountry(ctx context.Context, channelID uuid.UUID, providerSlug, country string) (bool, error) {
	m, err := s.repo.GetPaymentMethod(ctx, channelID, providerSlug)
	if err != nil {
		return false, err
	}
	if m.CountryMode != "specific" {
		return true, nil
	}
	countries, err := s.repo.ListCountriesForPaymentMethod(ctx, m.ID)
	if err != nil {
		return false, err
	}
	for _, c := range countries {
		if strings.EqualFold(c, country) {
			return true, nil
		}
	}
	return false, nil
}

// Enforces that a selected provider is usable at checkout - enabled on the channel and accepting the country
// ErrProviderDisabled when off, ErrProviderNotInstalled when no row exists, and an unknown country fails a "specific" method - Backs order.PaymentValidator
func (s *Service) ValidatePaymentForChannel(ctx context.Context, channelID uuid.UUID, providerSlug, country string) error {
	m, err := s.repo.GetPaymentMethod(ctx, channelID, providerSlug)
	if err != nil {
		return err
	}
	if !m.IsEnabled {
		return ErrProviderDisabled
	}
	if m.CountryMode != "specific" {
		return nil
	}
	countries, err := s.repo.ListCountriesForPaymentMethod(ctx, m.ID)
	if err != nil {
		return err
	}
	for _, c := range countries {
		if strings.EqualFold(c, country) {
			return nil
		}
	}
	return fmt.Errorf("payment: provider %q not available in %s", providerSlug, country)
}

// The channel's enabled methods accepting country - a "specific" mode method drops out unless the country is in its allow-list, case-insensitive
// Backs the storefront paymentMethods(countryCode) query, pre-sorted by sort_order from the repository
func (s *Service) ListEligiblePaymentMethods(ctx context.Context, channelID uuid.UUID, country string) ([]PaymentMethod, error) {
	methods, err := s.repo.ListEnabledPaymentMethodsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethod, 0, len(methods))
	for _, m := range methods {
		// payment_methods rows outlive the plugin that registered them - Sync only upserts, so uninstalling a provider would
		// otherwise keep offering it right up to the ErrProviderNotFound at placement
		if _, err := GetProvider(m.ProviderSlug); err != nil {
			continue
		}
		if m.CountryMode != "specific" {
			out = append(out, m)
			continue
		}
		countries, err := s.repo.ListCountriesForPaymentMethod(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		allowed := false
		for _, c := range countries {
			if strings.EqualFold(c, country) {
				allowed = true
				break
			}
		}
		if allowed {
			m.AllowedCountries = countries
			out = append(out, m)
		}
	}
	return out, nil
}

// Mirrors shipping's ReplaceMethodCountries
func (s *Service) SetPaymentMethodCountries(ctx context.Context, channelID uuid.UUID, providerSlug, mode string, countries []string) error {
	if _, err := GetProvider(providerSlug); err != nil {
		return err
	}
	m, err := s.repo.SetPaymentMethodCountryMode(ctx, channelID, providerSlug, mode)
	if err != nil {
		return err
	}
	if err := s.repo.ClearPaymentMethodCountries(ctx, m.ID); err != nil {
		return err
	}
	for _, c := range countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if err := s.repo.AddCountryToPaymentMethod(ctx, m.ID, c); err != nil {
			return err
		}
	}
	return nil
}

// ErrProviderNotInstalled when no row exists, i.e - Sync hasn't run for this provider - admins shouldn't see the option, but the API guards against direct mutation
func (s *Service) SetEnabled(ctx context.Context, channelID uuid.UUID, providerSlug string, enabled bool) (PaymentMethod, error) {
	if _, err := GetProvider(providerSlug); err != nil {
		return PaymentMethod{}, err
	}
	return s.repo.SetPaymentMethodEnabled(ctx, channelID, providerSlug, enabled)
}

// A thin pass-through with an event hook on success - the caller validates inputs like amount and currency
func (s *Service) Authorize(ctx context.Context, providerSlug string, req AuthorizeRequest) (AuthorizeResult, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return AuthorizeResult{}, err
	}
	method, err := s.repo.GetPaymentMethod(ctx, req.ChannelID, providerSlug)
	if err != nil {
		return AuthorizeResult{}, err
	}
	if !method.IsEnabled {
		return AuthorizeResult{}, ErrProviderDisabled
	}
	res, err := reg.Provider.Authorize(ctx, req)
	if err != nil {
		return AuthorizeResult{}, err
	}
	// A gateway that parked the authorization on a client-side step (3DS) has NOT authorized anything yet - PaymentAuthorized only fires once finalizeOrderPayment verifies completion
	if res.Action.Required() {
		return res, nil
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PaymentAuthorized{
			OrderID:           req.OrderID,
			ProviderSlug:      providerSlug,
			ProviderReference: res.ProviderReference,
			AmountCents:       req.AmountCents,
			CurrencyCode:      req.CurrencyCode,
		})
	}
	return res, nil
}

// ErrProviderDisabled when the method is off for the channel, and an error when the provider lacks the capability
func (s *Service) Vault(ctx context.Context, providerSlug string, req VaultRequest) (VaultedMethod, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return VaultedMethod{}, err
	}
	vaulter, ok := reg.Provider.(Vaulter)
	if !ok || reg.IsOffline {
		return VaultedMethod{}, fmt.Errorf("provider %q does not support saved payment methods", providerSlug)
	}
	method, err := s.repo.GetPaymentMethod(ctx, req.ChannelID, providerSlug)
	if err != nil {
		return VaultedMethod{}, err
	}
	if !method.IsEnabled {
		return VaultedMethod{}, ErrProviderDisabled
	}
	return vaulter.VaultPaymentMethod(ctx, req)
}

// A missing capability is an error here, but callers may choose to ignore not-found style provider errors
func (s *Service) DeleteVaulted(ctx context.Context, providerSlug string, req VaultDeleteRequest) error {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return err
	}
	vaulter, ok := reg.Provider.(Vaulter)
	if !ok {
		return fmt.Errorf("provider %q does not support saved payment methods", providerSlug)
	}
	return vaulter.DeleteVaultedPaymentMethod(ctx, req)
}

// Authorizes AND captures in one go against a vaulted method, publishing no order-payment events since callers like subscription renewals have no order to settle
// The vaulted token rides req.VaultedToken - the typed slot the provider charges the stored method from
func (s *Service) ChargeOffSession(ctx context.Context, providerSlug string, req AuthorizeRequest) (string, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return "", err
	}
	if reg.IsOffline {
		return "", fmt.Errorf("provider %q cannot charge off-session", providerSlug)
	}
	method, err := s.repo.GetPaymentMethod(ctx, req.ChannelID, providerSlug)
	if err != nil {
		return "", err
	}
	if !method.IsEnabled {
		return "", ErrProviderDisabled
	}
	auth, err := reg.Provider.Authorize(ctx, req)
	if err != nil {
		return "", err
	}
	if auth.Action.Required() {
		// No client is present to run a challenge, so treat it as declined and let dunning schedule a retry - the issuer may approve a later attempt
		return "", fmt.Errorf("gateway requires customer action (3DS) - cannot complete off-session")
	}
	if _, err := reg.Provider.Capture(ctx, CaptureRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: auth.ProviderReference,
		CurrencyCode:      req.CurrencyCode,
		IdempotencyKey:    "offsession-" + req.OrderID.String(),
	}); err != nil {
		return "", fmt.Errorf("capture after authorize: %w", err)
	}
	return auth.ProviderReference, nil
}

// Asks a gateway where a payment stands, for reconciliation that can't wait on a webhook - a
// redirect or BNPL customer who closes the tab leaves an order no notification will ever settle
// ErrSyncUnsupported when the provider doesn't implement the capability, so a caller sweeping many
// orders can skip rather than treat it as a failure
func (s *Service) SyncPayment(ctx context.Context, providerSlug string, req SyncPaymentRequest) (SyncPaymentResult, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return SyncPaymentResult{}, err
	}
	syncer, ok := reg.Provider.(PaymentSynchronizer)
	if !ok {
		return SyncPaymentResult{}, ErrSyncUnsupported
	}
	return syncer.SyncPayment(ctx, req)
}

// Asks the provider whether an authorization parked on a client-side action (3DS/SCA) has since completed, emitting the same PaymentAuthorized the direct path does
// Gateway truth only - the caller's claim is never trusted
func (s *Service) FinalizeAuthorization(ctx context.Context, providerSlug string, req VerifyAuthorizationRequest) (bool, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return false, err
	}
	// Offline tenders and providers with no SCA flow have nothing to verify - report "not authorized" rather than erroring, so callers can probe idempotently
	verifier, ok := reg.Provider.(AuthorizationVerifier)
	if !ok || reg.IsOffline {
		return false, nil
	}
	res, err := verifier.VerifyAuthorization(ctx, req)
	if err != nil {
		return false, err
	}
	if !res.Authorized {
		return false, nil
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PaymentAuthorized{
			OrderID:           req.OrderID,
			ProviderSlug:      providerSlug,
			ProviderReference: res.ProviderReference,
			AmountCents:       req.AmountCents,
			CurrencyCode:      req.CurrencyCode,
		})
	}
	return true, nil
}

// Idempotent at the provider's discretion - the core does not deduplicate calls
func (s *Service) Capture(ctx context.Context, providerSlug string, req CaptureRequest) (CaptureResult, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return CaptureResult{}, err
	}
	res, err := reg.Provider.Capture(ctx, req)
	if err != nil {
		return CaptureResult{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PaymentCaptured{
			OrderID:           req.OrderID,
			ProviderSlug:      providerSlug,
			ProviderReference: res.ProviderReference,
			AmountCents:       req.AmountCents,
		})
	}
	return res, nil
}

func (s *Service) Refund(ctx context.Context, providerSlug string, req RefundRequest) (RefundResult, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return RefundResult{}, err
	}
	res, err := reg.Provider.Refund(ctx, req)
	if err != nil {
		return RefundResult{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PaymentRefunded{
			OrderID:           req.OrderID,
			ProviderSlug:      providerSlug,
			ProviderReference: res.ProviderReference,
			AmountCents:       req.AmountCents,
			Reason:            req.Reason,
		})
	}
	return res, nil
}

func (s *Service) Void(ctx context.Context, providerSlug string, req VoidRequest) (VoidResult, error) {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return VoidResult{}, err
	}
	res, err := reg.Provider.Void(ctx, req)
	if err != nil {
		return VoidResult{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PaymentVoided{
			OrderID:           req.OrderID,
			ProviderSlug:      providerSlug,
			ProviderReference: req.ProviderReference,
		})
	}
	return res, nil
}

// Routes an incoming request to the provider's HandleWebhook, which verifies signatures with its own configured secret and returns a typed event to publish
// An unknown provider errors (404 upstream) as does a provider rejection (400); returning no error and no event name means "valid but no observable effect", which suits noisy probe and heartbeat webhooks
func (s *Service) DispatchWebhook(ctx context.Context, providerSlug string, req *http.Request) error {
	reg, err := GetProvider(providerSlug)
	if err != nil {
		return err
	}
	result, err := reg.Provider.HandleWebhook(ctx, req)
	if err != nil {
		return err
	}
	if result.EventName == "" {
		return nil
	}
	// The provider chose the event name and subscribers route on it, so a provider can emit any number of payment.* events without the core carrying a typed struct for each
	evt := &ProviderWebhook{
		ProviderSlug:      providerSlug,
		OrderID:           result.OrderID,
		EventName:         strings.TrimSpace(result.EventName),
		ProviderReference: result.ProviderReference,
		AmountCents:       result.AmountCents,
		CurrencyCode:      result.CurrencyCode,
	}
	// durable path first: the gateway's 2xx means "will be applied", and it will never resend - a
	// bus publish here could vanish on a subscriber error or a crash between response and settle
	if s.outbox != nil {
		return s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
			return s.outbox.Enqueue(ctx, sqlTx, evt)
		})
	}
	if s.bus == nil {
		return nil
	}
	s.bus.Publish(ctx, evt)
	return nil
}
