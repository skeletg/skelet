package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type signingStore struct{ objstore.Storage }

func (signingStore) MaxUploadBytes() int64 { return 1 << 20 }
func (signingStore) PublicURL(key string) string {
	return "https://cdn.test/" + key
}
func (signingStore) PresignUpload(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
	return "https://upload.test/" + key, nil
}

type savedPaths map[string]string

func (s savedPaths) GetBool(context.Context, string, config.Scope) (bool, error) { return true, nil }
func (s savedPaths) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return s[path], nil
}

func TestCatalogUploadsLandWhereTheSavedPathsSay(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn), nil)
	svc := NewService(Deps{Repo: repo, Media: signingStore{}})
	suffix := uuid.NewString()[:8]

	var typeID, productID uuid.UUID
	mustScan(t, conn, &typeID, `INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'keys') RETURNING id`, "key-"+suffix)
	mustScan(t, conn, &productID, `INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "KEY "+suffix)
	category, err := repo.CreateCategory(ctx, CreateCategoryInput{Slug: "key-cat-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	collection, err := repo.CreateCollection(ctx, CreateCollectionInput{Slug: "key-col-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, category.ID)
		_, _ = conn.Exec(`DELETE FROM catalog_collections WHERE id = $1`, collection.ID)
	})

	main := MediaRoleMain
	upload := func() (product, categoryCover, collectionCover string) {
		t.Helper()
		m, err := svc.RequestMediaUpload(ctx, RequestMediaUploadInput{ProductID: productID, Filename: "Front View.JPG", MimeType: "image/jpeg", SizeBytes: 10, Role: &main})
		if err != nil {
			t.Fatal(err)
		}
		c, err := svc.RequestCategoryCoverUpload(ctx, RequestCategoryCoverInput{CategoryID: category.ID, Filename: "Hero.png", MimeType: "image/png", SizeBytes: 10})
		if err != nil {
			t.Fatal(err)
		}
		l, err := svc.RequestCollectionCoverUpload(ctx, RequestCollectionCoverInput{CollectionID: collection.ID, Filename: "Hero.png", MimeType: "image/png", SizeBytes: 10})
		if err != nil {
			t.Fatal(err)
		}
		if m.UploadURL != "https://upload.test/"+m.Media.S3Key || c.UploadURL != "https://upload.test/"+c.Category.CoverImage.S3Key {
			t.Fatalf("the upload was signed for another key than the one stored: %s, %s", m.UploadURL, c.UploadURL)
		}
		return m.Media.S3Key, c.Category.CoverImage.S3Key, l.Collection.CoverImage.S3Key
	}

	product, categoryCover, collectionCover := upload()
	for key, shape := range map[string]string{
		product:         "catalog/products/" + productID.String() + "/main/",
		categoryCover:   "catalog/categories/" + category.ID.String() + "/cover/",
		collectionCover: "catalog/collections/" + collection.ID.String() + "/cover/",
	} {
		if id, ok := strings.CutPrefix(key, shape); !ok || uuid.Validate(strings.TrimSuffix(strings.TrimSuffix(id, ".jpg"), ".png")) != nil {
			t.Errorf("with nothing saved the key is %q, want %q then the file's id and extension", key, shape)
		}
	}

	svc.SetConfig(savedPaths{
		media.ProductImageKey.Path:    "p/{sku}/{type}-{name}-{short_id}{ext}",
		media.CategoryCoverKey.Path:   "covers/{slug}-{short_id}{ext}",
		media.CollectionCoverKey.Path: "covers/{yyyy}/{slug}-{name}-{short_id}{ext}",
	})
	product, categoryCover, collectionCover = upload()
	year := time.Now().UTC().Format("2006")
	for key, want := range map[string][2]string{
		product:         {"catalog/p/key-" + suffix + "/main-front-view-", ".jpg"},
		categoryCover:   {"catalog/covers/key-cat-" + suffix + "-", ".png"},
		collectionCover: {"catalog/covers/" + year + "/key-col-" + suffix + "-hero-", ".png"},
	} {
		short, ok := strings.CutPrefix(strings.TrimSuffix(key, want[1]), want[0])
		if !ok || len(short) != 8 || !strings.HasSuffix(key, want[1]) {
			t.Errorf("with paths saved the key is %q, want %q, eight characters of id, %q", key, want[0], want[1])
		}
	}
}
