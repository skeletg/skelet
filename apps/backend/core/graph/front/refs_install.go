package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/refs"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ExposeRefs hands graph/front/refs this graph's own by-id queries, with their visibility rules, for plugins' types
// that point back at core's records
func ExposeRefs(r *Resolver) {
	q := &queryResolver{r}
	refs.Install(refs.Loaders{
		Products:    q.ProductsByIds,
		Categories:  eachRef(q.Category),
		Collections: eachRef(q.Collection),
	})
}

// One at a time, a record not visible on the channel answering nil rather than failing the rest
func eachRef[T any](one func(ctx context.Context, id string) (*T, error)) func(context.Context, []string) ([]*T, error) {
	return func(ctx context.Context, ids []string) ([]*T, error) {
		out := make([]*T, 0, len(ids))
		for _, id := range ids {
			v, err := one(ctx, id)
			if err != nil {
				if apperr.ClassOf(err) == apperr.ClassInternal {
					return nil, err
				}
				continue
			}
			if v != nil {
				out = append(out, v)
			}
		}
		return out, nil
	}
}
