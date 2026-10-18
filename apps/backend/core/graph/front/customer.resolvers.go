package front

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Reachable only through myProfile (customer JWT), so no extra auth check here
func (r *customerResolver) Consents(ctx context.Context, obj *model.Customer) ([]*model.CustomerConsent, error) {
	customerID, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, invalidID("customer")
	}
	// Same guard as StoreCreditBalance below - every Customer today resolves to the caller's own, so this is
	// belt-and-braces against a future path that hands back somebody else's
	if current := channel.OptionalCustomerID(ctx); current == nil || *current != customerID {
		return []*model.CustomerConsent{}, nil
	}
	list, err := r.ConsentSvc.ListForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CustomerConsent, len(list))
	for i, c := range list {
		out[i] = toCustomerConsentModel(c)
	}
	return out, nil
}

// Customer objects only surface via me/auth flows, but a mismatched id still returns 0 rather than leaking another customer's balance
func (r *customerResolver) StoreCreditBalance(ctx context.Context, obj *model.Customer) (int32, error) {
	customerID, err := uuid.Parse(obj.ID)
	if err != nil {
		return 0, invalidID("customer")
	}
	if current := channel.OptionalCustomerID(ctx); current == nil || *current != customerID {
		return 0, nil
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return 0, nil
	}
	ch, err := r.ChannelSvc.GetByID(ctx, *chID)
	if err != nil {
		return 0, err
	}
	bal, err := r.StoreCreditSvc.Balance(ctx, customerID, ch.CurrencyCode)
	if err != nil {
		return 0, err
	}
	return int32(bal), nil
}

// A taken address answers like a registration that withheld its session, and its owner gets a note instead - the response never says which happened
func (r *mutationResolver) CustomerRegister(ctx context.Context, input model.CustomerRegisterInput) (*model.CustomerAuthPayload, error) {
	if err := r.checkAuthRate(ctx, "register"); err != nil {
		return nil, err
	}
	// Per address too, since each attempt on a taken one mails its owner
	if err := r.checkAuthRateKey(ctx, "register_email", customer.NormalizeEmail(input.Email)); err != nil {
		return nil, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return nil, storeRequired()
	}
	c, err := r.CustomerSvc.Register(ctx, store.ID, input.Email, input.Password, input.FirstName, input.LastName, input.MarketingOptIn)
	if errors.Is(err, customer.ErrEmailTaken) {
		return sessionWithheldPayload(), nil
	}
	if err != nil {
		return nil, err
	}
	// Referral attach is best-effort - a bad or self-referencing code must never fail the registration itself
	if input.ReferralCode != nil && *input.ReferralCode != "" {
		if aerr := r.ReferralSvc.Attach(ctx, c.ID, *input.ReferralCode); aerr != nil {
			slog.Warn("referral attach failed", "err", aerr)
		}
	}
	// Withhold the session when the channel gates on a verified address - Authenticate enforces this, and minting a token here would let anyone register with someone else's address and be signed in as a fresh account on it
	if r.CustomerSvc.RequiresEmailVerification(ctx, store.ID) {
		return sessionWithheldPayload(), nil
	}
	return r.customerAuthPayload(ctx, c)
}

func (r *mutationResolver) CustomerLogin(ctx context.Context, input model.CustomerLoginInput) (*model.CustomerAuthPayload, error) {
	// Brute-force guard keyed per-IP AND per-email - the per-email half was missing, so a botnet rotating IPs could stuff one account; the failure-count lockout below spans rate windows
	email := customer.NormalizeEmail(input.Email)
	if err := r.checkAuthRate(ctx, "login"); err != nil {
		return nil, err
	}
	if err := r.checkAuthRateKey(ctx, "login_email", email); err != nil {
		return nil, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return nil, storeRequired()
	}
	throttleScope := "customer:" + store.ID.String() // per (store, email) so one store's failures don't lock the same email on another
	if r.LoginThrottle.Locked(ctx, throttleScope, email) {
		return nil, auth.ErrAccountLocked
	}
	c, err := r.CustomerSvc.Authenticate(ctx, store.ID, input.Email, input.Password)
	if err != nil {
		// Only a wrong password counts toward the lockout - a storage fault or a correct password on a gated account is not a guess
		if errors.Is(err, customer.ErrInvalidCredentials) {
			r.LoginThrottle.RecordFailure(ctx, throttleScope, email)
		}
		return nil, err
	}
	r.LoginThrottle.Clear(ctx, throttleScope, email)
	return r.customerAuthPayload(ctx, c)
}

// Refresh tokens are bound to their home channel - refusing to rotate one whose owner registered elsewhere keeps a stolen cookie from becoming a full session on a store where that customer doesn't exist
func (r *mutationResolver) CustomerRefreshToken(ctx context.Context, token string) (*model.CustomerAuthPayload, error) {
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return nil, auth.ErrInvalidRefreshToken
	}
	// Checked before the rotate too, so a token presented on the wrong store is refused without being spent
	if owner, verr := r.CustomerRefreshStore.Validate(ctx, token); verr == nil {
		if c, gerr := r.CustomerSvc.GetByID(ctx, owner); gerr == nil && c.ChannelID != store.ID {
			return nil, auth.ErrInvalidRefreshToken
		}
	}
	// Atomic single-use consume with a short reuse grace, so an SSR storefront's concurrent prefetch refreshes coalesce onto one successor instead of racing and revoking each other (see Rotate)
	newRefresh, customerID, err := r.CustomerRefreshStore.Rotate(ctx, token)
	if err != nil {
		return nil, auth.ErrInvalidRefreshToken
	}
	c, err := r.CustomerSvc.GetByID(ctx, customerID)
	if err != nil {
		// "Customer row vanished" folds into the opaque invalid-token path - the caller doesn't get to tell a deleted account from a bogus token
		return nil, auth.ErrInvalidRefreshToken
	}
	if c.ChannelID != store.ID {
		return nil, auth.ErrInvalidRefreshToken
	}
	if !c.IsActive {
		return nil, customer.ErrAccountDisabled
	}
	return r.customerAuthPayloadWithRefresh(ctx, c, newRefresh)
}

func (r *mutationResolver) CustomerLogout(ctx context.Context, token string) (bool, error) {
	if err := r.CustomerRefreshStore.Revoke(ctx, token); err != nil {
		return false, fmt.Errorf("logout: %w", err)
	}
	return true, nil
}

func (r *mutationResolver) UpdateProfile(ctx context.Context, firstName *string, lastName *string, phone *string) (*model.Customer, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	input := customer.UpdateCustomerInput{Phone: phone}
	if firstName != nil {
		input.FirstName = *firstName
	}
	if lastName != nil {
		input.LastName = *lastName
	}
	c, err := r.CustomerSvc.UpdateProfile(ctx, claims.CustomerID, input)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// A stolen access token is all it takes to guess at the current password here, so wrong guesses count toward the same lockout login uses
func (r *mutationResolver) ChangeCustomerPassword(ctx context.Context, currentPassword string, newPassword string) (bool, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return false, err
	}
	if err := r.checkAuthRate(ctx, "change_password"); err != nil {
		return false, err
	}
	scope, key := "customer_password:"+claims.ChannelID.String(), claims.CustomerID.String()
	if r.LoginThrottle.Locked(ctx, scope, key) {
		return false, auth.ErrAccountLocked
	}
	if err := r.CustomerSvc.ChangePassword(ctx, claims.CustomerID, currentPassword, newPassword); err != nil {
		if errors.Is(err, customer.ErrIncorrectCurrentPassword) {
			r.LoginThrottle.RecordFailure(ctx, scope, key)
		}
		return false, err
	}
	r.LoginThrottle.Clear(ctx, scope, key)
	return true, nil
}

// Single-use, and unauthenticated - the token is the authorisation
func (r *mutationResolver) VerifyEmail(ctx context.Context, token string) (bool, error) {
	if err := r.CustomerSvc.VerifyEmail(ctx, token); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ResendVerificationEmail(ctx context.Context) (bool, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return false, err
	}
	if err := r.checkAuthRateKey(ctx, "resend_verification_email", claims.CustomerID.String()); err != nil {
		return false, err
	}
	if err := r.CustomerSvc.ResendVerificationEmail(ctx, claims.CustomerID); err != nil {
		return false, err
	}
	return true, nil
}

// Rate-limited per IP and per address like the reset request, since each call can cost an outbound email
func (r *mutationResolver) ResendVerification(ctx context.Context, email string) (bool, error) {
	if err := r.checkAuthRate(ctx, "resend_verification"); err != nil {
		return false, err
	}
	if err := r.checkAuthRateKey(ctx, "resend_verification_email", customer.NormalizeEmail(email)); err != nil {
		return false, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return false, storeRequired()
	}
	if err := r.CustomerSvc.ResendVerificationByEmail(ctx, store.ID, email); err != nil {
		return false, err
	}
	return true, nil
}

// Always returns true so attackers can't enumerate accounts
func (r *mutationResolver) RequestPasswordReset(ctx context.Context, email string) (bool, error) {
	// Rate-limited tightly because the abuse vector is email-bombing a victim address - the response tells an attacker nothing, but each call costs the storefront an outbound email
	if err := r.checkAuthRate(ctx, "password_reset"); err != nil {
		return false, err
	}
	// Also bucketed per target email so a distributed many-IP bomb of one address is capped - keyed regardless of whether the address is registered, so it adds no enumeration signal
	// Normalized, or cycling Victim@x.com / VICTIM@x.com / "victim@x.com " buys a fresh bucket per casing against a mailbox that receives all of them
	if err := r.checkAuthRateKey(ctx, "password_reset_email", customer.NormalizeEmail(email)); err != nil {
		return false, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return false, storeRequired()
	}
	if err := r.CustomerSvc.RequestPasswordReset(ctx, store.ID, email); err != nil {
		return false, err
	}
	return true, nil
}

// Revokes every live refresh session for the customer, and the token only lands its write on the store the customer registered on - cross-store calls get the same opaque error as an expired one
func (r *mutationResolver) ResetPassword(ctx context.Context, token string, newPassword string) (bool, error) {
	if err := r.checkRate(ctx, "password_reset_consume"); err != nil {
		return false, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return false, customer.ErrResetLinkInvalid
	}
	if err := r.CustomerSvc.ResetPassword(ctx, store.ID, token, newPassword); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AddAddress(ctx context.Context, input model.AddressInput) (*model.Address, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	addr, err := r.CustomerSvc.AddAddress(ctx, claims.CustomerID, toAddressInput(input))
	if err != nil {
		return nil, err
	}
	return toAddressModel(addr), nil
}

func (r *mutationResolver) UpdateAddress(ctx context.Context, id string, input model.AddressInput) (*model.Address, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	addressID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("address")
	}
	addr, err := r.CustomerSvc.UpdateAddress(ctx, claims.CustomerID, addressID, toAddressInput(input))
	if err != nil {
		return nil, err
	}
	return toAddressModel(addr), nil
}

func (r *mutationResolver) DeleteAddress(ctx context.Context, id string) (bool, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return false, err
	}
	addressID, err := uuid.Parse(id)
	if err != nil {
		return false, invalidID("address")
	}
	if err := r.CustomerSvc.DeleteAddress(ctx, claims.CustomerID, addressID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetDefaultBillingAddress(ctx context.Context, id string) (*model.Customer, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	addressID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("address")
	}
	c, err := r.CustomerSvc.SetDefaultBillingAddress(ctx, claims.CustomerID, addressID)
	if err != nil {
		return nil, err
	}
	c, err = r.CustomerSvc.GetByIDWithAddresses(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

func (r *mutationResolver) SetDefaultShippingAddress(ctx context.Context, id string) (*model.Customer, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	addressID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("address")
	}
	c, err := r.CustomerSvc.SetDefaultShippingAddress(ctx, claims.CustomerID, addressID)
	if err != nil {
		return nil, err
	}
	c, err = r.CustomerSvc.GetByIDWithAddresses(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return toCustomerModel(c), nil
}

// source is hardcoded to "profile" - this path is only ever the account page
func (r *mutationResolver) SetConsent(ctx context.Context, kind string, optedIn bool) (*model.CustomerConsent, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	c, err := r.ConsentSvc.SetConsent(ctx, claims.CustomerID, kind, optedIn, consent.SourceProfile)
	if err != nil {
		return nil, err
	}
	return toCustomerConsentModel(c), nil
}

// The points debit and the credit grant land in one transaction
func (r *mutationResolver) RedeemLoyaltyPoints(ctx context.Context, points int32) (*model1.LoyaltyRedeemResult, error) {
	customerID := channel.OptionalCustomerID(ctx)
	if customerID == nil {
		return nil, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to redeem points")
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return nil, storeRequired()
	}
	ch, err := r.ChannelSvc.GetByID(ctx, *chID)
	if err != nil {
		return nil, err
	}
	granted, err := r.LoyaltySvc.Redeem(ctx, *customerID, int64(points), *chID, ch.CurrencyCode)
	if err != nil {
		return nil, err
	}
	bal, err := r.LoyaltySvc.Balance(ctx, *customerID)
	if err != nil {
		return nil, err
	}
	credit, err := r.StoreCreditSvc.Balance(ctx, *customerID, ch.CurrencyCode)
	if err != nil {
		return nil, err
	}
	return &model1.LoyaltyRedeemResult{
		BalancePoints:      int32(bal),
		StoreCreditBalance: int32(credit),
		GrantedCents:       int32(granted),
	}, nil
}

func (r *queryResolver) MyDataExport(ctx context.Context) (string, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return "", err
	}
	bundle, err := r.DSAR.BuildSelfServe(ctx, claims.CustomerID)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (r *queryResolver) MyProfile(ctx context.Context) (*model.Customer, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	c, err := r.CustomerSvc.GetByIDWithAddresses(ctx, claims.CustomerID)
	if err != nil {
		return nil, apperr.New("CUSTOMER_NOT_FOUND", apperr.ClassNotFound, "customer not found")
	}
	return toCustomerModel(c), nil
}

func (r *queryResolver) ConsentDefinitions(ctx context.Context) ([]*model.ConsentDefinition, error) {
	defs := consent.AllDefinitions()
	out := make([]*model.ConsentDefinition, len(defs))
	for i, d := range defs {
		out[i] = toConsentDefinitionModel(d)
	}
	return out, nil
}

// Balance is 0 for guests - the program config comes back either way
func (r *queryResolver) LoyaltyStatus(ctx context.Context) (*model1.LoyaltyStatus, error) {
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return nil, storeRequired()
	}
	prog, err := r.LoyaltySvc.ProgramFor(ctx, *chID)
	if err != nil {
		return nil, err
	}
	out := &model1.LoyaltyStatus{
		Enabled:             prog.Enabled,
		EarnPointsPerUnit:   int32(prog.EarnPointsPerUnit),
		RedeemCentsPerPoint: int32(prog.RedeemCentsPerPoint),
	}
	if customerID := channel.OptionalCustomerID(ctx); customerID != nil {
		bal, err := r.LoyaltySvc.Balance(ctx, *customerID)
		if err != nil {
			return nil, err
		}
		out.BalancePoints = int32(bal)
	}
	return out, nil
}

// The share code is minted on first read
func (r *queryResolver) MyReferral(ctx context.Context) (*model1.ReferralInfo, error) {
	customerID := channel.OptionalCustomerID(ctx)
	if customerID == nil {
		return nil, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to see your referral link")
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return nil, storeRequired()
	}
	prog, err := r.ReferralSvc.ProgramFor(ctx, *chID)
	if err != nil {
		return nil, err
	}
	code, err := r.ReferralSvc.CodeFor(ctx, *customerID)
	if err != nil {
		return nil, err
	}
	stats, err := r.ReferralSvc.StatsFor(ctx, *customerID)
	if err != nil {
		return nil, err
	}
	return &model1.ReferralInfo{
		Enabled:             prog.Enabled,
		Code:                code,
		ReferrerRewardCents: int32(prog.ReferrerRewardCents),
		RefereeRewardCents:  int32(prog.RefereeRewardCents),
		ReferredCount:       int32(stats.Referred),
		RewardedCount:       int32(stats.Rewarded),
		EarnedCents:         int32(stats.EarnedCents),
	}, nil
}

func (r *Resolver) Customer() CustomerResolver { return &customerResolver{r} }

type customerResolver struct{ *Resolver }
