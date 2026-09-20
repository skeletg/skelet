package flow

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Contributes personalization merge tags for one customer - registered, so a suite adds tags off data flow never sees
type MergeVarProvider interface {
	MergeVars(ctx context.Context, channelID, customerID uuid.UUID) map[string]string
}

type mergeVarProviders struct {
	mu   sync.RWMutex
	list []MergeVarProvider
}

func (p *mergeVarProviders) add(v MergeVarProvider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, v)
}

// Contributed tags never overwrite the ones the engine sets, so a provider can't redefine customer.email
func (p *mergeVarProviders) merge(ctx context.Context, channelID, customerID uuid.UUID, into map[string]string) {
	if p == nil {
		return
	}
	p.mu.RLock()
	providers := make([]MergeVarProvider, len(p.list))
	copy(providers, p.list)
	p.mu.RUnlock()
	for _, v := range providers {
		for k, val := range v.MergeVars(ctx, channelID, customerID) {
			if _, taken := into[k]; !taken {
				into[k] = val
			}
		}
	}
}

// RegisterMergeVars adds a source of personalization tags - core sets the store and customer basics, and a suite adds what it computes
func (s *Service) RegisterMergeVars(v MergeVarProvider) {
	if s.varSources == nil {
		s.varSources = &mergeVarProviders{}
	}
	s.varSources.add(v)
}
