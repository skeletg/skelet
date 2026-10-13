package catalog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

func (r *catalogRig) listen(types ...string) {
	for _, et := range types {
		r.bus.Subscribe(et, func(_ context.Context, e events.Event) error {
			r.mu.Lock()
			r.events = append(r.events, e)
			r.mu.Unlock()
			return nil
		})
	}
}

func eventsOf[T events.Event](evs []events.Event) []T {
	var out []T
	for _, e := range evs {
		if v, ok := e.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func (r *catalogRig) sweep() {
	r.t.Cleanup(func() {
		like := "%" + r.sfx + "%"
		for _, q := range []string{
			`DELETE FROM catalog_product_children WHERE parent_id IN (SELECT id FROM catalog_products WHERE sku LIKE $1)`,
			`DELETE FROM catalog_product_configurable_options WHERE product_id IN (SELECT id FROM catalog_products WHERE sku LIKE $1)`,
			`DELETE FROM inventory_items WHERE product_id IN (SELECT id FROM catalog_products WHERE sku LIKE $1)`,
			`DELETE FROM catalog_products WHERE sku LIKE $1`,
			`DELETE FROM catalog_product_types WHERE slug LIKE $1`,
			`DELETE FROM catalog_attributes WHERE slug LIKE $1`,
			`DELETE FROM channels WHERE slug LIKE $1`,
		} {
			_, _ = r.conn.Exec(q, like)
		}
	})
}

func (r *catalogRig) ptype(template string, tracks bool) ProductType {
	r.t.Helper()
	pt, err := r.svc.CreateProductType(context.Background(), CreateProductTypeInput{
		Slug: "pt-" + uuid.NewString()[:6] + "-" + r.sfx, Name: "pt", URLTemplate: template, TracksInventory: tracks,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return pt
}

func (r *catalogRig) newProduct(typeID uuid.UUID, kind ProductKind, sku, name, currency string) Product {
	r.t.Helper()
	p, err := r.svc.CreateProduct(context.Background(), CreateProductInput{
		ProductTypeID: typeID, Kind: kind, SKU: sku, Name: name, Locale: "en", DefaultPrice: 1000, DefaultCurrencyCode: currency,
	})
	if err != nil {
		r.t.Fatalf("create %q: %v", sku, err)
	}
	return p
}

func (r *catalogRig) dropdown(name string, values ...string) Attribute {
	r.t.Helper()
	in := CreateAttributeInput{Slug: name + "-" + r.sfx, Name: name, InputType: InputTypeDropdown, IsPublic: true}
	for _, v := range values {
		in.Values = append(in.Values, AttributeValueInput{Slug: v, Name: v})
	}
	a, err := r.svc.CreateAttribute(context.Background(), in)
	if err != nil {
		r.t.Fatal(err)
	}
	full, err := r.svc.GetAttribute(context.Background(), a.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	return full
}

func (r *catalogRig) channelIn(currency string) uuid.UUID {
	r.t.Helper()
	r.exec(`INSERT INTO currencies (code, name) VALUES ($1, $1) ON CONFLICT DO NOTHING`, currency)
	return r.id(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rig', $2) RETURNING id`, "ch-"+uuid.NewString()[:6]+"-"+r.sfx, currency)
}

type dbChannels struct{ conn *sql.DB }

func (c dbChannels) GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	ch := channel.Channel{ID: id}
	err := c.conn.QueryRowContext(ctx, `SELECT currency_code FROM channels WHERE id = $1`, id).Scan(&ch.CurrencyCode)
	return ch, err
}

func valueID(a Attribute, slug string) uuid.UUID {
	for _, v := range a.Values {
		if v.Slug == slug {
			return v.ID
		}
	}
	panic("no value " + slug)
}

func TestAVariantEditThatDuplicatesASiblingPersistsNothing(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	color := r.dropdown("color", "red", "blue")
	if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: pt.ID, AttributeID: color.ID}); err != nil {
		t.Fatal(err)
	}
	parent := r.newProduct(pt.ID, KindConfigurable, "PAR-"+r.sfx, "Shirt", "USD")
	if _, err := r.svc.SetConfigurableOptions(ctx, parent.ID, []uuid.UUID{color.ID}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ sku, value string }{{"RED-" + r.sfx, "red"}, {"BLUE-" + r.sfx, "blue"}} {
		child := r.newProduct(pt.ID, KindSimple, c.sku, c.sku, "USD")
		if _, err := r.svc.SetProductAttributes(ctx, child.ID, []AttributeAssignment{{AttributeID: color.ID, ValueIDs: []uuid.UUID{valueID(color, c.value)}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.svc.AddProductChild(ctx, parent.ID, child.ID, 0); err != nil {
			t.Fatal(err)
		}
	}
	blue, err := r.svc.GetProductBySKU(ctx, "BLUE-"+r.sfx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.svc.SetProductAttributes(ctx, blue.ID, []AttributeAssignment{{AttributeID: color.ID, ValueIDs: []uuid.UUID{valueID(color, "red")}}})
	if !errors.Is(err, ErrDuplicateSuperAttributeCombination) {
		t.Fatalf("err = %v, want ErrDuplicateSuperAttributeCombination", err)
	}
	var held uuid.UUID
	if err := r.conn.QueryRow(`SELECT attribute_value_id FROM catalog_product_attribute_values WHERE product_id = $1 AND attribute_id = $2`, blue.ID, color.ID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != valueID(color, "blue") {
		t.Errorf("the refused edit persisted: blue now holds value %s", held)
	}
}

func TestOnlyADerivedSKUIsSuffixedOnCollision(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	sku := "EXPLICIT-" + r.sfx
	r.newProduct(pt.ID, KindSimple, sku, "First", "USD")
	if _, err := r.svc.CreateProduct(ctx, CreateProductInput{ProductTypeID: pt.ID, SKU: sku, Name: "Second", Locale: "en", DefaultCurrencyCode: "USD"}); !errors.Is(err, ErrSKUTaken) {
		t.Errorf("explicit duplicate SKU = %v, want ErrSKUTaken", err)
	}
	name := "gizmo " + r.sfx
	a := r.newProduct(pt.ID, KindSimple, "", name, "USD")
	b := r.newProduct(pt.ID, KindSimple, "", name, "USD")
	if a.SKU != "gizmo-"+r.sfx || b.SKU != "gizmo-"+r.sfx+"-1" {
		t.Errorf("derived SKUs = %q, %q, want the second suffixed", a.SKU, b.SKU)
	}
}

type failingInventory struct{}

func (failingInventory) EnsureItem(context.Context, uuid.UUID, bool) error { return errors.New("down") }
func (failingInventory) SyncProductTracking(context.Context, uuid.UUID, bool) error {
	return errors.New("down")
}

func TestACommittedWriteAnnouncesItselfEvenWhenAFollowUpFails(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.listen(EventProductCreated, EventProductDeleted)
	ctx := context.Background()
	pt := r.ptype("", true)
	r.svc.SetInventoryEnsurer(failingInventory{})

	_, err := r.svc.CreateProduct(ctx, CreateProductInput{ProductTypeID: pt.ID, SKU: "INV-" + r.sfx, Name: "Inv", Locale: "en", DefaultCurrencyCode: "USD"})
	if err == nil {
		t.Fatal("the failing inventory step didn't surface")
	}
	created, err := r.svc.GetProductBySKU(ctx, "INV-"+r.sfx)
	if err != nil {
		t.Fatalf("the product row should be committed: %v", err)
	}
	if got := eventsOf[*ProductCreated](r.published()); len(got) != 1 || got[0].ProductID != created.ID {
		t.Errorf("product.created = %v, want one for the committed row", got)
	}

	tracked := false
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: created.ID, TracksInventoryOverride: &tracked}); err == nil {
		t.Fatal("the failing tracking sync didn't surface")
	}
	if got := eventsOf[*ProductUpdated](r.published()); len(got) == 0 || got[len(got)-1].ProductID != created.ID {
		t.Errorf("product.updated = %v, want one for the committed update", got)
	}

	r.svc.SetInventoryEnsurer(nil)
	if err := r.svc.DeleteProduct(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if got := eventsOf[*ProductDeleted](r.published()); len(got) != 1 || got[0].SKU != created.SKU {
		t.Errorf("product.deleted = %v, want one carrying the SKU", got)
	}
}

func TestATemplateChangeRecomputesAndAnnouncesItsProducts(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.svc.RegisterSubscribers(r.bus)
	ctx := context.Background()
	pt := r.ptype("{name}", false)
	p := r.newProduct(pt.ID, KindSimple, "TPL-"+r.sfx, "Lamp "+r.sfx, "USD")
	r.reset()

	if _, err := r.svc.SetProductTypeURLPathTemplate(ctx, pt.ID, "lights/{name}"); err != nil {
		t.Fatal(err)
	}
	evs := r.published()
	if got := r.productURL(p.ID); got != "lights/lamp-"+r.sfx {
		t.Errorf("url after the template change = %q", got)
	}
	if !slices.ContainsFunc(eventsOf[*ProductUpdated](evs), func(e *ProductUpdated) bool { return e.ProductID == p.ID }) {
		t.Error("the recomputed product published no product.updated")
	}

	r.reset()
	tpl := "lamps/{name}"
	if _, err := r.svc.UpdateProductType(ctx, UpdateProductTypeInput{ID: pt.ID, URLTemplate: &tpl}); err != nil {
		t.Fatal(err)
	}
	r.published()
	if got := r.productURL(p.ID); got != "lamps/lamp-"+r.sfx {
		t.Errorf("UpdateProductType changed the template without recomputing: url = %q", got)
	}
}

func TestAttributeChangesAnnounceTheProductsCarryingThem(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.listen(EventAttributeUpdated, EventAttributeDeleted)
	ctx := context.Background()
	pt := r.ptype("", false)
	color := r.dropdown("shade", "red")
	note, err := r.svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "note-" + r.sfx, Name: "note", InputType: InputTypePlainText, IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []uuid.UUID{color.ID, note.ID} {
		if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: pt.ID, AttributeID: a}); err != nil {
			t.Fatal(err)
		}
	}
	p := r.newProduct(pt.ID, KindSimple, "ATTR-"+r.sfx, "Attr", "USD")
	if _, err := r.svc.SetProductAttributes(ctx, p.ID, []AttributeAssignment{
		{AttributeID: color.ID, ValueIDs: []uuid.UUID{valueID(color, "red")}},
		{AttributeID: note.ID, HasScalar: true, ScalarValue: "old note"},
	}); err != nil {
		t.Fatal(err)
	}
	r.reset()

	private := false
	if _, err := r.svc.UpdateAttribute(ctx, UpdateAttributeInput{ID: color.ID, IsPublic: &private}); err != nil {
		t.Fatal(err)
	}
	upd := eventsOf[*AttributeUpdated](r.published())
	if len(upd) != 1 || !slices.Contains(upd[0].ChangedFields, AttributeFieldIsPublic) || !slices.Contains(upd[0].ProductIDs, p.ID) {
		t.Fatalf("attribute.updated = %+v, want the isPublic flip naming the product", upd)
	}

	r.reset()
	if err := r.svc.DeleteAttribute(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	del := eventsOf[*AttributeDeleted](r.published())
	if len(del) != 1 || !slices.Contains(del[0].ProductIDs, p.ID) {
		t.Fatalf("attribute.deleted = %+v, want it naming the product", del)
	}
	again, err := r.svc.CreateAttribute(ctx, CreateAttributeInput{Slug: note.Slug, Name: "note", InputType: InputTypePlainText})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: pt.ID, AttributeID: again.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.GetProductWithAttributes(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got.Attributes[note.Slug]; ok {
		t.Errorf("the recreated slug resurfaced the deleted attribute's value %v", v)
	}
}

func TestProductTypeHydrationKeepsEveryAttributeField(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	a := r.dropdown("full", "x")
	r.exec(`UPDATE catalog_attributes SET unit = 'cm', value_required = TRUE, filterable = TRUE, searchable = TRUE, is_public = TRUE,
		facet_collapsed = TRUE, is_indexable = TRUE, h1_position = 'after', synapse_track = TRUE, auto_segment = TRUE, sort_order = 7,
		metadata = '{"k":"v"}', private_metadata = '{"p":"q"}', default_value = to_jsonb($2::text), relationship_target = NULL WHERE id = $1`, a.ID, valueID(a, "x").String())
	if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: pt.ID, AttributeID: a.ID}); err != nil {
		t.Fatal(err)
	}
	want, err := r.repo.GetAttributeByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	full, err := r.repo.GetProductTypeWithAttributes(ctx, pt.ID)
	if err != nil || len(full.Attributes) != 1 {
		t.Fatalf("hydrated type = %+v, %v", full, err)
	}
	got := full.Attributes[0].Attribute
	got.Values, want.Values = nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hydration diverged from the attribute row:\n got %+v\nwant %+v", got, want)
	}
	v := reflect.ValueOf(got)
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if name == "RelationshipTarget" || name == "Values" {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("Attribute.%s came back zero through GetProductTypeWithAttributes", name)
		}
	}
}

func TestAttributeGroupsStayWithinTheirProductType(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	mine, other := r.ptype("", false), r.ptype("", false)
	a := r.dropdown("grp", "x")
	if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: mine.ID, AttributeID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.CreateAttributeGroup(ctx, CreateAttributeGroupInput{ProductTypeID: other.ID, Name: "Specs"}); err != nil {
		t.Fatal(err)
	}
	groups, err := r.repo.ListAttributeGroupsByProductType(ctx, other.ID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %v, %v", groups, err)
	}
	gid := groups[0].ID
	if _, err := r.svc.MoveAttributeToGroup(ctx, mine.ID, a.ID, &gid); !errors.Is(err, ErrAttributeGroupWrongType) {
		t.Errorf("move into another type's group = %v, want ErrAttributeGroupWrongType", err)
	}
	if _, err := r.svc.AssignAttribute(ctx, AssignAttributeInput{ProductTypeID: mine.ID, AttributeID: a.ID, GroupID: &gid}); !errors.Is(err, ErrAttributeGroupWrongType) {
		t.Errorf("assign into another type's group = %v, want ErrAttributeGroupWrongType", err)
	}
	unassigned := r.dropdown("loose", "x")
	if _, err := r.svc.MoveAttributeToGroup(ctx, mine.ID, unassigned.ID, nil); !errors.Is(err, ErrAssignmentNotFound) {
		t.Errorf("move of an unassigned attribute = %v, want ErrAssignmentNotFound", err)
	}
	if _, err := r.svc.CreateAttributeGroup(ctx, CreateAttributeGroupInput{ProductTypeID: other.ID, Name: "Specs"}); !errors.Is(err, ErrAttributeGroupNameTaken) {
		t.Errorf("duplicate group name = %v, want ErrAttributeGroupNameTaken", err)
	}
	if _, err := r.svc.UpdateAttributeGroup(ctx, UpdateAttributeGroupInput{ID: uuid.New()}); !errors.Is(err, ErrAttributeGroupNotFound) {
		t.Errorf("unknown group = %v, want ErrAttributeGroupNotFound", err)
	}
}

func TestAChannelSlugCollisionIsAConflict(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	a := r.dropdown("cs", "x", "y")
	ch := r.channelIn("USD")
	if _, err := r.svc.SetAttributeValueChannelSlug(ctx, valueID(a, "x"), ch, "same"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.SetAttributeValueChannelSlug(ctx, valueID(a, "y"), ch, "same"); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("second value on the same slug = %v, want ErrSlugTaken", err)
	}
}

func TestListingPricesCannotGoNegative(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "NEG-"+r.sfx, "Neg", "USD")
	ch := r.channelIn("USD")
	neg := int64(-1)
	for name, in := range map[string]ProductChannelListingInput{
		"price":      {Price: &neg},
		"compare at": {CompareAtPrice: &neg},
		"special":    {SpecialPrice: &neg},
	} {
		in.ProductID, in.ChannelID = p.ID, ch
		if _, err := r.svc.SetProductListing(ctx, in); !errors.Is(err, ErrNegativePrice) {
			t.Errorf("%s: err = %v, want ErrNegativePrice", name, err)
		}
	}
	if _, err := r.conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price) VALUES ($1, $2, -5)`, p.ID, ch); err == nil {
		t.Error("the table accepted a negative price")
	}
	if _, err := r.svc.SetGroupPrices(ctx, p.ID, []GroupPriceInput{{MinQty: 2, Price: -1}}); !errors.Is(err, ErrNegativePrice) {
		t.Errorf("negative tier = %v, want ErrNegativePrice", err)
	}
}

func TestAPublishToggleKeepsTheListingsPricesAndURL(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.svc.channelSvc = dbChannels{r.conn}
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "PUB-"+r.sfx, "Pub", "USD")
	ch := r.channelIn("USD")
	price, special, url := int64(500), int64(400), "pub-"+r.sfx
	if _, err := r.svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: p.ID, ChannelID: ch, Price: &price, SpecialPrice: &special, URL: &url, IsPublished: true}); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.SetProductListingPublished(ctx, p.ID, ch, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPublished || got.Price == nil || *got.Price != price || got.SpecialPrice == nil || *got.SpecialPrice != special || got.URL == nil || *got.URL != url {
		t.Errorf("listing after the toggle = %+v, want only is_published changed", got)
	}
	eur := r.channelIn("EUR")
	if _, err := r.svc.SetProductListingPublished(ctx, p.ID, eur, true); !errors.Is(err, ErrListingCurrencyMismatch) {
		t.Errorf("a new inheriting row on a foreign-currency channel = %v, want ErrListingCurrencyMismatch", err)
	}
}

func TestACurrencyChangeCannotStrandAnInheritingListing(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.svc.channelSvc = dbChannels{r.conn}
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "CUR-"+r.sfx, "Cur", "USD")
	ch := r.channelIn("USD")
	if _, err := r.svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: p.ID, ChannelID: ch, IsPublished: true}); err != nil {
		t.Fatal(err)
	}
	eur := "EUR"
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, DefaultCurrencyCode: &eur}); !errors.Is(err, ErrListingCurrencyMismatch) {
		t.Fatalf("err = %v, want ErrListingCurrencyMismatch", err)
	}
	price := int64(900)
	if _, err := r.svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: p.ID, ChannelID: ch, Price: &price, IsPublished: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, DefaultCurrencyCode: &eur}); err != nil {
		t.Errorf("with the listing priced explicitly the change should pass: %v", err)
	}
}

func TestAGlobalTierAppliesOnlyInTheProductsCurrency(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	r.svc.channelSvc = dbChannels{r.conn}
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "TIER-"+r.sfx, "Tier", "EUR")
	eur, rsd := r.channelIn("EUR"), r.channelIn("RSD")
	if _, err := r.svc.SetGroupPrices(ctx, p.ID, []GroupPriceInput{{MinQty: 10, Price: 800}, {ChannelID: &rsd, MinQty: 10, Price: 90000}}); err != nil {
		t.Fatal(err)
	}
	onEUR, err := r.svc.ApplicableGroupPrices(ctx, p.ID, &eur, nil)
	if err != nil || len(onEUR) != 1 || onEUR[0].Price != 800 {
		t.Errorf("EUR channel tiers = %+v, %v, want the global one", onEUR, err)
	}
	onRSD, err := r.svc.ApplicableGroupPrices(ctx, p.ID, &rsd, nil)
	if err != nil || len(onRSD) != 1 || onRSD[0].Price != 90000 {
		t.Errorf("RSD channel tiers = %+v, %v, want only its own row - the EUR amount isn't an RSD price", onRSD, err)
	}
}

func TestConcurrentProductEditsKeepEachOthersFields(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "LOCK-"+r.sfx, "Lock", "USD")
	other, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Exec(`UPDATE catalog_products SET cost_cents = 777 WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		compareAt := int64(5000)
		_, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, CompareAtPrice: &compareAt})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond) // the update is parked on the row lock by now
	if err := other.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var cost, compareAt sql.NullInt64
	if err := r.conn.QueryRow(`SELECT cost_cents, compare_at_price FROM catalog_products WHERE id = $1`, p.ID).Scan(&cost, &compareAt); err != nil {
		t.Fatal(err)
	}
	if cost.Int64 != 777 || compareAt.Int64 != 5000 {
		t.Errorf("cost=%v compareAt=%v - one edit overwrote the other", cost, compareAt)
	}

	zero := int64(0)
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, CostCents: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := r.conn.QueryRow(`SELECT cost_cents FROM catalog_products WHERE id = $1`, p.ID).Scan(&cost); err != nil || !cost.Valid || cost.Int64 != 0 {
		t.Errorf("a zero cost read back as %v (%v), want a stored 0", cost, err)
	}
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, ClearCostCents: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.conn.QueryRow(`SELECT cost_cents FROM catalog_products WHERE id = $1`, p.ID).Scan(&cost); err != nil || cost.Valid {
		t.Errorf("ClearCostCents left %v", cost)
	}
}

func TestTheStorefrontReadSkipsPendingMedia(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "MED-"+r.sfx, "Med", "USD")
	ready, pending := uuid.New(), uuid.New()
	r.exec(`INSERT INTO catalog_product_media (id, product_id, s3_key, mime_type, status) VALUES ($1, $3, 'a', 'image/png', 'ready'), ($2, $3, 'b', 'image/png', 'pending')`, ready, pending, p.ID)
	got, err := r.svc.ListReadyProductMediaByProductIDs(ctx, []uuid.UUID{p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[p.ID]) != 1 || got[p.ID][0].ID != ready {
		t.Errorf("storefront media = %+v, want only the ready row", got[p.ID])
	}

	if _, err := r.svc.ReorderProductMedia(ctx, p.ID, []uuid.UUID{pending, ready, pending}); err != nil {
		t.Fatal(err)
	}
	var first uuid.UUID
	if err := r.conn.QueryRow(`SELECT id FROM catalog_product_media WHERE product_id = $1 ORDER BY sort_order LIMIT 1`, p.ID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if order := []uuid.UUID{first}; first != pending {
		t.Errorf("order after a duplicated id = %v, want its first position kept", order)
	}
	if _, err := r.svc.ReorderProductMedia(ctx, p.ID, []uuid.UUID{ready, uuid.New()}); !errors.Is(err, ErrMediaWrongProduct) {
		t.Errorf("a foreign id = %v, want ErrMediaWrongProduct", err)
	}
}

func TestASKUInAnActiveCartCannotBeRenamed(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "CART-"+r.sfx, "Cart", "USD")
	ch := r.channelIn("USD")
	cart := r.id(`INSERT INTO carts (channel_id) VALUES ($1) RETURNING id`, ch)
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM carts WHERE id = $1`, cart) })
	r.exec(`INSERT INTO cart_saved_items (cart_id, product_sku, product_name) VALUES ($1, $2, 'x')`, cart, p.SKU)
	next := "RENAMED-" + r.sfx
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, SKU: &next}); !errors.Is(err, ErrSKUInActiveCarts) {
		t.Fatalf("rename with a saved item = %v, want ErrSKUInActiveCarts", err)
	}
	r.exec(`UPDATE carts SET status = 'converted' WHERE id = $1`, cart)
	if _, err := r.svc.UpdateProduct(ctx, UpdateProductInput{ID: p.ID, SKU: &next}); err != nil {
		t.Errorf("rename once the cart closed: %v", err)
	}
}

func TestTheChannelProductListFiltersBeforeTheCap(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	listed := r.newProduct(pt.ID, KindSimple, "LISTED-"+r.sfx, "Listed", "USD")
	r.newProduct(pt.ID, KindSimple, "NEWER-"+r.sfx, "Newer", "USD")
	ch := r.channelIn("USD")
	r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price) VALUES ($1, $2, 1)`, listed.ID, ch)
	got, err := r.svc.ListProductsWithGraph(ctx, "en", 1, &ch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != listed.ID {
		t.Errorf("channel list capped at 1 = %v, want the listed product even though a newer one exists", got)
	}
}

func TestChannelPathsResolveOnlyPublishedBoundedOverrides(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "PATH-"+r.sfx, "Path", "USD")
	ch := r.channelIn("USD")
	path := "draft-" + r.sfx
	r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price, url, is_published) VALUES ($1, $2, 1, $3, FALSE)`, p.ID, ch, path)
	if _, err := r.svc.ResolveProductIDForChannelPath(ctx, ch, path); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("an unpublished override resolved: %v", err)
	}
	r.exec(`UPDATE catalog_product_channel_listings SET is_published = TRUE WHERE product_id = $1`, p.ID)
	if id, err := r.svc.ResolveProductIDForChannelPath(ctx, ch, path); err != nil || id != p.ID {
		t.Errorf("published override = %v, %v", id, err)
	}
	dsn := os.Getenv("VALKEY_URL")
	if dsn == "" {
		t.Skip("set VALKEY_URL to exercise the negative cache bound")
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	r.svc.channelPathCache = cache.New(client, "test:pathbound:"+r.sfx)
	long := strings.Repeat("a", maxChannelPathLen+1)
	if _, err := r.svc.ResolveProductIDForChannelPath(ctx, ch, long); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("over-long path = %v, want not found", err)
	}
	if _, hit := cache.GetJSON[cachedChannelPath](ctx, r.svc.channelPathCache, channelPathKey(ch, r.svc.channelPathVersion(ctx, ch), long)); hit {
		t.Error("an over-long path landed in the negative cache")
	}
}

func TestBulkWritesAreBoundedAndNameTheirMissingTargets(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "BULK-"+r.sfx, "Bulk", "USD")
	ghost := uuid.New()
	if _, err := r.svc.SetGroupPrices(ctx, p.ID, []GroupPriceInput{{ChannelID: &ghost, MinQty: 1, Price: 1}}); !errors.Is(err, channel.ErrChannelNotFound) {
		t.Errorf("unknown tier channel = %v, want ErrChannelNotFound", err)
	}
	if _, err := r.svc.SetGroupPrices(ctx, p.ID, make([]GroupPriceInput, maxGroupPriceTiers+1)); !errors.Is(err, ErrTooManyItems) {
		t.Errorf("oversized tier set = %v, want ErrTooManyItems", err)
	}
}

func TestSideWritesAnnounceTheProduct(t *testing.T) {
	r := newCatalogRig(t)
	r.sweep()
	ctx := context.Background()
	pt := r.ptype("", false)
	p := r.newProduct(pt.ID, KindSimple, "SIDE-"+r.sfx, "Side", "USD")
	r.exec(`INSERT INTO locales (code, name) VALUES ('xx', 'xx') ON CONFLICT DO NOTHING`)
	if _, err := r.svc.UpsertProductContent(ctx, p.ID, ProductContentInput{Locale: "xx", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"groupPrices": func() error {
			_, err := r.svc.SetGroupPrices(ctx, p.ID, []GroupPriceInput{{MinQty: 2, Price: 1}})
			return err
		},
		"content": func() error { return r.svc.DeleteProductContent(ctx, p.ID, "xx") },
		"media":   func() error { _, err := r.svc.ReorderProductMedia(ctx, p.ID, nil); return err },
		"subscriptionTerms": func() error {
			return r.svc.ClearSubscriptionTerms(ctx, p.ID)
		},
	} {
		r.reset()
		if err := write(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.ContainsFunc(eventsOf[*ProductUpdated](r.published()), func(e *ProductUpdated) bool {
			return e.ProductID == p.ID && slices.Contains(e.ChangedFields, name)
		}) {
			t.Errorf("%s write published no product.updated", name)
		}
	}
}
