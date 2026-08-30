package sdk

import (
	"context"
	"errors"
)

// A plugin extends the admin or storefront graph by shipping SDL alongside its Go code, at
// plugins/<dir>/graphql/{admin,front}/*.graphqls - gqlgen merges it into the core schema at codegen
// the same way every core schema file is merged, so plugin fields are first-class: typed, introspectable
// and picked up by the dashboard's TS codegen with no special casing
//
// Use `extend type Query` / `extend type Mutation` and prefix every field and type with the plugin
// slug - gqlgen rejects duplicate field names, so a clash between two plugins fails at codegen rather
// than at runtime, and the prefix is what keeps the global namespace honest
//
// SDL can only ADD. Redefining an existing field, or adding an argument to one, is a schema error -
// to change core BEHAVIOUR use SubscribeSync, which hands you the live event to mutate
//
// Don't declare new scalars: an unbound scalar silently degrades to Go `string` in gqlgen, taking your
// marshalling with it. Reuse JSON, Int64 and Time, which the host already binds
//
// A field on a list type (Product, Order) resolves once PER ROW - use RegisterDataLoader for anything
// that touches the database, or a listing page pays one query per item
//
// Core side, per field: gqlgen stubs the resolver once and copies your body through on every later run,
// so the delegation into this plugin is written by hand ONE time, with no codegen pass rewriting
// those stubs. Put the interface and slug const in graph/<surface>/plugin_resolvers.go,
// NOT beside the stub: gqlgen relocates non-resolver declarations out of a .resolvers.go into a
// commented-out block on the next run, so they'd survive right up until someone regenerated

// Which graph a resolver belongs to - a plugin can extend either or both, registering one impl per surface
type GraphSurface string

const (
	GraphAdmin GraphSurface = "admin"
	GraphFront GraphSurface = "front"
)

// Registers the object serving this plugin's fields on one surface - impl needs one method per field
// its SDL declares, matching the signature gqlgen generated for that field
//
// Nothing here names a shared type: core asserts impl against an interface it declares privately, and
// Go's structural typing does the rest, so the plugin imports no core resolver contract and core imports
// no plugin package. A signature that drifts from the SDL fails the host's boot-time check rather than
// the first query
//
// Every call through impl is wrapped in the plugin's kill switch, rate limit, hook timeout and panic
// isolation, exactly like a payment hook - a resolver that dies fails its own field, not the request
//
//	p.RegisterGraphResolver(sdk.GraphAdmin, &adminResolver{})
func (pl *Plugin) RegisterGraphResolver(surface GraphSurface, impl any) {
	withHost(func(h Host) error { return h.RegisterGraphResolver(pl.slug, surface, impl) })
}

// Refuses unless the calling ADMIN holds slug - for gating admin graph fields and any other operator
// surface a plugin exposes, using the same check core's own resolvers make
// Declare the permission with RegisterPermission first, or you're gating on one no role can hold
//
// A superuser passes everything; an unauthenticated caller is always refused, so this is safe to call
// as the first line of a resolver
// Admin-scoped by name and by nature: storefront resolvers act for a CUSTOMER, whose identity rides in
// the request context rather than in the admin permission table
func RequirePermission(ctx context.Context, slug string) error {
	h := hostNow()
	if h == nil {
		// no host means no auth wired, which is a test or an early-boot caller - refuse rather than admit
		return ErrPermissionUnavailable
	}
	return h.RequireAdminPermission(ctx, slug)
}

// Returned when the permission checker isn't wired - fail CLOSED, since the alternative is a plugin's
// privileged field answering during the window before auth exists
var ErrPermissionUnavailable = errors.New("sdk: permission checker unavailable")
