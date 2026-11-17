package plugin

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestListFilterIDs(t *testing.T) {
	sdk.ResetForTest()
	t.Cleanup(sdk.ResetForTest)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	answer := map[string][]uuid.UUID{"yes": {a, b, b}, "gold": {b, c}, "none": nil}
	match := func(_ context.Context, values []string) ([]uuid.UUID, error) {
		var out []uuid.UUID
		for _, v := range values {
			if v == "broken" {
				return nil, sdk.UserErrorWithCode(sdk.ErrorUnavailable, "ACME_DOWN", "the warranty service is down")
			}
			out = append(out, answer[v]...)
		}
		return out, nil
	}
	opts := []sdk.ListFilterOption{{Value: "yes", Label: "Yes"}}
	acme := sdk.NewPluginForTest("acme")
	acme.RegisterListFilter(sdk.ListFilter{List: sdk.ListProducts, Key: "warranty", Label: "Warranty", Options: opts, Match: match})
	acme.RegisterListFilter(sdk.ListFilter{List: sdk.ListProducts, Key: "tier", Label: "Tier", Options: opts, Match: match})
	acme.RegisterListFilter(sdk.ListFilter{List: sdk.ListOrders, Key: "synced", Label: "Synced", Options: opts, Match: match})

	if got := ListFilters(context.Background(), sdk.ListProducts); len(got) != 2 {
		t.Fatalf("products offers %d filters, want 2", len(got))
	}
	sorted := func(ids ...uuid.UUID) []uuid.UUID { return slices.SortedFunc(slices.Values(ids), compareIDs) }
	for name, tc := range map[string]struct {
		picks    []ListFilterPick
		ids      []uuid.UUID
		restrict bool
		code     string
	}{
		"nothing picked":     {nil, nil, false, ""},
		"no values ticked":   {[]ListFilterPick{{Key: "acme:warranty"}}, nil, false, ""},
		"one filter":         {[]ListFilterPick{{Key: "acme:warranty", Values: []string{"yes"}}}, sorted(a, b), true, ""},
		"values are any-of":  {[]ListFilterPick{{Key: "acme:warranty", Values: []string{"yes", "gold"}}}, sorted(a, b, c), true, ""},
		"filters intersect":  {[]ListFilterPick{{Key: "acme:warranty", Values: []string{"yes"}}, {Key: "acme:tier", Values: []string{"gold"}}}, []uuid.UUID{b}, true, ""},
		"nothing matched":    {[]ListFilterPick{{Key: "acme:warranty", Values: []string{"none"}}}, []uuid.UUID{}, true, ""},
		"another list's key": {[]ListFilterPick{{Key: "acme:synced", Values: []string{"yes"}}}, nil, false, "LIST_FILTER_UNKNOWN"},
		"the plugin refuses": {[]ListFilterPick{{Key: "acme:warranty", Values: []string{"broken"}}}, nil, false, "ACME_DOWN"},
	} {
		t.Run(name, func(t *testing.T) {
			ids, restrict, err := ListFilterIDs(context.Background(), sdk.ListProducts, tc.picks)
			if !slices.Equal(ids, tc.ids) || restrict != tc.restrict || (ids == nil) != (tc.ids == nil) || apperr.CodeOf(err) != tc.code {
				t.Errorf("got %v restrict %v err %v (code %q) - want %v restrict %v code %q", ids, restrict, err, apperr.CodeOf(err), tc.ids, tc.restrict, tc.code)
			}
		})
	}
}
