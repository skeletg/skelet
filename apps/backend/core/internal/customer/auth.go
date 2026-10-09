package customer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

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
	group := s.registeringGroup(ctx, ing)
	var c Customer
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		c, err = r.Create(ctx, CreateCustomerInput{
			ChannelID: channelID, Email: emailAddr, PasswordHash: string(hash),
			FirstName: firstName, LastName: lastName, GroupID: group,
		})
		if err != nil {
			return nil, err
		}
		// Tags from the pre-commit chain carry forward so enrichers can influence async subscribers
		return []events.Event{&CustomerRegistered{
			CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, LastName: c.LastName,
			Source: SourceSelf, Tags: ing.Tags,
		}}, nil
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

// The "email = profile" model signup forms need - a newsletter subscriber becomes targetable (segments, flows, consent) without an account created is true only on a fresh record, so the caller fires CustomerRegistered exactly once A random unusable password hash is stored so the row can never be logged into until a reset sets a real one, which is how the contact later claims the account
// verified marks a fresh contact's address as proven, which only a confirmed double opt-in may claim
func (s *Service) EnsureContact(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool) (Customer, bool, error) {
	return s.EnsureContactFrom(ctx, channelID, email, firstName, verified, SourceSignupForm)
}

func (s *Service) EnsureContactFrom(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool, source string) (Customer, bool, error) {
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
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		c, err = r.Create(ctx, CreateCustomerInput{
			ChannelID: channelID, Email: email, PasswordHash: string(hash), FirstName: firstName, EmailVerified: verified,
		})
		if err != nil {
			return nil, err
		}
		evs := []events.Event{&CustomerRegistered{
			CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email,
			FirstName: c.FirstName, LastName: c.LastName, Source: source,
		}}
		if verified {
			evs = append(evs, &CustomerEmailVerified{CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email})
		}
		return evs, nil
	})
	// A concurrent submit for the same address created it first - that row is the contact
	if errors.Is(err, ErrEmailTaken) {
		c, err = s.repo.GetByStoreAndEmail(ctx, channelID, email)
		return c, false, err
	}
	if err != nil {
		return Customer{}, false, err
	}
	return c, true, nil
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

// Where account mail links land, on the shop's own origin
const (
	VerifyEmailPath    = "/verify-email?token="
	ResetPasswordPath  = "/reset-password?token="
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
	link := storefront + VerifyEmailPath + token
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
	return s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		if err := r.MarkEmailVerified(ctx, c.ID, c.Email); err != nil {
			return nil, err
		}
		return []events.Event{&CustomerEmailVerified{CustomerID: c.ID, ChannelID: c.ChannelID, Email: c.Email}}, nil
	})
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
	link := storefront + ResetPasswordPath + token
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
