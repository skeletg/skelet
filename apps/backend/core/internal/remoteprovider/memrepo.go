package remoteprovider

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// MemRepo is an in-memory Repository - It exists so consumer packages (shipping, order, promotion, ...) can exercise their remote-provider
// adapters in tests without a database - Not used in production - the Postgres repository (NewRepository) is the real one
type MemRepo struct {
	mu        sync.Mutex
	providers map[uuid.UUID]Provider
	secrets   map[uuid.UUID]string
}

// NewMemRepo builds an empty in-memory repository
func NewMemRepo() *MemRepo {
	return &MemRepo{providers: map[uuid.UUID]Provider{}, secrets: map[uuid.UUID]string{}}
}

func (r *MemRepo) Create(_ context.Context, input CreateInput, secret string) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := Provider{
		ID: uuid.New(), Kind: input.Kind, Slug: input.Slug, Name: input.Name,
		TargetURL: input.TargetURL, Config: input.Config, TimeoutMs: input.TimeoutMs,
		CacheTTLs: input.CacheTTLs, FailurePolicy: input.FailurePolicy,
		ChannelID: input.ChannelID, IsActive: true,
	}
	r.providers[p.ID] = p
	r.secrets[p.ID] = secret
	return p, nil
}

func (r *MemRepo) GetByID(_ context.Context, id uuid.UUID) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return Provider{}, ErrNotFound
	}
	return p, nil
}

func (r *MemRepo) GetBySlug(_ context.Context, kind Kind, slug string, channelID *uuid.UUID) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var global *Provider
	for _, p := range r.providers {
		if p.Kind != kind || p.Slug != slug {
			continue
		}
		if p.ChannelID == nil {
			global = &p
		} else if channelID != nil && *p.ChannelID == *channelID {
			return p, nil
		}
	}
	if global != nil {
		return *global, nil
	}
	return Provider{}, ErrNotFound
}

func (r *MemRepo) List(_ context.Context) ([]Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	return out, nil
}

func (r *MemRepo) ListActiveByKind(_ context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Provider
	for _, p := range r.providers {
		if p.Kind != kind || !p.IsActive {
			continue
		}
		// global providers always match; scoped ones only their channel
		if p.ChannelID != nil {
			if channelID == nil || *p.ChannelID != *channelID {
				continue
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *MemRepo) Update(_ context.Context, input UpdateInput) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[input.ID]
	if !ok {
		return Provider{}, ErrNotFound
	}
	if input.Name != nil {
		p.Name = *input.Name
	}
	if input.TargetURL != nil {
		p.TargetURL = *input.TargetURL
	}
	if input.Config != nil {
		p.Config = input.Config
	}
	if input.TimeoutMs != nil {
		p.TimeoutMs = *input.TimeoutMs
	}
	if input.CacheTTLs != nil {
		p.CacheTTLs = *input.CacheTTLs
	}
	if input.FailurePolicy != nil {
		p.FailurePolicy = *input.FailurePolicy
	}
	r.providers[p.ID] = p
	return p, nil
}

func (r *MemRepo) SetActive(_ context.Context, id uuid.UUID, active bool) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return Provider{}, ErrNotFound
	}
	p.IsActive = active
	r.providers[id] = p
	return p, nil
}

func (r *MemRepo) UpdateSecret(_ context.Context, id uuid.UUID, secret string) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return Provider{}, ErrNotFound
	}
	r.secrets[id] = secret
	return p, nil
}

func (r *MemRepo) GetSecret(_ context.Context, id uuid.UUID) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.secrets[id]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (r *MemRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
	delete(r.secrets, id)
	return nil
}
