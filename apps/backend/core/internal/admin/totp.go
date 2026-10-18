package admin

// Secrets are encrypted at rest through internal/secrets (the same cipher config TypeSecret values use) and recovery codes are one-use, stored bcrypt-hashed - login enforcement lives in Service.Authenticate, not here

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

var ErrTotpRequired = errors.New("two-factor authentication code required") // credentials were valid, the account has TOTP on, and no code was supplied

var ErrTotpLocked = errors.New("too many two-factor attempts - try again later")

var ErrTotpAlreadyEnabled = errors.New("two-factor authentication is already on - turn it off first")

func init() {
	apperr.RegisterSentinel(ErrTotpRequired, "TOTP_REQUIRED", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrTotpLocked, "TOTP_LOCKED", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrTotpAlreadyEnabled, "TOTP_ALREADY_ENABLED", apperr.ClassConflict)
}

const (
	totpIssuer = "Skelet Admin"
	totpPeriod = 30

	// Five tries per quarter hour holds a 6-digit code out of reach whether or not the rate limiter is on
	totpMaxAttempts = 5
	totpLockout     = 15 * time.Minute
)

func (s *Service) SetSecretsCipher(c *secrets.Cipher) { s.cipher = c } // without it setup and enable refuse - 2FA can't store a secret without the key

// Generates a fresh secret, stores it encrypted as PENDING and not yet enforced, and returns the plaintext secret plus otpauth:// URL for the authenticator app - re-running replaces the pending secret
func (s *Service) TotpSetup(ctx context.Context, adminID uuid.UUID) (secret, otpauthURL string, err error) {
	if s.cipher == nil {
		return "", "", fmt.Errorf("two-factor setup unavailable: secrets encryption key not configured")
	}
	user, err := s.repo.GetByID(ctx, adminID)
	if err != nil {
		return "", "", err
	}
	if user.TotpEnabled { // re-running setup would swap the live factor for one of the caller's choosing without ever presenting it
		return "", "", ErrTotpAlreadyEnabled
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: user.Email})
	if err != nil {
		return "", "", fmt.Errorf("generate totp secret: %w", err)
	}
	enc, err := s.cipher.Encrypt(key.Secret())
	if err != nil {
		return "", "", fmt.Errorf("encrypt totp secret: %w", err)
	}
	if err := s.repo.SetTotpPending(ctx, adminID, enc); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// Promotes the pending secret once its first code verifies - the ten recovery codes come back in plaintext exactly here and only their bcrypt hashes are stored
func (s *Service) TotpEnable(ctx context.Context, adminID uuid.UUID, code string) ([]string, error) {
	if s.cipher == nil {
		return nil, fmt.Errorf("two-factor setup unavailable: secrets encryption key not configured")
	}
	user, err := s.repo.GetByID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if user.TotpEnabled {
		return nil, ErrTotpAlreadyEnabled
	}
	if user.TotpPendingSecret == "" {
		return nil, errNoPendingTotp
	}
	plain, err := s.cipher.Decrypt(user.TotpPendingSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt pending totp secret: %w", err)
	}
	step, ok := matchTotpStep(plain, strings.TrimSpace(code), s.now())
	if !ok {
		return nil, fmt.Errorf("invalid authentication code")
	}
	codes, hashes, err := generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(hashes)
	enabled, err := s.repo.EnableTotp(ctx, adminID, raw, step) // the enabling code counts as used, so it can't be replayed at login
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errNoPendingTotp
	}
	return codes, nil
}

func (s *Service) TotpDisable(ctx context.Context, adminID uuid.UUID, code string) error { // needs a current or recovery code - the account password alone must not be enough to turn 2FA off
	user, err := s.repo.GetByID(ctx, adminID)
	if err != nil {
		return err
	}
	if !user.TotpEnabled {
		return nil
	}
	ok, err := s.verifySecondFactor(ctx, &user, code)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid authentication code")
	}
	return s.repo.DisableTotp(ctx, adminID)
}

var errNoPendingTotp = errors.New("no pending two-factor setup - run setup first")

// Every non-empty code spends one claimed attempt, and a TOTP code is good for one login only
func (s *Service) verifySecondFactor(ctx context.Context, user *AdminUser, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}
	claimed, err := s.repo.ClaimTotpAttempt(ctx, user.ID, totpMaxAttempts, totpLockout)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, ErrTotpLocked
	}
	if rc, ok := normalizeRecoveryCode(code); ok {
		return s.redeemRecoveryCode(ctx, user, rc)
	}
	if s.cipher == nil || user.TotpSecret == "" {
		return false, nil
	}
	plain, err := s.cipher.Decrypt(user.TotpSecret)
	if err != nil {
		return false, nil
	}
	step, ok := matchTotpStep(plain, code, s.now())
	if !ok {
		return false, nil
	}
	return s.repo.AcceptTotpStep(ctx, user.ID, step)
}

func (s *Service) redeemRecoveryCode(ctx context.Context, user *AdminUser, code string) (bool, error) {
	for i, h := range user.TotpRecoveryCodes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(code)) != nil {
			continue
		}
		expected, _ := json.Marshal(user.TotpRecoveryCodes)
		remaining, _ := json.Marshal(append(append([]string{}, user.TotpRecoveryCodes[:i]...), user.TotpRecoveryCodes[i+1:]...))
		redeemed, err := s.repo.RedeemTotpRecoveryCode(ctx, user.ID, expected, remaining)
		if err != nil || !redeemed { // someone else changed the list since it was read - this code may already be spent
			return false, err
		}
		return true, s.repo.ClearTotpFailures(ctx, user.ID)
	}
	return false, nil
}

// The step a code belongs to within one period either side of now, matching totp.Validate's default window
func matchTotpStep(secret, code string, at time.Time) (int64, bool) {
	now := at.Unix() / totpPeriod
	for _, step := range []int64{now, now - 1, now + 1} {
		want, err := hotp.GenerateCodeCustom(secret, uint64(step), hotp.ValidateOpts{Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// XXXX-XXXX in any case, hyphen optional - anything else skips the ten bcrypt compares a recovery scan costs
func normalizeRecoveryCode(code string) (string, bool) {
	code = strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	if len(code) != 8 {
		return "", false
	}
	for _, c := range code {
		if !strings.ContainsRune(recoveryAlphabet, c) {
			return "", false
		}
	}
	return code[:4] + "-" + code[4:], true
}

const recoveryAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

func generateRecoveryCodes(n int) (codes, hashes []string, err error) { // XXXX-XXXX from a crockford-ish alphabet with the ambiguous characters dropped
	codes = make([]string, 0, n)
	hashes = make([]string, 0, n)
	buf := make([]byte, 8)
	for range n {
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		b := make([]byte, 9)
		for i, v := range buf {
			b[i+i/4] = recoveryAlphabet[int(v)%len(recoveryAlphabet)]
		}
		b[4] = '-'
		code := string(b)
		h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, fmt.Errorf("hash recovery code: %w", err)
		}
		codes = append(codes, code)
		hashes = append(hashes, string(h))
	}
	return codes, hashes, nil
}
