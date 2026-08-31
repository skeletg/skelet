package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
)

func (r *mutationResolver) PurgeCache(ctx context.Context, surfaceKey *string, tags []string, paths []string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.manage"); err != nil {
		return false, err
	}
	if r.CacheClient == nil {
		return false, errNotConfigured("cache revalidation")
	}
	var actor *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		actor = &id
	}
	switch {
	case surfaceKey != nil && *surfaceKey != "":
		if s, ok := revalidate.Lookup(*surfaceKey); !ok || s.AllTag == "" {
			return false, fmt.Errorf("surface %q is per-item only and can't be invalidated catalog-wide", *surfaceKey)
		}
		r.CacheClient.PurgeSurface(ctx, *surfaceKey, "manual invalidate (surface)", actor)
	case len(tags) > 0 || len(paths) > 0:
		r.CacheClient.Purge(ctx, "", tags, paths, "manual invalidate", actor)
	default:
		r.CacheClient.PurgeAll(ctx, "manual invalidate (everything)", actor)
	}
	return true, nil
}

func (r *mutationResolver) RebuildCache(ctx context.Context, paths []string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.manage"); err != nil {
		return false, err
	}
	if r.CacheClient == nil {
		return false, errNotConfigured("cache revalidation")
	}
	if len(paths) == 0 {
		return false, fmt.Errorf("at least one path is required")
	}
	// Synchronous rebuild blocks while the storefront re-renders, so it's capped
	// (the storefront also warms at most 25/request). Larger sets must be queued.
	if len(paths) > 25 {
		return false, fmt.Errorf("rebuildCache is for up to 25 paths; use enqueueRebuild for %d", len(paths))
	}
	var actor *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		actor = &id
	}
	// Invalidate the paths and warm (re-render) them now.
	r.CacheClient.Rebuild(ctx, "", nil, paths, paths, "manual rebuild", actor)
	return true, nil
}

func (r *mutationResolver) EnqueueRebuild(ctx context.Context, paths []string) (int32, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.manage"); err != nil {
		return 0, err
	}
	if r.CacheQueue == nil {
		return 0, errNotConfigured("rebuild queue")
	}
	norm := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		norm = append(norm, p)
	}
	var actor *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		actor = &id
	}
	n, err := r.CacheQueue.EnqueuePaths(ctx, "", norm, nil, "manual", actor)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *queryResolver) CacheSurfaces(ctx context.Context) ([]*model.CacheSurface, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.view"); err != nil {
		return nil, err
	}
	surfaces := revalidate.Surfaces()
	out := make([]*model.CacheSurface, 0, len(surfaces))
	for _, s := range surfaces {
		enabledPath, ttlPath := revalidate.SurfaceConfigPaths(s.Key)
		enabled, _ := r.ConfigSvc.GetBool(ctx, enabledPath, config.DefaultScope)
		ttl, _ := r.ConfigSvc.GetInt(ctx, ttlPath, config.DefaultScope)
		out = append(out, &model.CacheSurface{
			Key:               s.Key,
			Label:             s.Label,
			Enabled:           enabled,
			TTLSeconds:        int32(ttl),
			BulkInvalidatable: s.AllTag != "",
		})
	}
	return out, nil
}

func (r *queryResolver) CacheRevalidations(ctx context.Context, limit *int32) ([]*model.CacheRevalidation, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.view"); err != nil {
		return nil, err
	}
	if r.CacheClient == nil {
		return nil, nil
	}
	n := 50
	if limit != nil && *limit > 0 {
		n = int(*limit)
	}
	rows, err := r.CacheClient.Log(ctx, n)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CacheRevalidation, len(rows))
	for i, row := range rows {
		out[i] = toCacheRevalidationModel(row)
	}
	return out, nil
}

func (r *queryResolver) RebuildQueue(ctx context.Context, recentLimit *int32) (*model.RebuildQueueStatus, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cache.view"); err != nil {
		return nil, err
	}
	if r.CacheQueue == nil {
		return &model.RebuildQueueStatus{Recent: []*model.RebuildTask{}}, nil
	}
	counts, err := r.CacheQueue.Counts(ctx)
	if err != nil {
		return nil, err
	}
	limit := 25
	if recentLimit != nil && *recentLimit > 0 {
		limit = int(*recentLimit)
	}
	recent, err := r.CacheQueue.Recent(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := &model.RebuildQueueStatus{
		Pending: int32(counts["pending"]),
		Running: int32(counts["running"]),
		Done:    int32(counts["done"]),
		Failed:  int32(counts["failed"]),
		Recent:  make([]*model.RebuildTask, len(recent)),
	}
	for i, t := range recent {
		out.Recent[i] = toRebuildTaskModel(t)
	}
	return out, nil
}
