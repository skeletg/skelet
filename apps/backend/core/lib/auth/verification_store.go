package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const verificationTTL = 24 * time.Hour // people don't open welcome emails instantly, but holding the token open forever invites token-leak attacks

const verificationNamespace = "verify:customer"

type VerificationStore struct { // single-use, but Validate doesn't delete - the caller Revokes after flipping is_email_verified, so verification can be marked transactionally without racing token consumption
	backend TokenBackend
}

func NewVerificationStore(backend TokenBackend) *VerificationStore {
	return &VerificationStore{backend: backend}
}

// The token is bound to the address it is mailed to, so it proves that address and no other
func (s *VerificationStore) Create(ctx context.Context, customerID uuid.UUID, email string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := s.backend.Put(ctx, s.key(token), Token{Subject: customerID, Binding: emailBinding(email)}, verificationTTL); err != nil {
		return "", fmt.Errorf("store verification token: %w", err)
	}
	return token, nil
}

type VerificationClaim struct {
	CustomerID uuid.UUID
	binding    string
}

// BoundTo is false for a token minted for any other address, and for one minted before tokens were bound
func (c VerificationClaim) BoundTo(email string) bool {
	return c.binding != "" && c.binding == emailBinding(email)
}

func (s *VerificationStore) Validate(ctx context.Context, token string) (VerificationClaim, error) { // doesn't delete the token - the caller Revokes after verifying
	t, err := s.backend.Get(ctx, s.key(token))
	if errors.Is(err, ErrTokenNotFound) {
		return VerificationClaim{}, ErrVerificationLinkInvalid
	}
	if err != nil {
		return VerificationClaim{}, fmt.Errorf("lookup verification token: %w", err)
	}
	return VerificationClaim{CustomerID: t.Subject, binding: t.Binding}, nil
}

func (s *VerificationStore) Revoke(ctx context.Context, token string) error {
	if err := s.backend.Delete(ctx, s.key(token)); err != nil {
		return fmt.Errorf("revoke verification token: %w", err)
	}
	return nil
}

func (s *VerificationStore) RevokeAll(ctx context.Context, customerID uuid.UUID) error {
	if err := s.backend.DeleteSubject(ctx, verificationNamespace, customerID); err != nil {
		return fmt.Errorf("revoke verification tokens: %w", err)
	}
	return nil
}

func (s *VerificationStore) key(token string) string {
	return verificationNamespace + ":" + hashToken(token)
}

func emailBinding(email string) string {
	return hashToken("email:" + email)
}
