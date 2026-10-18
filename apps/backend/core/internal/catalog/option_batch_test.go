package catalog_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type optionBatchRepo struct {
	fakeRepo
	catalog.OptionRepository
	options      map[uuid.UUID][]catalog.ProductOption
	content      map[uuid.UUID][]catalog.ProductOptionContent
	values       map[uuid.UUID][]catalog.ProductOptionValue
	valueContent map[uuid.UUID][]catalog.ProductOptionValueContent
	optionCalls  int
	contentCalls int
	valueCalls   int
}

func (f *optionBatchRepo) ListProductOptionsByProductIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]catalog.ProductOption, error) {
	f.optionCalls++
	out := map[uuid.UUID][]catalog.ProductOption{}
	for _, id := range ids {
		if opts, ok := f.options[id]; ok {
			out[id] = append([]catalog.ProductOption(nil), opts...)
		}
	}
	return out, nil
}

func (f *optionBatchRepo) ListProductOptionContentByOptionIDs(_ context.Context, _ []uuid.UUID) (map[uuid.UUID][]catalog.ProductOptionContent, error) {
	f.contentCalls++
	return f.content, nil
}

func (f *optionBatchRepo) ListProductOptionValuesByOptionIDs(_ context.Context, _ []uuid.UUID) (map[uuid.UUID][]catalog.ProductOptionValue, error) {
	f.valueCalls++
	out := map[uuid.UUID][]catalog.ProductOptionValue{}
	for k, v := range f.values {
		out[k] = append([]catalog.ProductOptionValue(nil), v...)
	}
	return out, nil
}

func (f *optionBatchRepo) ListProductOptionValueContentByValueIDs(_ context.Context, _ []uuid.UUID) (map[uuid.UUID][]catalog.ProductOptionValueContent, error) {
	return f.valueContent, nil
}

func (f *optionBatchRepo) ListProductOptionChannelPricesByOptionIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]catalog.OptionChannelPrice, error) {
	return nil, nil
}

func (f *optionBatchRepo) ListProductOptionValueChannelPricesByValueIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]catalog.OptionChannelPrice, error) {
	return nil, nil
}

func (f *optionBatchRepo) ListProductRowsByIDs(context.Context, []uuid.UUID) (map[uuid.UUID]catalog.Product, error) {
	return nil, nil
}

func TestGetProductOptionsByProductIDs_GroupsAndHydratesInOneQuerySet(t *testing.T) {
	parent, variant, bare := uuid.New(), uuid.New(), uuid.New()
	engraving, colour, gift := uuid.New(), uuid.New(), uuid.New()
	red := uuid.New()
	repo := &optionBatchRepo{
		options: map[uuid.UUID][]catalog.ProductOption{
			parent:  {{ID: engraving, ProductID: parent, SortOrder: 1}, {ID: colour, ProductID: parent, SortOrder: 2}},
			variant: {{ID: gift, ProductID: variant}},
		},
		content: map[uuid.UUID][]catalog.ProductOptionContent{
			engraving: {{Locale: "en", Title: "Engraving"}, {Locale: "de", Title: "Gravur"}},
			colour:    {{Locale: "en", Title: "Colour"}},
			gift:      {{Locale: "en", Title: "Gift wrap"}},
		},
		values:       map[uuid.UUID][]catalog.ProductOptionValue{colour: {{ID: red, OptionID: colour}}},
		valueContent: map[uuid.UUID][]catalog.ProductOptionValueContent{red: {{Locale: "en", Title: "Red"}}},
	}
	svc := svcWith(repo, func(d *catalog.Deps) { d.DefaultLocale = fakeDefaultLocale{code: "en"} })

	got, err := svc.GetProductOptionsByProductIDs(context.Background(), []uuid.UUID{parent, variant, bare, parent}, "de")
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if repo.optionCalls != 1 || repo.contentCalls != 1 || repo.valueCalls != 1 {
		t.Fatalf("want one query per table, got options=%d content=%d values=%d", repo.optionCalls, repo.contentCalls, repo.valueCalls)
	}
	if _, ok := got[bare]; ok {
		t.Fatalf("a product without options must be absent, got %v", got[bare])
	}
	if n := len(got[parent]); n != 2 {
		t.Fatalf("duplicate keys must not double the parent's options, got %d", n)
	}
	if got[parent][0].Title != "Gravur" || got[parent][1].Title != "Colour" {
		t.Fatalf("titles resolve to the request locale with default fallback, got %q / %q", got[parent][0].Title, got[parent][1].Title)
	}
	if vals := got[parent][1].Values; len(vals) != 1 || vals[0].Title != "Red" {
		t.Fatalf("values hydrate onto their option, got %+v", vals)
	}
	if len(got[variant]) != 1 || got[variant][0].Title != "Gift wrap" {
		t.Fatalf("variant options stay on the variant, got %+v", got[variant])
	}

	single, err := svc.GetProductOptions(context.Background(), bare, "de")
	if err != nil || single == nil || len(single) != 0 {
		t.Fatalf("single-product path keeps returning an empty non-nil slice, got %v, %v", single, err)
	}
}
