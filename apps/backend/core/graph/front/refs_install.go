package front

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminrefs "github.com/skeletg/skelet/apps/backend/core/graph/admin/refs"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/refs"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// A category or a collection is asked for one at a time, so this is only how many go by before the next deadline check
const refsOneByOne = 50

// ExposeRefs hands graph/front/refs this graph's own by-id queries, with their visibility rules, for plugins' types
// that point back at core's records
func ExposeRefs(r *Resolver) {
	q := &queryResolver{r}
	refs.Install(refs.Loaders{
		// The cap productsByIds itself truncates at - past it an id would come back as not visible
		Products:    adminrefs.Loader[model.Product]{Load: q.ProductsByIds, Batch: catalog.ProductsPerLookup},
		Categories:  adminrefs.Loader[model.Category]{Load: adminrefs.Each(q.Category), Batch: refsOneByOne},
		Collections: adminrefs.Loader[model.Collection]{Load: adminrefs.Each(q.Collection), Batch: refsOneByOne},
	})
}
