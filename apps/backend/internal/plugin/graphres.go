package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/sdk"
)

// Replaces a codegen pass that rewrote gqlgen's stub bodies in place - gqlgen already copies hand-written
// resolver bodies through, so delegation is written once and Do is the one chokepoint giving plugin
// GraphQL the gating every other surface has - see sdk/graphql.go for the plugin-author contract

var ErrNoGraphResolver = errors.New("plugin: no graph resolver registered")

type graphKey struct {
	surface sdk.GraphSurface
	slug    string
}

var graphRegistry struct {
	mu    sync.RWMutex
	impls map[graphKey]any
}

func (h *Host) RegisterGraphResolver(slug string, surface sdk.GraphSurface, impl any) error {
	if impl == nil {
		return fmt.Errorf("plugin %s: RegisterGraphResolver(%s) called with nil impl", slug, surface)
	}
	if surface != sdk.GraphAdmin && surface != sdk.GraphFront {
		return fmt.Errorf("plugin %s: unknown graph surface %q", slug, surface)
	}
	key := graphKey{surface: surface, slug: slug}
	graphRegistry.mu.Lock()
	defer graphRegistry.mu.Unlock()
	if graphRegistry.impls == nil {
		graphRegistry.impls = make(map[graphKey]any)
	}
	if _, dup := graphRegistry.impls[key]; dup {
		return fmt.Errorf("plugin %s: duplicate %s graph resolver", slug, surface)
	}
	graphRegistry.impls[key] = impl
	return nil
}

// I is an interface the CALLER declares privately, so the plugin imports no core resolver contract and
// core imports no plugin package - Go matches them structurally
// call takes the ctx back rather than closing over the caller's, or the resolver runs outside Instrument's
// timeout, span and tagged logger while looking like it doesn't
func Do[I any, T any](ctx context.Context, surface sdk.GraphSurface, slug, field string, call func(context.Context, I) (T, error)) (_ T, err error) {
	var zero T
	impl, ok := lookupGraphResolver(surface, slug)
	if !ok {
		return zero, fmt.Errorf("%w: %s.%s (%s)", ErrNoGraphResolver, slug, field, surface)
	}
	typed, ok := impl.(I)
	if !ok {
		return zero, fmt.Errorf("plugin %s: %s resolver does not implement %T (field %s)", slug, surface, *new(I), field)
	}
	ctx, finish, gate := Instrument(ctx, slug, "graph "+string(surface)+"."+field, graphChannelID(ctx))
	defer finish(&err)
	if gate != nil {
		return zero, gate
	}
	out, callErr := call(ctx, typed)
	return out, translateUserError(callErr)
}

// Plugins can't import internal/apperr, so without this an unclassified resolver error reaches the storefront scrubbed to "internal error"
func translateUserError(err error) error {
	var ue *sdk.UserError
	if !errors.As(err, &ue) {
		return err
	}
	return apperr.New(ue.Code, userErrorClass(ue.Kind), ue.Message)
}

func userErrorClass(k sdk.ErrorKind) apperr.Class {
	switch k {
	case sdk.ErrorNotFound:
		return apperr.ClassNotFound
	case sdk.ErrorConflict:
		return apperr.ClassConflict
	case sdk.ErrorForbidden:
		return apperr.ClassForbidden
	case sdk.ErrorUnavailable:
		return apperr.ClassUnavailable
	default:
		return apperr.ClassValidation
	}
}

// Boot-time proof that a plugin still serves the fields its SDL declares, so drift fails the deploy
// rather than the first query to touch that field
func RequireGraphResolver[I any](surface sdk.GraphSurface, slug string) error {
	impl, ok := lookupGraphResolver(surface, slug)
	if !ok {
		return nil // not registered - either the plugin dir is gone, or it's out of plugins/firstparty while its SDL is still globbed into generated.go, and Do() answers ErrNoGraphResolver
	}
	if _, ok := impl.(I); !ok {
		return fmt.Errorf("plugin %s: %s graph resolver %T does not implement %T - its SDL and its Go methods have drifted",
			slug, surface, impl, *new(I))
	}
	return nil
}

func lookupGraphResolver(surface sdk.GraphSurface, slug string) (any, bool) {
	graphRegistry.mu.RLock()
	defer graphRegistry.mu.RUnlock()
	impl, ok := graphRegistry.impls[graphKey{surface: surface, slug: slug}]
	return impl, ok
}

// So a per-channel kill switch bites here the way it bites the payment hooks
func graphChannelID(ctx context.Context) uuid.UUID {
	if ch, ok := channel.FromContext(ctx); ok {
		return ch.ID
	}
	return uuid.Nil
}

func GraphResolverSlugs(surface sdk.GraphSurface) []string {
	graphRegistry.mu.RLock()
	defer graphRegistry.mu.RUnlock()
	out := make([]string, 0, len(graphRegistry.impls))
	for k := range graphRegistry.impls {
		if k.surface == surface {
			out = append(out, k.slug)
		}
	}
	sort.Strings(out)
	return out
}
