package medialibrary

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
)

func picture(key, mime string, width int32) Asset {
	return Asset{ObjectKey: key, MimeType: mime, Width: &width, Status: AssetStatusReady}
}

func TestAThumbnailIsTheStoredCopyOnceThereIsOne(t *testing.T) {
	svc, st, _ := fixture()
	ctx := context.Background()
	a := picture("banners/summer sale.jpg", "image/jpeg", 3000)

	if got, want := svc.ThumbnailURL(ctx, a, 600), "/api/media/t/w640/banners/summer%20sale.jpg"; got != want {
		t.Fatalf("with no copy yet the thumbnail is %q, want the path that makes one, %q", got, want)
	}
	copyKey := media.DerivedKey(a.ObjectKey, 640, 0, media.CanEncodeWebP)
	st.objects[copyKey] = true
	for range 3 {
		if got, want := svc.ThumbnailURL(ctx, a, 600), "https://cdn.test/"+copyKey; got != want {
			t.Fatalf("with the copy stored the thumbnail is %q, want %q", got, want)
		}
	}
	if st.heads != 2 {
		t.Errorf("storage was asked %d times, want once while the copy was missing and once to find it", st.heads)
	}
}

func TestTheOriginalStandsInWhereACopyWouldGainNothing(t *testing.T) {
	ctx := context.Background()
	pending := picture("a.jpg", "image/jpeg", 3000)
	pending.Status = AssetStatusPending
	unmeasured := picture("b.jpg", "image/jpeg", 0)
	unmeasured.Width = nil
	for name, a := range map[string]Asset{
		"a document":                   picture("terms.pdf", "application/pdf", 0),
		"a vector":                     picture("logo.svg", MimeSVG, 3000),
		"a type only one build reads":  picture("phone.heic", "image/heic", 3000),
		"a picture already that small": picture("icon.png", "image/png", 640),
		"an upload not yet confirmed":  pending,
		"a picture of unknown size":    unmeasured,
	} {
		svc, st, _ := fixture()
		if got := svc.ThumbnailURL(ctx, a, 640); got != "https://cdn.test/"+a.ObjectKey || st.heads != 0 {
			t.Errorf("%s: thumbnail %q after %d storage reads, want its own address and none", name, got, st.heads)
		}
	}

	svc, st, _ := fixture()
	st.private = true
	if got := svc.ThumbnailURL(ctx, picture("a.jpg", "image/jpeg", 3000), 640); strings.Contains(got, "/media/t/") {
		t.Errorf("with no public address for storage every view would go through the resizer: %q", got)
	}
}

func TestTheMemoStartsOverWhenFull(t *testing.T) {
	var m thumbMemo
	for i := range thumbMemoMax {
		m.add("copy-" + strconv.Itoa(i))
	}
	if !m.has("copy-0") {
		t.Fatal("a copy seen before the memo filled was forgotten early")
	}
	m.add("one too many")
	if m.has("copy-0") || m.has("one too many") {
		t.Error("the memo kept growing past its bound")
	}
	m.add("after")
	if !m.has("after") {
		t.Error("the memo stopped remembering after starting over")
	}
}

func TestUploadTypesCoverEverythingTheLibraryTakes(t *testing.T) {
	types := UploadTypes()
	for _, mime := range types {
		if mime != MimeSVG && !isAllowedMime(mime) {
			t.Errorf("%s is offered to a file picker but refused on upload", mime)
		}
	}
	for _, want := range []string{"image/jpeg", MimeSVG, "video/mp4", "application/pdf"} {
		if !strings.Contains(strings.Join(types, " "), want) {
			t.Errorf("%s is accepted on upload but not offered", want)
		}
	}
}

func TestAFinishedFileOpensFromItsOwnStore(t *testing.T) {
	svc, st, repo := fixture()
	vault := newMemStore()
	svc.SetPrivateStore(vault)
	ctx := context.Background()
	ready := func(dir, file string, in Visibility) Asset {
		a := seedAsset(repo, st, dir, file)
		a.Status, a.Visibility = AssetStatusReady, in
		repo.assets[a.ID] = a
		return a
	}
	photo, contract := ready("banners", "a.jpg", Public), ready("contracts", "acme.pdf", Private)
	delete(st.objects, contract.ObjectKey)
	vault.objects[contract.ObjectKey] = true

	for _, a := range []Asset{photo, contract} {
		body, got, err := svc.Open(ctx, a.ID)
		if err != nil {
			t.Fatalf("%s: %v", a.Filename, err)
		}
		if raw, _ := io.ReadAll(body); string(raw) != "bytes of "+a.ObjectKey || got.ID != a.ID {
			t.Errorf("%s opened as %q", a.Filename, raw)
		}
	}

	pending := seedAsset(repo, st, "banners", "b.jpg")
	for name, id := range map[string]uuid.UUID{"an unconfirmed upload": pending.ID, "an unknown id": uuid.New()} {
		if _, _, err := svc.Open(ctx, id); !errors.Is(err, ErrAssetNotFound) {
			t.Errorf("%s: %v, want ErrAssetNotFound", name, err)
		}
	}
}
