package shoptools

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
)

func TestSimilarProductsKeepToTheStoresMatchingAttributes(t *testing.T) {
	dress, skirt, far, hidden := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	gql := &fakeGraphQL{answers: map[string]func(map[string]any) (any, error){
		"AgentProduct": func(vars map[string]any) (any, error) {
			return map[string]any{"product": productJSON("SIMPLE", map[string]any{"id": vars["id"], "name": "Linen Shirt Dress"})}, nil
		},
		"AgentProductBySku": func(vars map[string]any) (any, error) {
			if vars["skus"].([]string)[0] != "WRAP-1" {
				return map[string]any{"productsBySkus": []any{}}, nil
			}
			return map[string]any{"productsBySkus": []any{productJSON("SIMPLE", map[string]any{"id": uuid.NewString(), "sku": "WRAP-1", "name": "Wrap Dress"})}}, nil
		},
		"AgentSearch": func(map[string]any) (any, error) {
			return map[string]any{"browseProducts": map[string]any{"totalCount": 2, "items": []any{hit(uuid.NewString(), "Linen Shirt Dress Kids"), hit(dress.String(), "Linen Shirt Dress")}}}, nil
		},
		"AgentHits": func(vars map[string]any) (any, error) {
			var out []any
			for _, id := range vars["ids"].([]string) {
				if id != hidden.String() {
					out = append(out, hit(id, map[string]string{skirt.String(): "Linen Wrap Skirt", far.String(): "Wool Coat"}[id]))
				}
			}
			return map[string]any{"productsByIds": out}, nil
		},
	}}
	m := &fakeMeaning{similar: embedding.Similar{
		Hits:    []embedding.Hit{{ProductID: hidden, Distance: 0.20}, {ProductID: skirt, Distance: 0.21}, {ProductID: far, Distance: 0.6}},
		Matched: []string{"Brand", "Gender"},
	}}
	ts := meaningRig(gql, m)

	out, isErr := run(t, ts, "find_similar_products", map[string]any{"ids": []string{dress.String(), "WRAP-1"}})
	if isErr || !strings.HasPrefix(out, "Products like Linen Shirt Dress and Wrap Dress, each with the same brand and gender, closest first:\n- Linen Wrap Skirt") {
		t.Fatalf("answer = %q", out)
	}
	if !strings.Contains(out, "- Wool Coat") || strings.Index(out, "Wool Coat") < strings.Index(out, "Linen Wrap Skirt") || len(m.seeds) != 2 || m.seeds[0] != dress {
		t.Errorf("answer = %q, seeds %v - want every one found, closest first, and both products asked about", out, m.seeds)
	}

	m.similar = embedding.Similar{Matched: []string{"Gender"}}
	if out, _ := run(t, ts, "find_similar_products", map[string]any{"ids": []string{dress.String()}}); out != "Nothing else in the store is close enough to be like Linen Shirt Dress, each with the same gender." {
		t.Errorf("nothing found = %q", out)
	}
	if _, _ = run(t, ts, "find_similar_products", map[string]any{"ids": []string{"linen shirt dress"}}); len(m.seeds) != 1 || m.seeds[0] != dress {
		t.Errorf("named = %v - want the product going by exactly that name, not the search's first", m.seeds)
	}
	if out, isErr := run(t, ts, "find_similar_products", map[string]any{"ids": []string{"Silk Dress"}}); !isErr || !strings.Contains(out, "No product goes by that name") {
		t.Errorf("an unknown name = %q", out)
	}
	m.similar = embedding.Similar{Unembedded: true}
	if out, _ := run(t, ts, "find_similar_products", map[string]any{"ids": []string{dress.String()}}); !strings.Contains(out, "too new to the store's search") {
		t.Errorf("a product not embedded yet = %q", out)
	}
	if _, isErr := run(t, ts, "find_similar_products", map[string]any{"ids": []string{}}); !isErr {
		t.Error("no products to start from wasn't refused")
	}
}
