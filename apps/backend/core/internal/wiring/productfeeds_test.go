package wiring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestProductFeedDownloadRequiresAnActiveConfiguredAccountAndSnapshot(t *testing.T) {
	ctx := context.Background()
	conn := testdb.Require(t)
	id := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels(id,slug,name,currency_code) VALUES($1,$2,'Feed shop','USD')`, id, "feed-"+id.String()); err != nil {
		t.Fatal(err)
	}
	kind := "test-" + uuid.NewString()
	reg := sdk.RegisteredProductFeed{PluginSlug: "test-feed", Feed: sdk.ProductFeed{Kind: kind, Extension: "jsonl", ContentType: "application/x-ndjson"}}
	tracking.RegisterKind(tracking.Kind{Kind: tracking.CatalogKind(kind), Catalog: true})
	svc := tracking.NewService(db.New(conn), nil, nil)
	dest, err := svc.Create(ctx, tracking.Destination{Kind: tracking.CatalogKind(kind), ChannelID: id, Name: "Account", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := objstore.New(ctx, objstore.Config{Driver: objstore.DriverFilesystem, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	exporter := catalogio.NewProductFeedExporter(nil, storage, nil, nil)
	h := productFeedHandler(reg, svc, exporter)
	ch := channel.Channel{ID: id, IsActive: true, PublicURL: "https://shop.example"}
	request := func(method, query string, ch *channel.Channel) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/feeds/"+kind+".jsonl"+query, nil)
		if ch != nil {
			r = r.WithContext(channel.WithChannel(r.Context(), ch))
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	if w := request(http.MethodGet, "", &ch); w.Code != 404 {
		t.Fatalf("missing snapshot: %d", w.Code)
	}
	key := "feeds/" + kind + "/" + id.String() + "/" + dest.ID.String() + ".jsonl"
	if err := storage.PutBytes(ctx, key, []byte("snapshot"), reg.Feed.ContentType); err != nil {
		t.Fatal(err)
	}
	if w := request(http.MethodGet, "", &ch); w.Code != 200 || w.Body.String() != "snapshot" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("download: %d, %s", w.Code, w.Body)
	}
	if w := request(http.MethodHead, "", &ch); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d", w.Code)
	}
	if w := request(http.MethodGet, "?destination="+uuid.Nil.String(), &ch); w.Code != 404 {
		t.Errorf("zero account ID selected a default account: %d", w.Code)
	}
	if w := request(http.MethodGet, "?destination=invalid", &ch); w.Code != 400 {
		t.Errorf("invalid account ID: %d", w.Code)
	}
	for _, c := range []*channel.Channel{nil, {ID: uuid.New(), IsActive: true, PublicURL: ch.PublicURL}, {ID: id, IsActive: false, PublicURL: ch.PublicURL}} {
		if w := request(http.MethodGet, "", c); w.Code != 404 {
			t.Errorf("wrong channel returned %d", w.Code)
		}
	}
	second, err := svc.Create(ctx, tracking.Destination{Kind: tracking.CatalogKind(kind), ChannelID: id, Name: "Second account", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(http.MethodGet, "", &ch); w.Code != 400 {
		t.Errorf("ambiguous account: %d", w.Code)
	}
	if w := request(http.MethodGet, "?destination="+dest.ID.String(), &ch); w.Code != 200 {
		t.Errorf("explicit account: %d", w.Code)
	}
	dest.Enabled = false
	if _, err := svc.Update(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if w := request(http.MethodGet, "?destination="+dest.ID.String(), &ch); w.Code != 404 {
		t.Errorf("disabled account: %d", w.Code)
	}
	if w := request(http.MethodGet, "?destination="+second.ID.String(), &ch); w.Code != 404 {
		t.Errorf("account borrowed another snapshot: %d", w.Code)
	}
}
