package remoteprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
)

const (
	// MaxTimeoutMs matches the webhook cap - a provider blocks a
	// user-facing request, 30s is already generous
	MaxTimeoutMs = 30000
	// MaxCacheTTLs bounds response caching at an hour
	MaxCacheTTLs = 3600
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)

type Service struct {
	repo Repository

	// AllowPrivateTargets disables the SSRF range checks on target URLs
	// (same knob as the webhook service - dev only)
	AllowPrivateTargets bool
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (CreateResult, error) {
	if !input.Kind.Valid() {
		return CreateResult{}, fmt.Errorf("%w: %q", ErrInvalidKind, input.Kind)
	}
	if !slugPattern.MatchString(input.Slug) {
		return CreateResult{}, fmt.Errorf("%w: %q (lowercase letters, digits, - and _)", ErrInvalidSlug, input.Slug)
	}
	if input.FailurePolicy == "" {
		input.FailurePolicy = FailOpen
	}
	if err := validateSettings(input.FailurePolicy, input.TimeoutMs, input.CacheTTLs, input.Config); err != nil {
		return CreateResult{}, err
	}
	if err := urlguard.ValidateTargetURL(input.TargetURL, s.AllowPrivateTargets); err != nil {
		return CreateResult{}, err
	}
	raw, err := generateSecret()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate secret: %w", err)
	}
	p, err := s.repo.Create(ctx, input, raw)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{RawSecret: raw, Provider: p}, nil
}

func (s *Service) Update(ctx context.Context, input UpdateInput) (Provider, error) {
	if input.TargetURL != nil {
		if err := urlguard.ValidateTargetURL(*input.TargetURL, s.AllowPrivateTargets); err != nil {
			return Provider{}, err
		}
	}
	policy := FailurePolicy("")
	if input.FailurePolicy != nil {
		policy = *input.FailurePolicy
	}
	var timeout, ttl int32
	if input.TimeoutMs != nil {
		timeout = *input.TimeoutMs
	}
	if input.CacheTTLs != nil {
		ttl = *input.CacheTTLs
	}
	if err := validateSettings(policy, timeout, ttl, input.Config); err != nil {
		return Provider{}, err
	}
	p, err := s.repo.Update(ctx, input)
	if err != nil {
		return Provider{}, err
	}
	// an edit usually means the endpoint got fixed - let it prove it now
	breaker.Reset(p.ID)
	invocationCache.invalidate(p.ID)
	return p, nil
}

func (s *Service) List(ctx context.Context) ([]Provider, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Provider, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetBySlug(ctx context.Context, kind Kind, slug string) (Provider, error) {
	return s.repo.GetBySlug(ctx, kind, slug)
}

// ListActiveByKind returns the providers a consumer should invoke for one
// decision: global providers plus the ones scoped to the given channel
func (s *Service) ListActiveByKind(ctx context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error) {
	return s.repo.ListActiveByKind(ctx, kind, channelID)
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) (Provider, error) {
	p, err := s.repo.SetActive(ctx, id, active)
	if err != nil {
		return Provider{}, err
	}
	if active {
		breaker.Reset(id)
	}
	return p, nil
}

func (s *Service) RotateSecret(ctx context.Context, id uuid.UUID) (CreateResult, error) {
	raw, err := generateSecret()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate secret: %w", err)
	}
	p, err := s.repo.UpdateSecret(ctx, id, raw)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{RawSecret: raw, Provider: p}, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	invocationCache.invalidate(id)
	return s.repo.Delete(ctx, id)
}

func validateSettings(policy FailurePolicy, timeoutMs, cacheTTLs int32, config []byte) error {
	if policy != "" && policy != FailOpen && policy != FailClosed {
		return fmt.Errorf("%w: %q", ErrInvalidPolicy, policy)
	}
	if timeoutMs < 0 || timeoutMs > MaxTimeoutMs {
		return fmt.Errorf("%w: %d ms (0 = default, max %d)", ErrInvalidTimeout, timeoutMs, MaxTimeoutMs)
	}
	if cacheTTLs < 0 || cacheTTLs > MaxCacheTTLs {
		return fmt.Errorf("%w: %d s (0 = no cache, max %d)", ErrInvalidCacheTTL, cacheTTLs, MaxCacheTTLs)
	}
	if len(config) > 0 && !json.Valid(config) {
		return fmt.Errorf("config must be valid JSON")
	}
	return nil
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "rpsec_" + hex.EncodeToString(b), nil
}
