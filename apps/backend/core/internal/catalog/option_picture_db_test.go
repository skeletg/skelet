package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// What these pin: a picture has to be a finished upload, an update that doesn't name it keeps it, a zero id clears it,
// a copy keeps its pictures, and deleting the asset clears it everywhere

func (r structureRig) asset(status string) uuid.UUID {
	r.t.Helper()
	id := uuid.New()
	if _, err := r.conn.Exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status) VALUES ($1, 'a.png', $2, 'image/png', 1, $3)`,
		id, "test/"+id.String(), status); err != nil {
		r.t.Fatalf("asset: %v", err)
	}
	return id
}

func TestOptionPicturesSetClearAndTravelWithCopies(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	ready, pending := r.asset("ready"), r.asset("pending")
	p := r.product(KindSimple, "mug")

	_, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputRadio, Locale: "en", Title: "Glaze", ImageAssetID: &pending})
	wantErr(t, "a picture still uploading", err, ErrOptionImageInvalid)
	glaze, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputRadio, Locale: "en", Title: "Glaze", ImageAssetID: &ready})
	if err != nil {
		t.Fatal(err)
	}
	blue, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: glaze.ID, Locale: "en", Title: "Blue", PriceType: OptionPriceFixed, Quantity: 1, ImageAssetID: &ready})
	if err != nil {
		t.Fatal(err)
	}
	if glaze.ImageAssetID == nil || *glaze.ImageAssetID != ready || blue.ImageAssetID == nil || *blue.ImageAssetID != ready {
		t.Fatalf("pictures = %v / %v, want both set", glaze.ImageAssetID, blue.ImageAssetID)
	}

	copied, err := r.svc.DuplicateProductOption(ctx, glaze.ID)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := r.svc.GetProductOptions(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range opts {
		if o.ID == copied.ID && (o.ImageAssetID == nil || len(o.Values) != 1 || o.Values[0].ImageAssetID == nil) {
			t.Fatalf("the copy lost its pictures: %+v", o)
		}
	}

	none := uuid.UUID{}
	cleared, err := r.svc.UpdateProductOptionValue(ctx, UpdateProductOptionValueInput{ID: blue.ID, ImageAssetID: &none})
	if err != nil || cleared.ImageAssetID != nil {
		t.Fatalf("cleared choice picture = %v %v, want none", cleared.ImageAssetID, err)
	}
	kept, err := r.svc.UpdateProductOption(ctx, UpdateProductOptionInput{ID: glaze.ID, IsRequired: new(bool)})
	if err != nil || kept.ImageAssetID == nil {
		t.Fatalf("an update that doesn't name the picture dropped it: %v %v", kept.ImageAssetID, err)
	}
	if _, err := r.conn.Exec(`DELETE FROM media_assets WHERE id = $1`, ready); err != nil {
		t.Fatal(err)
	}
	gone, err := r.svc.GetProductOption(ctx, glaze.ID)
	if err != nil || gone.ImageAssetID != nil {
		t.Fatalf("after the asset went the option still points at %v (%v)", gone.ImageAssetID, err)
	}
}

// A customer upload swept as an orphan must not take a picture an option uses with it
func TestOrphanSweepSparesOptionPictures(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	upload := func() uuid.UUID {
		id := uuid.New()
		if _, err := r.conn.Exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status, created_at)
			VALUES ($1, 'a.png', $2, 'image/png', 1, 'ready', now() - interval '2 days')`, id, "product-options/sweep/"+id.String()+".png"); err != nil {
			t.Fatalf("upload: %v", err)
		}
		return id
	}
	used, loose := upload(), upload()
	p := r.product(KindSimple, "sweep")
	if _, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputField, Locale: "en", Title: "Note", ImageAssetID: &used}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.New(r.conn).ListOrphanProductOptionAssets(ctx, db.ListOrphanProductOptionAssetsParams{Cutoff: time.Now().Add(-time.Hour), MaxRows: 1000})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[uuid.UUID]bool{}
	for _, row := range rows {
		listed[row.ID] = true
	}
	if listed[used] || !listed[loose] {
		t.Fatalf("swept used=%v loose=%v, want only the loose upload", listed[used], listed[loose])
	}
}
