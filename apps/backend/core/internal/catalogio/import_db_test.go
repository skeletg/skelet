package catalogio

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type ioRig struct {
	t    *testing.T
	conn *sql.DB
	svc  *catalog.Service
	reg  *dataio.Registry
	sfx  string
	pt   catalog.ProductType
}

type fixedLocale string

func (f fixedLocale) DefaultLocale(context.Context) (string, error) { return string(f), nil }

type dbChannels struct{ conn *sql.DB }

func (c dbChannels) GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	ch := channel.Channel{ID: id}
	err := c.conn.QueryRowContext(ctx, `SELECT currency_code FROM channels WHERE id = $1`, id).Scan(&ch.CurrencyCode)
	return ch, err
}

func newIORig(t *testing.T) *ioRig {
	t.Helper()
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	svc := catalog.NewService(catalog.Deps{
		Repo: catalog.NewRepository(db.New(conn), tx), Tx: tx, Bus: events.NewBus(),
		Channel: dbChannels{conn}, DefaultLocale: fixedLocale("en"),
	})
	r := &ioRig{t: t, conn: conn, svc: svc, reg: dataio.NewRegistry(), sfx: strings.ToLower(uuid.NewString()[:8])}
	r.reg.Register(Resource(db.New(conn), svc))
	r.reg.Register(CategoryResource(svc))
	for _, c := range []string{"USD", "EUR"} {
		r.exec(`INSERT INTO currencies (code, name) VALUES ($1, $1) ON CONFLICT DO NOTHING`, c)
	}
	pt, err := svc.CreateProductType(context.Background(), catalog.CreateProductTypeInput{Slug: "pt-" + r.sfx, Name: "pt", URLTemplate: "{name}"})
	if err != nil {
		t.Fatal(err)
	}
	r.pt = pt
	return r
}

func (r *ioRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.conn.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *ioRig) channel(currency string) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	if err := r.conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rig', $2) RETURNING id`,
		"ch-"+uuid.NewString()[:6]+"-"+r.sfx, currency).Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r *ioRig) sku(s string) string { return s + "-" + r.sfx }

// Placeholders: {pt} is the rig's product type slug, {s} the run suffix
func (r *ioRig) file(lines ...string) string {
	return strings.NewReplacer("{pt}", r.pt.Slug, "{s}", r.sfx).Replace(strings.Join(lines, "\n")) + "\n"
}

func (r *ioRig) importFile(resource, body string, dryRun bool, ch uuid.UUID, opts map[string]string) dataio.ImportResult {
	r.t.Helper()
	o := dataio.NewOpts(opts)
	o.DryRun, o.ChannelID = dryRun, ch
	res, err := r.reg.Import(context.Background(), dataio.Run{Resource: resource, Format: dataio.FormatCSV, Opts: o}, strings.NewReader(body))
	if err != nil {
		r.t.Fatalf("import: %v", err)
	}
	return res
}

func (r *ioRig) export(ch uuid.UUID) []map[string]string {
	r.t.Helper()
	o := dataio.NewOpts(nil)
	o.ChannelID = ch
	var buf bytes.Buffer
	if _, err := r.reg.Export(context.Background(), dataio.Run{Resource: resourceProducts, Format: dataio.FormatCSV, Opts: o}, &buf); err != nil {
		r.t.Fatalf("export: %v", err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		r.t.Fatal(err)
	}
	out := make([]map[string]string, 0, len(recs))
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, h := range recs[0] {
			m[h] = rec[i]
		}
		out = append(out, m)
	}
	return out
}

func (r *ioRig) product(sku string) (catalog.Product, bool) {
	r.t.Helper()
	p, err := r.svc.GetProductBySKU(context.Background(), r.sku(sku))
	if errors.Is(err, catalog.ErrProductNotFound) {
		return catalog.Product{}, false
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return p, true
}

func mustClean(t *testing.T, res dataio.ImportResult) {
	t.Helper()
	if res.Failed != 0 || len(res.Errors) != 0 {
		t.Fatalf("import failed: %+v", res.Errors)
	}
}

func TestADryRunNeverCreatesAttributeValues(t *testing.T) {
	r := newIORig(t)
	ctx := context.Background()
	attr, err := r.svc.CreateAttribute(ctx, catalog.CreateAttributeInput{
		Slug: "color-" + r.sfx, Name: "Color", InputType: catalog.InputTypeDropdown, Values: []catalog.AttributeValueInput{{Slug: "red", Name: "Red"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AssignAttribute(ctx, catalog.AssignAttributeInput{ProductTypeID: r.pt.ID, AttributeID: attr.ID}); err != nil {
		t.Fatal(err)
	}
	body := r.file(
		"sku,name,product_type,price,currency,attr:color-{s}",
		"A-{s},A,{pt},100,USD,Blue",
		"B-{s},B,{pt},100,USD,blue",
	)
	values := func() int {
		full, err := r.svc.GetAttribute(ctx, attr.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(full.Values)
	}
	opts := map[string]string{optCreateMissingValues: "true"}

	res := r.importFile(resourceProducts, body, true, uuid.Nil, opts)
	mustClean(t, res)
	if res.Created != 2 || values() != 1 {
		t.Fatalf("dry run: created %d rows and left %d values, want 2 rows previewed and the 1 value untouched", res.Created, values())
	}
	mustClean(t, r.importFile(resourceProducts, body, false, uuid.Nil, opts))
	if n := values(); n != 2 {
		t.Fatalf("real run left %d values, want the missing one created exactly once", n)
	}
}

func TestExportedURLPathsRoundTripWithoutPinningAutoURLs(t *testing.T) {
	r := newIORig(t)
	mustClean(t, r.importFile(resourceProducts, r.file(
		"sku,name,product_type,price,currency,url_path",
		"AUTO-{s},Auto,{pt},100,USD,",
		"MAN-{s},Manual,{pt},100,USD,hand-made-{s}",
	), false, uuid.Nil, nil))

	byCell := map[string]map[string]string{}
	var kept []string
	rows := r.export(uuid.Nil)
	header := []string{}
	for _, row := range rows {
		byCell[row[colSKU]] = row
	}
	auto, manual := byCell[r.sku("AUTO")], byCell[r.sku("MAN")]
	if auto == nil || manual == nil {
		t.Fatalf("export is missing the rig products: %v", rows)
	}
	if auto[colURLPath] != "" {
		t.Errorf("an auto-URL product exported url_path %q, want it blank", auto[colURLPath])
	}
	if manual[colURLPath] != "hand-made-"+r.sfx {
		t.Errorf("a manual product exported url_path %q", manual[colURLPath])
	}
	for h := range auto {
		if !strings.HasPrefix(h, attrPrefix) {
			header = append(header, h)
		}
	}
	for _, row := range []map[string]string{auto, manual} {
		cells := make([]string, len(header))
		for i, h := range header {
			cells[i] = row[h]
		}
		kept = append(kept, csvLine(cells))
	}
	mustClean(t, r.importFile(resourceProducts, csvLine(header)+"\n"+strings.Join(kept, "\n")+"\n", false, uuid.Nil, nil))

	if p, _ := r.product("AUTO"); !p.URLAuto {
		t.Error("re-importing the export pinned an auto-URL product to manual URLs")
	}
	if p, _ := r.product("MAN"); p.URLAuto || p.URL != "hand-made-"+r.sfx {
		t.Errorf("manual product came back as auto=%v url=%q", p.URLAuto, p.URL)
	}

	res := r.importFile(resourceProducts, r.file("sku,name,product_type,price,currency,slug", "OLD-{s},Old,{pt},100,USD,legacy-{s}"), false, uuid.Nil, nil)
	mustClean(t, res)
	if len(res.Warnings) != 0 {
		t.Errorf("the old slug header was reported: %v", res.Warnings)
	}
	if p, ok := r.product("OLD"); !ok || p.URLAuto || p.URL != "legacy-"+r.sfx {
		t.Errorf("an old file's slug column didn't land as the url path: %+v", p)
	}
}

func csvLine(cells []string) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(cells)
	w.Flush()
	return strings.TrimRight(b.String(), "\n")
}

func TestAChannelExportLeavesOtherChannelsProductsOut(t *testing.T) {
	r := newIORig(t)
	a, b := r.channel("USD"), r.channel("USD")
	mustClean(t, r.importFile(resourceProducts, r.file("sku,name,product_type,price,currency,published", "ON-A-{s},A,{pt},100,USD,true"), false, a, nil))
	mustClean(t, r.importFile(resourceProducts, r.file("sku,name,product_type,price,currency,published", "ON-B-{s},B,{pt},100,USD,true"), false, b, nil))
	seen := map[string]bool{}
	for _, row := range r.export(a) {
		seen[row[colSKU]] = true
	}
	if !seen[r.sku("ON-A")] || seen[r.sku("ON-B")] {
		t.Fatalf("channel A export: %v, want ON-A and not ON-B", seen)
	}
}

func TestARowThatFailsAfterItsCreateLeavesNoProduct(t *testing.T) {
	r := newIORig(t)
	usd := r.channel("USD")
	res := r.importFile(resourceProducts, r.file("sku,name,product_type,price,currency,published", "HALF-{s},Half,{pt},100,EUR,true"), false, usd, nil)
	if res.Failed != 1 {
		t.Fatalf("want the listing write to fail the row, got %+v", res)
	}
	if _, ok := r.product("HALF"); ok {
		t.Fatal("the failed row left its product behind")
	}
}

func TestPriceIsRequiredOnlyToCreate(t *testing.T) {
	r := newIORig(t)
	res := r.importFile(resourceProducts, r.file("sku,name,product_type,currency", "FREE-{s},Free,{pt},USD"), false, uuid.Nil, nil)
	if res.Failed != 1 || !strings.Contains(res.Errors[0].Message, "price is required") {
		t.Fatalf("a create without a price: %+v", res)
	}
	mustClean(t, r.importFile(resourceProducts, r.file("sku,name,product_type,price,currency", "PAID-{s},Paid,{pt},500,USD"), false, uuid.Nil, nil))
	mustClean(t, r.importFile(resourceProducts, r.file("sku,name,currency", "PAID-{s},Renamed,USD"), false, uuid.Nil, nil))
	if p, _ := r.product("PAID"); p.DefaultPrice != 500 {
		t.Fatalf("an update without a price changed it to %d", p.DefaultPrice)
	}
}

func TestAnExistingProductKeepsItsKind(t *testing.T) {
	r := newIORig(t)
	ctx := context.Background()
	attr, err := r.svc.CreateAttribute(ctx, catalog.CreateAttributeInput{
		Slug: "size-" + r.sfx, Name: "Size", InputType: catalog.InputTypeDropdown, Values: []catalog.AttributeValueInput{{Slug: "m", Name: "M"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AssignAttribute(ctx, catalog.AssignAttributeInput{ProductTypeID: r.pt.ID, AttributeID: attr.ID}); err != nil {
		t.Fatal(err)
	}
	mustClean(t, r.importFile(resourceProducts, r.file(
		"sku,kind,name,product_type,price,currency",
		"CONF-{s},configurable,Conf,{pt},100,USD",
		"SIMP-{s},simple,Simp,{pt},100,USD",
	), false, uuid.Nil, nil))

	res := r.importFile(resourceProducts, r.file("sku,kind,name,currency", "SIMP-{s},configurable,Simp,USD"), false, uuid.Nil, nil)
	if res.Failed != 1 || !strings.Contains(res.Errors[0].Message, "does not match") {
		t.Fatalf("a kind cell disagreeing with the product: %+v", res)
	}
	mustClean(t, r.importFile(resourceProducts, r.file("sku,name,currency,options", "CONF-{s},Conf,USD,size-{s}"), false, uuid.Nil, nil))
	p, _ := r.product("CONF")
	full, err := r.svc.GetProductWithAttributes(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.ConfigurableOptions) != 1 {
		t.Fatalf("options = %v, want the size axis set on the existing configurable", full.ConfigurableOptions)
	}
}

func TestACategoryTreeImportsInAnyRowOrder(t *testing.T) {
	r := newIORig(t)
	body := r.file(
		"slug,name,parent_slug",
		"leaf-{s},Leaf,mid-{s}",
		"mid-{s},Mid,root-{s}",
		"root-{s},Root,",
	)
	res := r.importFile(resourceCategories, body, true, uuid.Nil, nil)
	mustClean(t, res)
	if res.Created != 3 {
		t.Fatalf("dry run created %d, want 3", res.Created)
	}
	res = r.importFile(resourceCategories, body, false, uuid.Nil, nil)
	mustClean(t, res)
	if res.Created != 3 {
		t.Fatalf("real run created %d, want 3", res.Created)
	}
	ctx := context.Background()
	get := func(slug string) catalog.Category {
		c, err := r.svc.GetCategoryBySlug(ctx, slug+"-"+r.sfx)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	root, mid, leaf := get("root"), get("mid"), get("leaf")
	if mid.ParentID == nil || *mid.ParentID != root.ID || leaf.ParentID == nil || *leaf.ParentID != mid.ID {
		t.Fatal("the reverse-ordered tree didn't link up")
	}

	res = r.importFile(resourceCategories, r.file("slug,name,parent_slug", "orphan-{s},Orphan,nowhere-{s}"), false, uuid.Nil, nil)
	if res.Failed != 1 || !strings.Contains(res.Errors[0].Message, "not found") {
		t.Fatalf("a parent in neither the catalog nor the file: %+v", res)
	}
}

// A failed lookup is not a missing category - a dry run must not preview a create it can't vouch for
func TestACategoryLookupFailureIsNotAMissingCategory(t *testing.T) {
	r := newIORig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := dataio.NewOpts(nil)
	o.DryRun = true
	im := &categoryImporter{cat: r.svc, opts: o, locale: "en", bySlug: map[string]uuid.UUID{}}
	row := dataio.NewRow()
	row.Set(colCatSlug, "x-"+r.sfx)
	row.Set(colCatName, "X")
	if out, err := im.Row(ctx, row); err == nil || out == dataio.OutcomeCreated {
		t.Fatalf("outcome %v, err %v: a lookup failure previewed as a create", out, err)
	}
	child := dataio.NewRow()
	child.Set(colCatSlug, "y-"+r.sfx)
	child.Set(colCatParent, "p-"+r.sfx)
	if _, err := im.Row(ctx, child); err == nil || errors.Is(err, catalog.ErrCategoryNotFound) || !errors.Is(err, context.Canceled) {
		t.Fatalf("parent lookup failure surfaced as %v", err)
	}
	if _, err := newResolver(r.svc, false).categoryBySlug(ctx, "z-"+r.sfx); err == nil || strings.Contains(err.Error(), "not found") {
		t.Fatalf("resolver lookup failure surfaced as %v", err)
	}
}
