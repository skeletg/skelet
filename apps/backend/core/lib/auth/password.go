package auth

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

const (
	MinPasswordLen   = 8
	MaxPasswordBytes = 72 // bcrypt's truncation point - it silently ignores bytes past 72, so a longer password would hash a quietly-shortened secret
	HashCost         = 12
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("skelet-login-timing-equalizer"), HashCost) // burns the same CPU as a real verify on the account-not-found login path, so response time doesn't leak whether an email is registered

func CompareDummyPassword(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

var (
	ErrWeakPassword    = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong = fmt.Errorf("password must be at most %d bytes", MaxPasswordBytes)

	// What a spent, expired, revoked or foreign one-time link reads as - one message, so a link never says which
	ErrResetLinkInvalid        = errors.New("invalid or expired reset link")
	ErrVerificationLinkInvalid = errors.New("invalid or expired verification link")
)

func init() {
	apperr.RegisterSentinel(ErrWeakPassword, "WEAK_PASSWORD", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPasswordTooLong, "PASSWORD_TOO_LONG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrResetLinkInvalid, "RESET_LINK_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrVerificationLinkInvalid, "VERIFICATION_LINK_INVALID", apperr.ClassValidation)
}

func ValidatePasswordStrength(pw string) error { // the one policy shared by customer registration and reset and by admin password set and change
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return ErrWeakPassword
	}
	if len(pw) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}
