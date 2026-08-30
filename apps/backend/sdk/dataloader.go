package sdk

// Batched storefront lookups plugins can register - the host builds a fresh loader per HTTP request from the batch fn, so an in-request cache never leaks across requests
// It lives here rather than in graph/front/dataloaders because plugins import sdk by contract, and registration has to happen in init() before any request bundle exists - so it queues through withHost like every other registration

import (
	"time"

	"github.com/graph-gophers/dataloader/v7"
)

// Shared with the host so plugin-supplied loaders batch on the same window as core ones
const DataLoaderWait = 250 * time.Microsecond

// Lets the host hold registrations of differing K/V in one slice - plugins get this from RegisterDataLoader rather than implementing it
// Build returns *dataloader.Loader[K, V] as any, called once per request
type DataLoaderRegistration interface {
	Slug() string
	Build() any
}

type loaderReg[K comparable, V any] struct {
	slug string
	fn   dataloader.BatchFunc[K, V]
}

func (r *loaderReg[K, V]) Slug() string { return r.slug }

// Fresh cache per call so a request's keys never resolve to a prior request's values
func (r *loaderReg[K, V]) Build() any {
	return dataloader.NewBatchedLoader(
		r.fn,
		dataloader.WithWait[K, V](DataLoaderWait),
		dataloader.WithCache[K, V](dataloader.NewCache[K, V]()),
	)
}

// Resolvers retrieve with dataloaders.Plugin[K, V](dataloaders.For(ctx), slug)
// slug doubles as the cache namespace so keep it stable - convention is "<plugin-slug>.<entity>", duplicates panic at boot
// fn returns one *dataloader.Result[V] per key in key order; a missing key resolves to a zero V or a typed not-found depending on the entity - see the core batch fns in graph/front/dataloaders
func RegisterDataLoader[K comparable, V any](slug string, fn dataloader.BatchFunc[K, V]) {
	reg := &loaderReg[K, V]{slug: slug, fn: fn}
	withHost(func(h Host) error { return h.RegisterDataLoader(reg) })
}
