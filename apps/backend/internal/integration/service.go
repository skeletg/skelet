package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Generates a new integration - the raw token is returned once in the result and never stored
func (s *Service) Create(ctx context.Context, input CreateIntegrationInput) (CreateIntegrationResult, error) {
	raw, hash, err := generateToken()
	if err != nil {
		return CreateIntegrationResult{}, fmt.Errorf("generate token: %w", err)
	}
	i, err := s.repo.Create(ctx, input, hash)
	if err != nil {
		return CreateIntegrationResult{}, err
	}
	return CreateIntegrationResult{RawToken: raw, Integration: i}, nil
}

// Looks up an integration by its raw token and checks it's active and not expired
func (s *Service) Validate(ctx context.Context, rawToken string) (Integration, error) {
	hash := hashToken(rawToken)
	i, err := s.repo.GetByHash(ctx, hash)
	if err != nil {
		return Integration{}, fmt.Errorf("invalid token")
	}
	if !i.IsActive {
		return Integration{}, fmt.Errorf("integration is disabled")
	}
	if i.ExpiresAt.Valid && time.Now().After(i.ExpiresAt.Time) {
		return Integration{}, fmt.Errorf("integration token is expired")
	}
	return i, nil
}

// Edits name, description, permissions or expiry - nil pointer and slice inputs mean no change
func (s *Service) Update(ctx context.Context, input UpdateIntegrationInput) (Integration, error) {
	return s.repo.Update(ctx, input)
}

// Enables or disables an integration - disabling makes Validate refuse the token immediately, with no TTL on the decision
func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) (Integration, error) {
	return s.repo.SetActive(ctx, id, active)
}

// Rotates the raw token, returning the new one exactly once - the old token stops working immediately
func (s *Service) RegenerateToken(ctx context.Context, id uuid.UUID) (CreateIntegrationResult, error) {
	raw, hash, err := generateToken()
	if err != nil {
		return CreateIntegrationResult{}, fmt.Errorf("generate token: %w", err)
	}
	i, err := s.repo.RegenerateTokenHash(ctx, id, hash)
	if err != nil {
		return CreateIntegrationResult{}, err
	}
	return CreateIntegrationResult{RawToken: raw, Integration: i}, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Integration, error) {
	return s.repo.List(ctx)
}

// Produces a raw token and its SHA-256 hash, formatted "sk_" plus hex of 32 random bytes - the prefix is a stable wire contract for consumers
func generateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "sk_" + hex.EncodeToString(b)
	hash = hashToken(raw)
	return raw, hash, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
