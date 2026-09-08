package admin

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/secrets"
)

type Service struct {
	repo         Repository
	bus          *events.Bus
	cipher       *secrets.Cipher          // encrypts TOTP secrets at rest - nil disables 2FA setup
	resetStore   *auth.PasswordResetStore // reset plumbing below is nil until SetPasswordReset, and RequestPasswordReset stays a safe no-op while it is
	refresh      *auth.RefreshStore       // admin refresh store, for RevokeAll on reset
	emailer      email.Sender
	config       *config.Service
	dashboardURL string
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b } // optional - omitting it keeps Authenticate silent on the bus

// Wires the self-service reset flow: admin-namespaced token store, admin refresh store (a successful reset revokes sessions), sender, config for the From address, and the dashboard base URL the link points at - RequestPasswordReset is a no-op while unset
func (s *Service) SetPasswordReset(store *auth.PasswordResetStore, refresh *auth.RefreshStore, emailer email.Sender, cfg *config.Service, dashboardURL string) {
	s.resetStore = store
	s.refresh = refresh
	s.emailer = emailer
	s.config = cfg
	s.dashboardURL = strings.TrimRight(dashboardURL, "/")
}

// --- admin users ---

func (s *Service) CreateSuperuser(ctx context.Context, email, password, firstName, lastName string) error { // superusers bypass permission checks regardless of role assignments
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.HashCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.repo.Create(ctx, CreateAdminUserInput{
		Email:        email,
		PasswordHash: string(hash),
		FirstName:    firstName,
		LastName:     lastName,
		IsSuperuser:  true,
	})
	return err
}

func (s *Service) CreateAdmin(ctx context.Context, email, password, firstName, lastName string, roleSlugs []string) (AdminUser, error) {
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return AdminUser{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.HashCost)
	if err != nil {
		return AdminUser{}, fmt.Errorf("hash password: %w", err)
	}
	return s.repo.Create(ctx, CreateAdminUserInput{
		Email:        email,
		PasswordHash: string(hash),
		FirstName:    firstName,
		LastName:     lastName,
		IsSuperuser:  false,
		RoleSlugs:    roleSlugs,
	})
}

// Auth events bracket the check: sync auth.login.attempting runs before any DB read so rate-limit and IP-block plugins short-circuit cheaply, then async auth.login.succeeded / .failed fire after the verdict
// RecordLogin stamps the login and reports the PREVIOUS value - nil means this account had never
// logged in, which is what the first-run introduction keys off. Callers on the login path must treat
// an error as "not first" rather than failing the login
func (s *Service) RecordLogin(ctx context.Context, adminID uuid.UUID) (*time.Time, time.Time, error) {
	return s.repo.RecordLogin(ctx, adminID)
}

func (s *Service) Authenticate(ctx context.Context, email, password, totpCode string) (AdminUser, error) {
	ip, ua := clientAttribution(ctx)
	if s.bus != nil {
		attempting := &auth.LoginAttempting{
			Subject:   auth.SubjectAdmin,
			Email:     email,
			IP:        ip,
			UserAgent: ua,
		}
		if err := s.bus.PublishSync(ctx, attempting); err != nil {
			s.bus.Publish(ctx, &auth.LoginFailed{
				Subject:   auth.SubjectAdmin,
				Email:     email,
				IP:        ip,
				UserAgent: ua,
				Reason:    "rejected_by_subscriber",
			})
			return AdminUser{}, fmt.Errorf("login rejected: %w", err)
		}
	}
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		auth.CompareDummyPassword(password) // equalize timing with the found-but-wrong-password path, anti-enumeration
		s.publishLoginFailed(ctx, email, ip, ua, "invalid_credentials")
		return AdminUser{}, fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.publishLoginFailed(ctx, email, ip, ua, "invalid_credentials")
		return AdminUser{}, fmt.Errorf("invalid credentials")
	}
	if !user.IsActive {
		s.publishLoginFailed(ctx, email, ip, ua, "account_disabled")
		return AdminUser{}, fmt.Errorf("account is disabled")
	}
	if user.TotpEnabled { // a valid password alone never yields a session - missing code gives typed ErrTotpRequired so the UI can prompt and resubmit, wrong code gives a generic failure
		if strings.TrimSpace(totpCode) == "" {
			return AdminUser{}, ErrTotpRequired
		}
		ok, verr := s.verifySecondFactor(ctx, &user, totpCode)
		if verr != nil {
			return AdminUser{}, verr
		}
		if !ok {
			s.publishLoginFailed(ctx, email, ip, ua, "invalid_totp")
			return AdminUser{}, fmt.Errorf("invalid credentials")
		}
	}
	user.Roles, err = s.repo.GetRoles(ctx, user.ID)
	if err != nil {
		return AdminUser{}, fmt.Errorf("load roles: %w", err)
	}
	user.Permissions, err = s.repo.GetPermissions(ctx, user.ID)
	if err != nil {
		return AdminUser{}, fmt.Errorf("load permissions: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &auth.LoginSucceeded{
			Subject:   auth.SubjectAdmin,
			SubjectID: user.ID,
			Email:     email,
			IP:        ip,
			UserAgent: ua,
		})
	}
	return user, nil
}

func (s *Service) publishLoginFailed(ctx context.Context, email, ip, ua, reason string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &auth.LoginFailed{
		Subject:   auth.SubjectAdmin,
		Email:     email,
		IP:        ip,
		UserAgent: ua,
		Reason:    reason,
	})
}

// Best-effort, off the request the HTTP layer stashed in context - empty strings on CLI and test paths that have none
func clientAttribution(ctx context.Context) (ip, ua string) {
	r, ok := auth.RequestFromContext(ctx)
	if !ok || r == nil {
		return "", ""
	}
	return auth.ClientIP(r), r.UserAgent()
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (AdminUser, error) {
	return s.repo.GetByIDWithRoles(ctx, id)
}

func (s *Service) GetByIDWithRoles(ctx context.Context, id uuid.UUID) (AdminUser, error) { // alias of GetByID, for call sites that want the loaded state spelled out
	return s.repo.GetByIDWithRoles(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]AdminUser, error) {
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateAdminUserInput) (AdminUser, error) { // RoleSlugs nil means no change, an empty slice clears all roles
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("admin not found")
	}
	mergedIsActive := existing.IsActive
	if input.IsActive != nil {
		mergedIsActive = *input.IsActive
	}
	merged := UpdateAdminUserInput{
		FirstName: existing.FirstName,
		LastName:  existing.LastName,
		IsActive:  &mergedIsActive,
		RoleSlugs: input.RoleSlugs,
	}
	if input.FirstName != "" {
		merged.FirstName = input.FirstName
	}
	if input.LastName != "" {
		merged.LastName = input.LastName
	}
	return s.repo.Update(ctx, id, merged)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error { // role assignments cascade
	return s.repo.Delete(ctx, id)
}

func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("admin not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("incorrect current password")
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
	if s.refresh != nil { // matches the reset path - a password change has to evict every other session or it doesn't actually lock anyone out
		if err := s.refresh.RevokeAll(ctx, id); err != nil {
			log.Printf("[auth:warn] revoke admin %s sessions after password change: %v", id, err)
		}
	}
	return nil
}

// Enumeration-safe by design: nil comes back whether or not the email maps to an active admin, so a nil error does NOT mean an email was sent and callers must not reveal the outcome
func (s *Service) RequestPasswordReset(ctx context.Context, emailAddr string) error {
	emailAddr = strings.TrimSpace(emailAddr)
	if s.resetStore == nil || s.emailer == nil || emailAddr == "" {
		return nil // flow isn't wired
	}
	ip, _ := clientAttribution(ctx) // feeds the sync veto hook below, matching login and the customer flow - a veto stays silent so it can't leak whether the address exists
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, &auth.PasswordResetRequesting{
			Subject: auth.SubjectAdmin,
			Email:   emailAddr,
			IP:      ip,
		}); err != nil {
			return nil
		}
	}
	user, err := s.repo.GetByEmail(ctx, emailAddr)
	if err != nil || !user.IsActive {
		return nil // unknown or disabled: stay silent
	}
	token, err := s.resetStore.Create(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("create reset token: %w", err)
	}
	from, displayName := s.fromAddress(ctx)
	subject, html, text, err := email.RenderPasswordResetEmail(email.PasswordResetEmailData{
		FirstName: user.FirstName,
		Link:      s.dashboardURL + "/reset-password?token=" + token,
		StoreName: displayName,
		ExpiresIn: humanTTL(s.resetStore.TTL()),
	})
	if err != nil {
		return fmt.Errorf("render reset email: %w", err)
	}
	if err := s.emailer.Send(ctx, email.Message{
		From: from, To: user.Email, Subject: subject, HTMLBody: html, TextBody: text,
	}); err != nil {
		return fmt.Errorf("send reset email: %w", err)
	}
	return nil
}

// Redeems a single-use reset token, sets the new password, then revokes every existing admin session so a leaked-then-reset account can't stay logged in elsewhere
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if s.resetStore == nil {
		return fmt.Errorf("password reset is not enabled")
	}
	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	adminID, err := s.resetStore.Validate(ctx, token)
	if err != nil {
		return err // "invalid or expired reset link"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), auth.HashCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.repo.UpdatePassword(ctx, adminID, string(hash)); err != nil {
		return err
	}
	if s.refresh != nil {
		if err := s.refresh.RevokeAll(ctx, adminID); err != nil {
			return fmt.Errorf("password reset but session revoke failed: %w", err) // the reset stands, but say so - the admin believes old sessions are dead when they may not be
		}
	}
	_ = s.resetStore.Revoke(ctx, token)
	if s.bus != nil {
		user, _ := s.repo.GetByID(ctx, adminID)
		s.bus.Publish(ctx, &auth.PasswordResetCompleted{
			Subject:   auth.SubjectAdmin,
			SubjectID: adminID,
			Email:     user.Email,
		})
	}
	return nil
}

// Reads default-scope contact config, falling back to a safe default - displayName doubles as the store name in the email body
func (s *Service) fromAddress(ctx context.Context) (from, displayName string) {
	addr, name := "no-reply@skelet.local", ""
	if s.config != nil {
		if a, _ := s.config.GetString(ctx, "general/contact/email_from_address", config.DefaultScope); strings.TrimSpace(a) != "" {
			addr = strings.TrimSpace(a)
		}
		n, _ := s.config.GetString(ctx, "general/contact/email_from_name", config.DefaultScope)
		name = strings.TrimSpace(n)
	}
	if name == "" {
		name = "Skelet"
	}
	return name + " <" + addr + ">", name
}

func humanTTL(d time.Duration) string { // reset-token lifetime for the email body, e.g. "30 minutes"
	m := int(d.Minutes())
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

// Every resource registers a `<resource>.view` / `<resource>.manage` pair where manage is a strict superset, so a foo.view check also passes on foo.manage - manage checks return as-is, view does NOT imply manage
// Implements auth.PermissionChecker off the admin's role assignments - superuser bypass happens in the auth guard before this is reached
func (s *Service) HasPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error) {
	snap, err := s.grantsFor(ctx, adminID)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(auth.EffectiveSlugs(slug), snap.HasAny) {
		return true, nil
	}
	return false, nil
}

func (s *Service) HasGlobalPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error) { // true when an assigned role grants slug at no channel and no locale
	snap, err := s.grantsFor(ctx, adminID)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(auth.EffectiveSlugs(slug), snap.HasGlobal) {
		return true, nil
	}
	return false, nil
}

// True when an assigned role grants slug at the requested channel and optional locale
func (s *Service) HasChannelPermission(ctx context.Context, adminID uuid.UUID, slug string, channelID uuid.UUID, locale *string) (bool, error) {
	snap, err := s.grantsFor(ctx, adminID)
	if err != nil {
		return false, err
	}
	for _, candidate := range auth.EffectiveSlugs(slug) {
		if snap.HasStore(candidate, channelID, locale) {
			return true, nil
		}
	}
	return false, nil
}

// A view-suffixed slug unions the channels its view and manage grants permit, since manage implies view
func (s *Service) ListPermittedChannelIDs(ctx context.Context, adminID uuid.UUID, slug string) ([]uuid.UUID, error) {
	snap, err := s.grantsFor(ctx, adminID)
	if err != nil {
		return nil, err
	}
	candidates := auth.EffectiveSlugs(slug)
	if len(candidates) == 1 {
		return snap.PermittedChannelIDs(candidates[0]), nil
	}
	seen := make(map[uuid.UUID]struct{})
	var merged []uuid.UUID
	for _, candidate := range candidates {
		for _, id := range snap.PermittedChannelIDs(candidate) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	return merged, nil
}

func (s *Service) ListPermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	return s.repo.ListPermissionGroups(ctx)
}

// --- roles ---

func (s *Service) CreateRole(ctx context.Context, input CreateRoleInput) (Role, error) {
	if err := validateRoleSlug(input.Slug); err != nil {
		return Role{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Role{}, fmt.Errorf("role name is required")
	}
	return s.repo.CreateRole(ctx, input)
}

func (s *Service) UpdateRole(ctx context.Context, id uuid.UUID, input UpdateRoleInput) (Role, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Role{}, fmt.Errorf("role name cannot be empty")
	}
	return s.repo.UpdateRole(ctx, id, input)
}

func (s *Service) DeleteRole(ctx context.Context, id uuid.UUID) error { // ErrRoleInUse while the role still has members - reassign them first
	return s.repo.DeleteRole(ctx, id)
}

func (s *Service) GetRole(ctx context.Context, id uuid.UUID) (Role, error) {
	return s.repo.GetRoleByID(ctx, id)
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func validateRoleSlug(slug string) error { // keeps slugs usable as stable identifiers in GraphQL inputs and CLI prompts
	if slug == "" {
		return fmt.Errorf("role slug is required")
	}
	if slug != strings.ToLower(slug) {
		return fmt.Errorf("role slug must be lowercase")
	}
	if strings.ContainsAny(slug, " \t\n") {
		return fmt.Errorf("role slug must not contain whitespace")
	}
	return nil
}
