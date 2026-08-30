package catalog_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/tx"
)

// Same no-op sql driver trick as the cart and inventory suites - the fake repo
// ignores the *sql.Tx, only the Begin/Commit lifecycle has to exist
type catNoopDriver struct{}
type catNoopConn struct{}
type catNoopTx struct{}

func (catNoopDriver) Open(string) (driver.Conn, error)  { return catNoopConn{}, nil }
func (catNoopConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (catNoopConn) Close() error                        { return nil }
func (catNoopConn) Begin() (driver.Tx, error)           { return catNoopTx{}, nil }
func (catNoopTx) Commit() error                         { return nil }
func (catNoopTx) Rollback() error                       { return nil }

var registerCatNoopDriver sync.Once

func newCatTxManager(t *testing.T) *tx.Manager {
	t.Helper()
	registerCatNoopDriver.Do(func() { sql.Register("catalog-noop", catNoopDriver{}) })
	db, err := sql.Open("catalog-noop", "")
	if err != nil {
		t.Fatalf("open noop db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return tx.New(db)
}

// variantModeRepo extends the shared fakeRepo with the write + hydrating reads
// the SetProductAttributes revalidation path touches
type variantModeRepo struct {
	fakeRepo
	productWithAttrs catalog.Product
	productType      catalog.ProductType
	modeWrites       []catalog.VariantMode
}

func (f *variantModeRepo) WithTx(*sql.Tx) catalog.Repository { return f }

func (f *variantModeRepo) GetProductWithAttributes(_ context.Context, _ uuid.UUID) (catalog.Product, error) {
	return f.productWithAttrs, nil
}

func (f *variantModeRepo) GetProductTypeWithAttributes(_ context.Context, _ uuid.UUID) (catalog.ProductType, error) {
	return f.productType, nil
}

func (f *variantModeRepo) SetProductAttributes(context.Context, uuid.UUID, catalog.ProductScalarAttributes, []catalog.ProductAttributeSelection) error {
	return nil
}

func (f *variantModeRepo) SetProductVariantMode(_ context.Context, _ uuid.UUID, mode catalog.VariantMode) (catalog.Product, error) {
	f.modeWrites = append(f.modeWrites, mode)
	return catalog.Product{}, nil
}

func listMode() *catalog.VariantMode {
	m := catalog.VariantModeList
	return &m
}

func TestSetProductVariantMode_AttributeFlipGuard(t *testing.T) {
	parentID := uuid.New()

	t.Run("rejected while list children exist without options", func(t *testing.T) {
		repo := &variantModeRepo{
			productByID: map[uuid.UUID]catalog.Product{
				parentID: {ID: parentID, Kind: catalog.KindConfigurable, VariantMode: listMode()},
			},
			children: []catalog.Product{{ID: uuid.New(), Kind: catalog.KindSimple}}}
		svc := svcWith(repo)
		_, err := svc.SetProductVariantMode(context.Background(), parentID, catalog.VariantModeAttribute)
		if !errors.Is(err, catalog.ErrConfigurableNeedsOption) {
			t.Fatalf("got %v, want ErrConfigurableNeedsOption - with zero options matchVariant would sell the first child", err)
		}
		if len(repo.modeWrites) != 0 {
			t.Fatal("mode must not be written on a rejected flip")
		}
	})

	t.Run("allowed once the children are unlinked", func(t *testing.T) {
		repo := &variantModeRepo{
			productByID: map[uuid.UUID]catalog.Product{
				parentID: {ID: parentID, Kind: catalog.KindConfigurable, VariantMode: listMode()},
			}}
		svc := svcWith(repo)
		if _, err := svc.SetProductVariantMode(context.Background(), parentID, catalog.VariantModeAttribute); err != nil {
			t.Fatalf("SetProductVariantMode: %v", err)
		}
		if len(repo.modeWrites) != 1 || repo.modeWrites[0] != catalog.VariantModeAttribute {
			t.Fatalf("mode write missing, got %v", repo.modeWrites)
		}
	})

	t.Run("flip to list still rejected while options exist", func(t *testing.T) {
		repo := &variantModeRepo{
			productByID: map[uuid.UUID]catalog.Product{
				parentID: {ID: parentID, Kind: catalog.KindConfigurable},
			},
			configOptions: []catalog.Attribute{{ID: uuid.New(), Slug: "color"}}}
		svc := svcWith(repo)
		if _, err := svc.SetProductVariantMode(context.Background(), parentID, catalog.VariantModeList); !errors.Is(err, catalog.ErrListModeForbidsOptions) {
			t.Fatalf("got %v, want ErrListModeForbidsOptions", err)
		}
	})
}

// An attribute edit clearing a child's option value must be rejected - the
// variant picker could never match it, leaving a silently unsellable child
func TestSetProductAttributes_RejectsClearingOptionValue(t *testing.T) {
	parentID := uuid.New()
	childID := uuid.New()
	siblingID := uuid.New()
	colorID := uuid.New()
	red := uuid.New()
	typeID := uuid.New()

	color := catalog.Attribute{ID: colorID, Slug: "color", InputType: catalog.InputTypeDropdown}
	repo := &variantModeRepo{
		productByID: map[uuid.UUID]catalog.Product{
			parentID: {ID: parentID, Kind: catalog.KindConfigurable},
		},
		configOptions: []catalog.Attribute{color},
		// Post-edit state: the edited child carries NO color value, its sibling keeps red
		children: []catalog.Product{
			{ID: childID, Kind: catalog.KindSimple},
			{ID: siblingID, Kind: catalog.KindSimple},
		},
		selectableValues: map[uuid.UUID]map[uuid.UUID][]uuid.UUID{
			siblingID: {colorID: {red}},
			childID:   {},
		},
		productParents: map[uuid.UUID][]catalog.Product{
			childID: {{ID: parentID, Kind: catalog.KindConfigurable}},
		},
		productWithAttrs: catalog.Product{ID: childID, Kind: catalog.KindSimple, ProductTypeID: typeID},
		productType: catalog.ProductType{
			ID:         typeID,
			Attributes: []catalog.ProductTypeAttribute{{Attribute: color}},
		},
	}

	svc := svcWith(repo, func(d *catalog.Deps) { d.Tx = newCatTxManager(t) })
	_, err := svc.SetProductAttributes(context.Background(), childID, []catalog.AttributeAssignment{
		{AttributeID: colorID, ValueIDs: []uuid.UUID{}},
	})
	if err == nil || !strings.Contains(err.Error(), "color") {
		t.Fatalf("clearing a super-attribute value must be rejected naming the option, got %v", err)
	}
	if !errors.Is(err, catalog.ErrChildMissingConfigurableOptions) {
		t.Fatalf("got %v, want ErrChildMissingConfigurableOptions", err)
	}
}
