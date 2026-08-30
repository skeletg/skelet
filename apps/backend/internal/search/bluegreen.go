package search

import (
	"context"
	"fmt"
	"log"
	"time"
)

// BlueGreen is the zero-downtime rebuild: build a fresh physical index, then flip the read alias in one atomic step. Indexes implementing
// it never serve empty mid-rebuild - the registry prefers this over Recreate+ReindexAll when present
type BlueGreen interface {
	RebuildAndSwap(ctx context.Context) (int, error)
}

// rebuildAndSwap is the shared flip choreography: alias is the name every reader
// and live writer uses (ProductIndex / BlogPostIndex), build fills an index it is
// handed the PHYSICAL name of, and the swap happens only on a clean build - any
// error leaves the live index serving untouched and deletes the debris.
//
// A name that resolves to a concrete index rather than an alias is converted in
// place: the index is deleted and the alias added under that name. That one
// conversion has a sub-second window where the name resolves to nothing; every
// later rebuild is a pure atomic alias swap
func rebuildAndSwap(
	ctx context.Context,
	client *Client,
	alias string,
	mappingJSON string,
	build func(ctx context.Context, physicalIndex string) (int, error),
) (int, error) {
	fresh := fmt.Sprintf("%s-%d", alias, time.Now().UTC().Unix())
	if err := client.CreateIndex(ctx, fresh, mappingJSON); err != nil {
		return 0, err
	}
	n, err := build(ctx, fresh)
	if err != nil {
		// Strict on purpose: swapping in a partially-built index trades a complete
		// live one for a known-incomplete one. The operator retries; live never moved
		if derr := client.DeleteIndex(ctx, fresh); derr != nil {
			log.Printf("[search] cleanup of failed rebuild %s: %v", fresh, derr)
		}
		return n, fmt.Errorf("rebuild into %s (live index untouched): %w", fresh, err)
	}

	// Any failure between here and the swap orphans the fully-built fresh index -
	// delete it on the way out so a failed flip can't leak physicals
	fail := func(ferr error) (int, error) {
		if derr := client.DeleteIndex(ctx, fresh); derr != nil {
			log.Printf("[search] cleanup of unswapped rebuild %s: %v", fresh, derr)
		}
		return n, ferr
	}
	old, err := client.ResolveAlias(ctx, alias)
	if err != nil {
		return fail(err)
	}
	if len(old) == 0 {
		// Not an alias - either a concrete index under this name, or nothing
		concrete, cerr := client.IndexExists(ctx, alias)
		if cerr != nil {
			return fail(cerr)
		}
		if concrete {
			if derr := client.DeleteIndex(ctx, alias); derr != nil {
				return fail(fmt.Errorf("retire concrete index %s: %w", alias, derr))
			}
		}
	}
	if err := client.SwapAlias(ctx, alias, fresh, old); err != nil {
		// NOT fail(): past that deletion the fresh index may be the only copy,
		// and deleting it on a flaky swap response would drop the catalogue outright
		return n, err
	}
	for _, retired := range old {
		if retired == fresh {
			continue
		}
		if derr := client.DeleteIndex(ctx, retired); derr != nil {
			// The flip already happened - a leftover physical wastes disk, not correctness
			log.Printf("[search] delete retired index %s: %v", retired, derr)
		}
	}
	return n, nil
}
