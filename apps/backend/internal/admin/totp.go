package admin

// Secrets are encrypted at rest through internal/secrets (the same cipher config TypeSecret values use) and recovery codes are one-use, stored bcrypt-hashed - login enforcement lives in Service.Authenticate, not here

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/secrets"
)

var ErrTotpRequired = errors.New("two-factor authentication code required") // credentials were valid, the account has TOTP on, and no code was supplied

func init() {
	apperr.RegisterSentinel(ErrTotpRequired, "TOTP_REQUIRED", apperr.ClassAuth)
}

const totpIssuer = "Skelet Admin"

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
	if user.TotpPendingSecret == "" {
		return nil, fmt.Errorf("no pending two-factor setup - run setup first")
	}
	plain, err := s.cipher.Decrypt(user.TotpPendingSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt pending totp secret: %w", err)
	}
	if !totp.Validate(strings.TrimSpace(code), plain) {
		return nil, fmt.Errorf("invalid authentication code")
	}
	codes, hashes, err := generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(hashes)
	if err := s.repo.EnableTotp(ctx, adminID, raw); err != nil {
		return nil, err
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

func (s *Service) verifySecondFactor(ctx context.Context, user *AdminUser, code string) (bool, error) { // falls back to the one-use recovery codes and redeems on match
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}
	if s.cipher != nil && user.TotpSecret != "" {
		if plain, err := s.cipher.Decrypt(user.TotpSecret); err == nil && totp.Validate(code, plain) {
			return true, nil
		}
	}
	for i, h := range user.TotpRecoveryCodes { // recovery path - constant-shape scan, then redeem the matched hash
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(strings.ToUpper(code))) == nil {
			remaining := append(append([]string{}, user.TotpRecoveryCodes[:i]...), user.TotpRecoveryCodes[i+1:]...)
			raw, _ := json.Marshal(remaining)
			if err := s.repo.SetTotpRecoveryCodes(ctx, user.ID, raw); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func generateRecoveryCodes(n int) (codes, hashes []string, err error) { // XXXX-XXXX from a crockford-ish alphabet with the ambiguous characters dropped
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	codes = make([]string, 0, n)
	hashes = make([]string, 0, n)
	buf := make([]byte, 8)
	for range n {
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		b := make([]byte, 9)
		for i, v := range buf {
			b[i+i/4] = alphabet[int(v)%len(alphabet)]
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
