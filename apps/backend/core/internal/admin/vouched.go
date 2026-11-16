package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// SignInVouched is a password sign-in's checks and events without the password. The admin's own second factor is still
// owed unless the provider proved one
func (s *Service) SignInVouched(ctx context.Context, id uuid.UUID, totpCode string, providerMFA bool) (AdminUser, error) {
	ip, ua := clientAttribution(ctx)
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return AdminUser{}, err
	}
	if s.bus != nil {
		attempting := &auth.LoginAttempting{Subject: auth.SubjectAdmin, Email: user.Email, IP: ip, UserAgent: ua}
		if err := s.bus.PublishSync(ctx, attempting); err != nil {
			s.publishLoginFailed(ctx, user.Email, ip, ua, "rejected_by_subscriber")
			return AdminUser{}, fmt.Errorf("login rejected: %w", err)
		}
	}
	if !user.IsActive {
		s.publishLoginFailed(ctx, user.Email, ip, ua, "account_disabled")
		return AdminUser{}, fmt.Errorf("account is disabled")
	}
	if user.TotpEnabled && !providerMFA {
		if err := s.secondFactor(ctx, &user, totpCode, ip, ua); err != nil {
			return AdminUser{}, err
		}
	}
	if user.Roles, err = s.repo.GetRoles(ctx, user.ID); err != nil {
		return AdminUser{}, fmt.Errorf("load roles: %w", err)
	}
	if user.Permissions, err = s.repo.GetPermissions(ctx, user.ID); err != nil {
		return AdminUser{}, fmt.Errorf("load permissions: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &auth.LoginSucceeded{Subject: auth.SubjectAdmin, SubjectID: user.ID, Email: user.Email, IP: ip, UserAgent: ua})
	}
	return user, nil
}
