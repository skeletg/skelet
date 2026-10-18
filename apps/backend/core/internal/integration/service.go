package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The prefix consumers key on, and the middleware's cheap filter before any lookup - part of the wire contract
const tokenPrefix = "sk_"

const maxNameRunes = 100

// Refused for a reason the caller caused - an unknown, disabled or expired token - as opposed to a lookup that failed
var errTokenRefused = errors.New("integration token refused")

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrNameRequired
	case utf8.RuneCountInString(name) > maxNameRunes:
		return "", ErrNameTooLong
	}
	return name, nil
}

// A token minted already expired is dead on arrival, and nothing in the UI says why
func (s *Service) checkExpiry(at time.Time) error {
	if !at.After(s.now()) {
		return ErrExpiryInPast
	}
	return nil
}

// Generates a new integration - the raw token is returned once in the result and never stored
func (s *Service) Create(ctx context.Context, input CreateIntegrationInput) (CreateIntegrationResult, error) {
	name, err := checkName(input.Name)
	if err != nil {
		return CreateIntegrationResult{}, err
	}
	input.Name = name
	if input.ExpiresAt.Valid {
		if err := s.checkExpiry(input.ExpiresAt.Time); err != nil {
			return CreateIntegrationResult{}, err
		}
	}
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
	i, err := s.repo.GetByHash(ctx, hashToken(rawToken))
	switch {
	case errors.Is(err, ErrNotFound):
		return Integration{}, fmt.Errorf("%w: unknown", errTokenRefused)
	case err != nil:
		return Integration{}, err
	case !i.IsActive:
		return Integration{}, fmt.Errorf("%w: disabled", errTokenRefused)
	case i.ExpiresAt.Valid && s.now().After(i.ExpiresAt.Time):
		return Integration{}, fmt.Errorf("%w: expired", errTokenRefused)
	}
	return i, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Integration, error) {
	return s.repo.GetByID(ctx, id)
}

// Edits name, description, permissions or expiry - nil pointer and slice inputs mean no change
func (s *Service) Update(ctx context.Context, input UpdateIntegrationInput) (Integration, error) {
	if input.Name != nil {
		name, err := checkName(*input.Name)
		if err != nil {
			return Integration{}, err
		}
		input.Name = &name
	}
	if input.ExpiresAt != nil && !input.ClearExpiresAt {
		if err := s.checkExpiry(*input.ExpiresAt); err != nil {
			return Integration{}, err
		}
	}
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
	raw = tokenPrefix + hex.EncodeToString(b)
	hash = hashToken(raw)
	return raw, hash, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
