package remoteprovider

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

// Scoped to one request because activation is a kill switch
type requestCache struct {
	mu        sync.Mutex
	byChannel map[string]map[Kind][]Provider
	flight    singleflight.Group
}

type requestCacheKey struct{}

func WithRequestCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestCacheKey{}, &requestCache{byChannel: map[string]map[Kind][]Provider{}})
}

func RequestCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithRequestCache(r.Context())))
	})
}

func requestCacheFrom(ctx context.Context) *requestCache {
	c, _ := ctx.Value(requestCacheKey{}).(*requestCache)
	return c
}

func invalidateRequestCache(ctx context.Context) {
	if c := requestCacheFrom(ctx); c != nil {
		c.mu.Lock()
		c.byChannel = map[string]map[Kind][]Provider{}
		c.mu.Unlock()
	}
}

func (s *Service) listActive(ctx context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error) {
	c := requestCacheFrom(ctx)
	if c == nil {
		return s.repo.ListActiveByKind(ctx, kind, channelID)
	}
	key := ""
	if channelID != nil {
		key = channelID.String()
	}
	c.mu.Lock()
	byKind, ok := c.byChannel[key]
	c.mu.Unlock()
	if !ok {
		v, err, _ := c.flight.Do(key, func() (any, error) {
			all, err := s.repo.ListActiveForChannel(ctx, channelID)
			if err != nil {
				return nil, err
			}
			grouped := map[Kind][]Provider{}
			for _, p := range all {
				grouped[p.Kind] = append(grouped[p.Kind], p)
			}
			c.mu.Lock()
			c.byChannel[key] = grouped
			c.mu.Unlock()
			return grouped, nil
		})
		if err != nil {
			return nil, err
		}
		byKind = v.(map[Kind][]Provider)
	}
	return append([]Provider(nil), byKind[kind]...), nil
}
