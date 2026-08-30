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

type VerificationStore struct { // single-use, but Validate doesn't delete - the caller Revokes after flipping is_email_verified, so verification can be marked transactionally without racing token consumption
	backend TokenBackend
}

func NewVerificationStore(backend TokenBackend) *VerificationStore {
	return &VerificationStore{backend: backend}
}

func (s *VerificationStore) Create(ctx context.Context, customerID uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := s.backend.Put(ctx, s.key(token), customerID, verificationTTL); err != nil {
		return "", fmt.Errorf("store verification token: %w", err)
	}
	return token, nil
}

func (s *VerificationStore) Validate(ctx context.Context, token string) (uuid.UUID, error) { // doesn't delete the token - the caller Revokes after verifying
	id, err := s.backend.Get(ctx, s.key(token))
	if errors.Is(err, ErrTokenNotFound) {
		return uuid.Nil, fmt.Errorf("invalid or expired verification link")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup verification token: %w", err)
	}
	return id, nil
}

func (s *VerificationStore) Revoke(ctx context.Context, token string) error {
	if err := s.backend.Delete(ctx, s.key(token)); err != nil {
		return fmt.Errorf("revoke verification token: %w", err)
	}
	return nil
}

func (s *VerificationStore) key(token string) string {
	return "verify:customer:" + hashToken(token)
}
