package plugin

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A plugin filter an admin picked on a list: its "<plugin>:<key>" and the values ticked
type ListFilterPick struct {
	Key    string
	Values []string
}

// ListFilters is the filters the list offers - those of plugins switched on for the installation
func ListFilters(ctx context.Context, list sdk.ListKind) []sdk.RegisteredListFilter {
	var out []sdk.RegisteredListFilter
	for _, r := range sdk.ListFilters() {
		if r.Filter.List == list && Enabled(ctx, r.PluginSlug, uuid.Nil) {
			out = append(out, r)
		}
	}
	return out
}

// ListFilterIDs is the records of the list every picked filter matched - restrict is false when nothing was picked, so
// the list stays whole. A key the list doesn't offer is refused rather than ignored, since a list that quietly drops a
// filter shows records the admin asked to leave out
func ListFilterIDs(ctx context.Context, list sdk.ListKind, picks []ListFilterPick) (ids []uuid.UUID, restrict bool, err error) {
	offered := ListFilters(ctx, list)
	for _, p := range picks {
		if len(p.Values) == 0 {
			continue
		}
		i := slices.IndexFunc(offered, func(r sdk.RegisteredListFilter) bool { return r.Key() == p.Key })
		if i < 0 {
			return nil, false, apperr.New("LIST_FILTER_UNKNOWN", apperr.ClassValidation, fmt.Sprintf("the %s list has no filter %q", list, p.Key))
		}
		matched, err := matchListFilter(ctx, offered[i], p.Values)
		if err != nil {
			return nil, false, err
		}
		if !restrict {
			ids, restrict = matched, true
		} else {
			ids = slices.DeleteFunc(ids, func(id uuid.UUID) bool {
				_, found := slices.BinarySearchFunc(matched, id, compareIDs)
				return !found
			})
		}
	}
	if restrict && ids == nil {
		ids = []uuid.UUID{}
	}
	return ids, restrict, nil
}

// Unmetered because paging a filtered list asks again on every page - an admin flicking through results must not use
// up the plugin's allowance for its own work
func matchListFilter(ctx context.Context, r sdk.RegisteredListFilter, values []string) (ids []uuid.UUID, err error) {
	ctx, finish, gate := Instrument(ctx, r.PluginSlug, "list filter "+r.Filter.Key, uuid.Nil, Unmetered())
	defer finish(&err)
	if gate != nil {
		return nil, gate
	}
	ids, err = r.Filter.Match(ctx, slices.Clone(values))
	if err != nil {
		return nil, translateUserError(err)
	}
	ids = slices.Clone(ids) // sorted here, so a slice the plugin keeps isn't reordered under it
	slices.SortFunc(ids, compareIDs)
	return slices.Compact(ids), nil
}

func compareIDs(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }
