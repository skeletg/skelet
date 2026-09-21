package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// --- test doubles -----------------------------------------------------------

// fakeRepo embeds catalog.Repository so any method the test doesn't exercise panics (nil interface deref) - a loud signal that coverage is missing rather than a silent wrong answer - Only the read methods the validation paths touch are overridden - Mirrors the payment/giftcard service-test pattern
type fakeRepo struct {
	catalog.Repository

	productByID      map[uuid.UUID]catalog.Product
	configOptions    []catalog.Attribute
	attrWithValues   map[uuid.UUID]catalog.Attribute
	children         []catalog.Product
	selectableValues map[uuid.UUID]map[uuid.UUID][]uuid.UUID
	getProductErr    error
	// AddProductChild path: ancestor walk (cycle check) + existing-parent lookup
	productParents          map[uuid.UUID][]catalog.Product
	configurableParentLinks map[uuid.UUID]catalog.ConfigurableParentLink
}

func (f *fakeRepo) ListProductParents(_ context.Context, id uuid.UUID) ([]catalog.Product, error) {
	return f.productParents[id], nil
}

func (f *fakeRepo) ListConfigurableParentByChildIDs(_ context.Context, childIDs []uuid.UUID) (map[uuid.UUID]catalog.ConfigurableParentLink, error) {
	out := make(map[uuid.UUID]catalog.ConfigurableParentLink, len(childIDs))
	for _, id := range childIDs {
		if l, ok := f.configurableParentLinks[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

func (f *fakeRepo) GetProductByID(_ context.Context, id uuid.UUID) (catalog.Product, error) {
	if f.getProductErr != nil {
		return catalog.Product{}, f.getProductErr
	}
	return f.productByID[id], nil
}

func (f *fakeRepo) ListConfigurableOptions(_ context.Context, _ uuid.UUID) ([]catalog.Attribute, error) {
	return f.configOptions, nil
}

func (f *fakeRepo) GetAttributeWithValues(_ context.Context, id uuid.UUID) (catalog.Attribute, error) {
	return f.attrWithValues[id], nil
}

func (f *fakeRepo) ListProductChildren(_ context.Context, _ uuid.UUID) ([]catalog.Product, error) {
	return f.children, nil
}

func (f *fakeRepo) ListProductSelectableValuesByIDs(_ context.Context, _, _ []uuid.UUID) (map[uuid.UUID]map[uuid.UUID][]uuid.UUID, error) {
	return f.selectableValues, nil
}

type fakeDefaultLocale struct{ code string }

func (f fakeDefaultLocale) DefaultLocale(context.Context) (string, error) { return f.code, nil }

func svcWith(repo catalog.Repository, deps ...func(*catalog.Deps)) *catalog.Service {
	d := catalog.Deps{Repo: repo}
	for _, fn := range deps {
		fn(&d)
	}
	return catalog.NewService(d)
}

// --- GenerateConfigurableChildren: validation characterization ---------------

func TestGenerateConfigurableChildren_Validation(t *testing.T) {
	parentID := uuid.New()
	colorID := uuid.New()
	red, blue := uuid.New(), uuid.New()

	// A configurable parent with one option attribute "color" (red, blue)
	base := func() *fakeRepo {
		return &fakeRepo{
			productByID: map[uuid.UUID]catalog.Product{
				parentID: {ID: parentID, URL: "shirt", Kind: catalog.KindConfigurable, DefaultCurrencyCode: "USD"},
			},
			configOptions: []catalog.Attribute{{ID: colorID, Slug: "color"}},
			attrWithValues: map[uuid.UUID]catalog.Attribute{
				colorID: {ID: colorID, Slug: "color", Values: []catalog.AttributeValue{
					{ID: red, AttributeID: colorID, Slug: "red"},
					{ID: blue, AttributeID: colorID, Slug: "blue"},
				}},
			},
		}
	}

	tests := []struct {
		name    string
		repo    *fakeRepo
		inputs  []catalog.GenerateConfigurableChildInput
		wantErr string
	}{
		{
			name:    "empty inputs",
			repo:    base(),
			inputs:  nil,
			wantErr: "at least one variant is required",
		},
		{
			name: "parent not configurable",
			repo: func() *fakeRepo {
				r := base()
				r.productByID[parentID] = catalog.Product{ID: parentID, URL: "shirt", Kind: catalog.KindSimple}
				return r
			}(),
			inputs:  []catalog.GenerateConfigurableChildInput{{AttributeValueIDs: []uuid.UUID{red}}},
			wantErr: "kind",
		},
		{
			name: "no configurable options",
			repo: func() *fakeRepo {
				r := base()
				r.configOptions = nil
				return r
			}(),
			inputs:  []catalog.GenerateConfigurableChildInput{{AttributeValueIDs: []uuid.UUID{red}}},
			wantErr: "no configurable options",
		},
		{
			name:    "wrong value count",
			repo:    base(),
			inputs:  []catalog.GenerateConfigurableChildInput{{AttributeValueIDs: []uuid.UUID{red, blue}}},
			wantErr: "expected 1 configurable option values, got 2",
		},
		{
			name:    "value not belonging to any option",
			repo:    base(),
			inputs:  []catalog.GenerateConfigurableChildInput{{AttributeValueIDs: []uuid.UUID{uuid.New()}}},
			wantErr: "does not belong to any configurable option",
		},
		{
			name: "duplicate combination within batch",
			repo: base(),
			inputs: []catalog.GenerateConfigurableChildInput{
				{AttributeValueIDs: []uuid.UUID{red}},
				{AttributeValueIDs: []uuid.UUID{red}},
			},
			wantErr: "same option combination",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := svcWith(tc.repo)
			_, err := svc.GenerateConfigurableChildren(context.Background(), parentID, tc.inputs)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got err %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestGenerateConfigurableChildren_ExistingCombinationRejected(t *testing.T) {
	parentID := uuid.New()
	colorID := uuid.New()
	red := uuid.New()
	existingChild := uuid.New()

	repo := &fakeRepo{
		productByID: map[uuid.UUID]catalog.Product{
			parentID: {ID: parentID, URL: "shirt", Kind: catalog.KindConfigurable, DefaultCurrencyCode: "USD"},
		},
		configOptions: []catalog.Attribute{{ID: colorID, Slug: "color"}},
		attrWithValues: map[uuid.UUID]catalog.Attribute{
			colorID: {ID: colorID, Slug: "color", Values: []catalog.AttributeValue{{ID: red, AttributeID: colorID, Slug: "red"}}},
		},
		// One existing child already occupies the "color=red" combination
		children: []catalog.Product{{ID: existingChild, URL: "shirt-red"}},
		selectableValues: map[uuid.UUID]map[uuid.UUID][]uuid.UUID{
			existingChild: {colorID: {red}},
		},
	}

	svc := svcWith(repo)
	_, err := svc.GenerateConfigurableChildren(context.Background(), parentID,
		[]catalog.GenerateConfigurableChildInput{{AttributeValueIDs: []uuid.UUID{red}}})
	if err == nil || !strings.Contains(err.Error(), "already exists on child") {
		t.Fatalf("got err %v, want 'already exists on child'", err)
	}
}

// A simple may be a variant of only ONE configurable - Linking a child that already belongs to a different configurable is rejected with a clean conflict (the UNIQUE(child_id) index is the DB backstop; this is the friendly service-layer error) - the unique index on catalog_product_children(child_id) is what refuses
func TestAddProductChild_RejectsSecondConfigurableParent(t *testing.T) {
	newParentID := uuid.New()
	otherParentID := uuid.New()
	childID := uuid.New()

	repo := &fakeRepo{
		productByID: map[uuid.UUID]catalog.Product{
			newParentID: {ID: newParentID, Kind: catalog.KindConfigurable},
			childID:     {ID: childID, Kind: catalog.KindSimple},
		},
		// The child is already a variant of a DIFFERENT configurable
		configurableParentLinks: map[uuid.UUID]catalog.ConfigurableParentLink{
			childID: {ParentID: otherParentID},
		},
	}

	svc := svcWith(repo)
	_, err := svc.AddProductChild(context.Background(), newParentID, childID, 0)
	if !errors.Is(err, catalog.ErrChildAlreadyHasParent) {
		t.Fatalf("got err %v, want ErrChildAlreadyHasParent", err)
	}
}

// --- CreateProduct: input-validation characterization ------------------------

func TestCreateProduct_Validation(t *testing.T) {
	ptID := uuid.New()
	valid := func() catalog.CreateProductInput {
		return catalog.CreateProductInput{
			ProductTypeID:       ptID,
			Kind:                catalog.KindSimple,
			Name:                "Widget",
			SKU:                 "WID-1",
			DefaultPrice:        100,
			DefaultCurrencyCode: "USD",
			Locale:              "en",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*catalog.CreateProductInput)
		deps    []func(*catalog.Deps)
		wantErr string
	}{
		{"empty locale", func(in *catalog.CreateProductInput) { in.Locale = "" }, nil, "locale is required"},
		{"empty name", func(in *catalog.CreateProductInput) { in.Name = "" }, nil, "name is required"},
		{"unknown kind", func(in *catalog.CreateProductInput) { in.Kind = "wat" }, nil, "wat"},
		{"negative price", func(in *catalog.CreateProductInput) { in.DefaultPrice = -1 }, nil, "default_price must be non-negative"},
		{"bad currency", func(in *catalog.CreateProductInput) { in.DefaultCurrencyCode = "US" }, nil, "currency"},
		{
			name:    "locale must be default",
			mutate:  func(in *catalog.CreateProductInput) { in.Locale = "fr" },
			deps:    []func(*catalog.Deps){func(d *catalog.Deps) { d.DefaultLocale = fakeDefaultLocale{code: "en"} }},
			wantErr: "got \"fr\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid()
			tc.mutate(&in)
			svc := svcWith(&fakeRepo{}, tc.deps...)
			_, err := svc.CreateProduct(context.Background(), in)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got err %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
