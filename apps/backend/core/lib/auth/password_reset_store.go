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

const passwordResetTTL = 30 * time.Minute // shorter than verification - a leaked reset link hands over the account

type PasswordResetStore struct { // same shape as VerificationStore, namespaced by subject so the two can't collide or be cross-redeemed
	backend TokenBackend
	subject string
}

func NewPasswordResetStore(backend TokenBackend) *PasswordResetStore {
	return NewPasswordResetStoreFor(backend, "customer")
}

func NewPasswordResetStoreFor(backend TokenBackend, subject string) *PasswordResetStore {
	return &PasswordResetStore{backend: backend, subject: subject}
}

func (s *PasswordResetStore) Create(ctx context.Context, subjectID uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := s.backend.Put(ctx, s.key(token), Token{Subject: subjectID}, passwordResetTTL); err != nil {
		return "", fmt.Errorf("store reset token: %w", err)
	}
	return token, nil
}

func (s *PasswordResetStore) Validate(ctx context.Context, token string) (uuid.UUID, error) { // doesn't delete the token - the caller Revokes after the reset
	t, err := s.backend.Get(ctx, s.key(token))
	if errors.Is(err, ErrTokenNotFound) {
		return uuid.Nil, ErrResetLinkInvalid
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup reset token: %w", err)
	}
	return t.Subject, nil
}

func (s *PasswordResetStore) Consume(ctx context.Context, token string) (uuid.UUID, error) { // Validate and Revoke as one step - the token is spent even if what follows fails
	t, err := s.backend.Consume(ctx, s.key(token))
	if errors.Is(err, ErrTokenNotFound) {
		return uuid.Nil, ErrResetLinkInvalid
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consume reset token: %w", err)
	}
	return t.Subject, nil
}

func (s *PasswordResetStore) Revoke(ctx context.Context, token string) error {
	if err := s.backend.Delete(ctx, s.key(token)); err != nil {
		return fmt.Errorf("revoke reset token: %w", err)
	}
	return nil
}

// Every outstanding link the subject holds - a changed password or address makes them stale
func (s *PasswordResetStore) RevokeAll(ctx context.Context, subjectID uuid.UUID) error {
	if err := s.backend.DeleteSubject(ctx, s.namespace(), subjectID); err != nil {
		return fmt.Errorf("revoke reset tokens: %w", err)
	}
	return nil
}

func (s *PasswordResetStore) TTL() time.Duration { return passwordResetTTL }

func (s *PasswordResetStore) namespace() string { return "reset:" + s.subject }

func (s *PasswordResetStore) key(token string) string {
	return s.namespace() + ":" + hashToken(token)
}
