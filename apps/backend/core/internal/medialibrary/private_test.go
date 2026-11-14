package medialibrary

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func twoHalves(publicKeys ...string) (*Service, *memStore, *memStore, *memRepo) {
	svc, open, repo := fixture(publicKeys...)
	vault := newMemStore()
	svc.SetPrivateStore(vault)
	return svc, open, vault, repo
}

func seedIn(repo *memRepo, st *memStore, in Visibility, dir, file string) Asset {
	a := seedAsset(repo, st, dir, file)
	a.Visibility, a.Status = in, AssetStatusReady
	repo.assets[a.ID] = a
	return a
}

func TestWithNoPrivateStoreThereIsNoPrivateHalf(t *testing.T) {
	svc, st, repo := fixture()
	svc.SetPrivateStore(st) // the media store standing in for a missing private one is not a private half
	if svc.HasPrivate() {
		t.Fatal("the public store was taken for a private one - files marked private would sit where anyone can fetch them")
	}
	ctx := context.Background()
	a := seedIn(repo, st, Public, "banners", "a.jpg")
	private := Private
	for name, err := range map[string]error{
		"browse":     func() error { _, err := svc.GetDirectory(ctx, "", Private); return err }(),
		"search":     func() error { _, err := svc.SearchAssets(ctx, SearchInput{Visibility: Private}); return err }(),
		"folders":    func() error { _, err := svc.ListAllDirectories(ctx, Private); return err }(),
		"new folder": func() error { _, err := svc.CreateDirectory(ctx, "", "contracts", Private); return err }(),
		"upload": func() error {
			_, err := svc.RequestUpload(ctx, RequestUploadInput{Visibility: Private, Filename: "a.pdf", MimeType: "application/pdf", SizeBytes: 1})
			return err
		}(),
		"make private": func() error {
			_, err := svc.MoveAsset(ctx, MoveAssetInput{ID: a.ID, TargetPath: "banners", Visibility: &private})
			return err
		}(),
	} {
		if !errors.Is(err, ErrNoPrivateStore) {
			t.Errorf("%s: %v, want ErrNoPrivateStore", name, err)
		}
	}
	if !st.objects[a.ObjectKey] {
		t.Error("a refused move still took the file")
	}
}

func TestAPrivateUploadIsSignedForThePrivateStoreAndHasNoAddress(t *testing.T) {
	svc, open, vault, _ := twoHalves()
	res, err := svc.RequestUpload(context.Background(), RequestUploadInput{Path: "contracts", Visibility: Private, Filename: "Acme.pdf", MimeType: "application/pdf", SizeBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(vault.signed) != 1 || len(open.signed) != 0 || vault.signed[0] != res.Asset.ObjectKey {
		t.Fatalf("signed by the private store %v and the public one %v, want only the private", vault.signed, open.signed)
	}
	if res.Asset.Visibility != Private || res.Asset.URL != "" {
		t.Errorf("the asset came back %s at %q, want private with no address", res.Asset.Visibility, res.Asset.URL)
	}
	if got := svc.ThumbnailURL(context.Background(), picture(res.Asset.ObjectKey, "image/jpeg", 3000).in(Private), 640); got != "" {
		t.Errorf("a private picture was given a thumbnail address: %q", got)
	}
}

func (a Asset) in(v Visibility) Asset { a.Visibility = v; return a }

func TestThePlainGettersNeverReturnAPrivateFile(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	banner := seedIn(repo, open, Public, "banners", "a.jpg")
	deed := seedIn(repo, open, Private, "contracts", "deed.pdf")
	delete(open.objects, deed.ObjectKey)
	vault.objects[deed.ObjectKey] = true

	if _, err := svc.GetAsset(ctx, deed.ID); !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("GetAsset on a private file: %v, want ErrAssetNotFound", err)
	}
	if got, err := svc.GetAsset(ctx, banner.ID); err != nil || got.URL == "" {
		t.Errorf("GetAsset on a public file: %+v, %v", got, err)
	}
	byID, err := svc.GetAssetsByIDs(ctx, []uuid.UUID{banner.ID, deed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := byID[deed.ID]; leaked || len(byID) != 1 {
		t.Errorf("GetAssetsByIDs returned %d files, the private one among them: %v", len(byID), leaked)
	}
	got, err := svc.GetAnyAsset(ctx, deed.ID)
	if err != nil || got.Visibility != Private || got.URL != "" {
		t.Errorf("GetAnyAsset: %s at %q, %v - want the private file, with no address", got.Visibility, got.URL, err)
	}
}

func TestMakingAFilePrivateCarriesItAcrossAndTakesItsCopiesDown(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	a := seedIn(repo, open, Public, "banners", "a.jpg")
	other := seedIn(repo, open, Public, "banners", "b.jpg")
	copies := []string{"derived/w640-webp/" + a.ObjectKey, "derived/w320/" + a.ObjectKey}
	kept := "derived/w640-webp/" + other.ObjectKey
	for _, k := range append(copies, kept) {
		open.objects[k] = true
	}

	private := Private
	moved, err := svc.MoveAsset(ctx, MoveAssetInput{ID: a.ID, TargetPath: "contracts", Visibility: &private})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Visibility != Private || moved.URL != "" || moved.DirectoryPrefix != "contracts" {
		t.Fatalf("after the move: %s at %q in %q", moved.Visibility, moved.URL, moved.DirectoryPrefix)
	}
	if vault.written[moved.ObjectKey] != "bytes of "+a.ObjectKey {
		t.Errorf("the private store holds %q, want the file's own bytes", vault.written[moved.ObjectKey])
	}
	if open.objects[a.ObjectKey] {
		t.Error("the original is still in the public store")
	}
	for _, k := range copies {
		if open.objects[k] {
			t.Errorf("a resized copy is still public: %s", k)
		}
	}
	if !open.objects[kept] || !open.objects[other.ObjectKey] {
		t.Error("another picture, or its resized copy, went too")
	}

	public := Public
	back, err := svc.MoveAsset(ctx, MoveAssetInput{ID: a.ID, From: Private, TargetPath: "banners", Visibility: &public})
	if err != nil {
		t.Fatal(err)
	}
	if back.Visibility != Public || !strings.HasPrefix(back.URL, "https://cdn.test/banners/") || !open.objects[back.ObjectKey] || vault.objects[moved.ObjectKey] {
		t.Fatalf("made public again: %s at %q, public store has it %v, private still has it %v", back.Visibility, back.URL, open.objects[back.ObjectKey], vault.objects[moved.ObjectKey])
	}
}

func TestAFailedCrossingLeavesNoCopyInTheOtherStore(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	deed := seedIn(repo, open, Private, "contracts", "deed.pdf")
	delete(open.objects, deed.ObjectKey)
	vault.objects[deed.ObjectKey] = true
	banner := seedIn(repo, open, Public, "banners", "a.jpg")
	repo.failMove = true

	public, private := Public, Private
	if _, err := svc.MoveAsset(ctx, MoveAssetInput{ID: deed.ID, From: Private, TargetPath: "contracts", Visibility: &public}); err == nil {
		t.Fatal("making it public reported success")
	}
	if _, err := svc.MoveAsset(ctx, MoveAssetInput{ID: banner.ID, TargetPath: "banners", Visibility: &private}); err == nil {
		t.Fatal("making it private reported success")
	}
	for key := range open.objects {
		if strings.Contains(key, "deed") {
			t.Errorf("the private file was left in the public store at %s", key)
		}
	}
	for key := range vault.objects {
		if strings.Contains(key, "a.jpg") {
			t.Errorf("a copy of the public file was left in the private store at %s", key)
		}
	}
	if !vault.objects[deed.ObjectKey] || !open.objects[banner.ObjectKey] {
		t.Error("a file that could not be moved is gone from where it was")
	}
	if repo.assets[deed.ID].Visibility != Private || repo.assets[banner.ID].Visibility != Public {
		t.Error("a failed move changed which half a file is in")
	}
}

func TestDeletingAPublicFileTakesItsResizedCopiesWithIt(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	photo := seedIn(repo, open, Public, "banners", "a.jpg")
	contract := seedIn(repo, open, Private, "banners", "a.pdf")
	delete(open.objects, contract.ObjectKey)
	vault.objects[contract.ObjectKey] = true
	copyOf := "derived/w640-webp/" + photo.ObjectKey
	sameNameCopy := "derived/w640-webp/" + contract.ObjectKey
	open.objects[copyOf], open.objects[sameNameCopy] = true, true

	if err := svc.DeleteAsset(ctx, contract.ID, Private, true); err != nil {
		t.Fatal(err)
	}
	if vault.objects[contract.ObjectKey] || !open.objects[sameNameCopy] {
		t.Errorf("deleting the private file: still in the private store %v, the public store's objects touched %v", vault.objects[contract.ObjectKey], !open.objects[sameNameCopy])
	}
	if err := svc.DeleteAsset(ctx, photo.ID, Public, true); err != nil {
		t.Fatal(err)
	}
	if open.objects[photo.ObjectKey] || open.objects[copyOf] {
		t.Errorf("deleting the public file left the original %v or its resized copy %v", open.objects[photo.ObjectKey], open.objects[copyOf])
	}
}

func TestAPrivateFilesResizedCopiesGoWithIt(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	place := func(file string) (Asset, string) {
		a := seedIn(repo, open, Private, "ids", file)
		delete(open.objects, a.ObjectKey)
		small := "derived/w640-webp/" + a.ObjectKey
		vault.objects[a.ObjectKey], vault.objects[small] = true, true
		return a, small
	}
	gone, goneCopy := place("passport.jpg")
	crossing, crossingCopy := place("licence.jpg")
	_, keptCopy := place("permit.jpg")

	if err := svc.DeleteAsset(ctx, gone.ID, Private, true); err != nil {
		t.Fatal(err)
	}
	if vault.objects[gone.ObjectKey] || vault.objects[goneCopy] {
		t.Errorf("deleted, yet the private store still holds the file %v or its resized copy %v", vault.objects[gone.ObjectKey], vault.objects[goneCopy])
	}
	public := Public
	if _, err := svc.MoveAsset(ctx, MoveAssetInput{ID: crossing.ID, From: Private, TargetPath: "ids", Visibility: &public}); err != nil {
		t.Fatal(err)
	}
	if vault.objects[crossing.ObjectKey] || vault.objects[crossingCopy] {
		t.Errorf("made public, yet the private store still holds the file %v or its resized copy %v", vault.objects[crossing.ObjectKey], vault.objects[crossingCopy])
	}
	if !vault.objects[keptCopy] {
		t.Error("another private picture's resized copy went too")
	}
}

func TestFoldersBelongToOneHalf(t *testing.T) {
	svc, open, vault, repo := twoHalves("contracts/readme.txt")
	ctx := context.Background()
	if _, err := svc.CreateDirectory(ctx, "", "contracts", Private); err != nil {
		t.Fatal(err)
	}
	if !vault.objects["contracts/.keep"] || open.objects["contracts/.keep"] {
		t.Fatalf("the private folder's marker is in the private store %v and the public one %v", vault.objects["contracts/.keep"], open.objects["contracts/.keep"])
	}
	secret := seedIn(repo, vault, Private, "contracts", "acme.pdf")
	listed := seedIn(repo, open, Public, "contracts", "terms.pdf")

	renamed, err := svc.RenameDirectory(ctx, "contracts", "signed", Private)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Path != "signed" || !vault.objects["signed/acme.pdf"] || vault.objects[secret.ObjectKey] {
		t.Fatalf("the private folder wasn't renamed in its store: %v", vault.objects)
	}
	if repo.assets[secret.ID].ObjectKey != "signed/acme.pdf" || repo.assets[secret.ID].Visibility != Private {
		t.Errorf("the private file's row: %+v", repo.assets[secret.ID])
	}
	if !open.objects[listed.ObjectKey] || repo.assets[listed.ID].ObjectKey != listed.ObjectKey {
		t.Error("renaming the private folder moved the public folder of the same name")
	}

	if err := svc.DeleteDirectory(ctx, "signed", true, Private); err != nil {
		t.Fatal(err)
	}
	if vault.objects["signed/acme.pdf"] || !open.objects[listed.ObjectKey] || !open.objects["contracts/readme.txt"] {
		t.Errorf("deleting the private folder: private file left %v, public files kept %v", vault.objects["signed/acme.pdf"], open.objects[listed.ObjectKey])
	}
	if !slices.Contains(repo.deletedBy, "private:signed/") {
		t.Errorf("rows deleted by %v, want only the private half's under signed/", repo.deletedBy)
	}
}

func TestThePrivateHalfShowsOnlyTheLibrarysOwnFolders(t *testing.T) {
	svc, _, vault, _ := twoHalves()
	for _, k := range []string{"data/exports/j1/customers.csv", "webforms/f1/cv/u-cv.pdf", ".private-probe/abc", "contracts/.keep", "contracts/stray.pdf"} {
		vault.objects[k] = true
	}
	ctx := context.Background()
	root, err := svc.GetDirectory(ctx, "", Private)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Directories) != 1 || root.Directories[0].Path != "contracts" {
		t.Fatalf("the private root lists %v, want only contracts", root.Directories)
	}
	all, err := svc.ListAllDirectories(ctx, Private)
	if err != nil || len(all) != 1 || all[0].Path != "contracts" {
		t.Fatalf("every private folder: %v %v, want only contracts", all, err)
	}
	dir, err := svc.GetDirectory(ctx, "contracts", Private)
	if err != nil {
		t.Fatal(err)
	}
	if len(dir.UntrackedFiles) != 1 || dir.UntrackedFiles[0].URL != "" {
		t.Errorf("a file found in the private store was given an address: %+v", dir.UntrackedFiles)
	}
	if _, err := svc.GetDirectory(ctx, "data/exports", Private); !errors.Is(err, ErrReservedPath) {
		t.Errorf("the exports were browsable through the private half: %v", err)
	}
}

func TestThePrivateHalfCannotTouchWhatElseLivesInItsStore(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	theirs := []string{"data/exports/j1/customers.csv", "webforms/f1/cv/u-cv.pdf", "derived/w640-webp/ids/a.jpg", ".private-probe/abc"}
	for _, k := range theirs {
		vault.objects[k] = true
	}
	ctx := context.Background()
	mine := seedIn(repo, open, Private, "contracts", "deed.pdf")
	delete(open.objects, mine.ObjectKey)
	vault.objects[mine.ObjectKey] = true
	private := Private

	for _, p := range []string{"data", "data/exports", "webforms", "webforms/f1/cv", "derived", "derived/w640-webp"} {
		parent, name, nested := strings.CutLast(p, "/")
		if !nested {
			parent, name = "", p
		}
		refused := map[string]error{
			"browse":     func() error { _, err := svc.GetDirectory(ctx, p, Private); return err }(),
			"new folder": func() error { _, err := svc.CreateDirectory(ctx, parent, name, Private); return err }(),
			"folder in":  func() error { _, err := svc.CreateDirectory(ctx, p, "mine", Private); return err }(),
			"upload": func() error {
				_, err := svc.RequestUpload(ctx, RequestUploadInput{Path: p, Visibility: Private, Filename: "a.pdf", MimeType: "application/pdf", SizeBytes: 1})
				return err
			}(),
			"svg": func() error {
				_, err := svc.UploadSVG(ctx, UploadSVGInput{Path: p, Visibility: Private, Filename: "a.svg", Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)})
				return err
			}(),
			"move in": func() error {
				_, err := svc.MoveAsset(ctx, MoveAssetInput{ID: mine.ID, From: Private, TargetPath: p, Visibility: &private})
				return err
			}(),
			"rename": func() error { _, err := svc.RenameDirectory(ctx, p, "mine", Private); return err }(),
			"delete": func() error { return svc.DeleteDirectory(ctx, p, true, Private) }(),
		}
		if parent == "" {
			_, refused["rename onto"] = svc.RenameDirectory(ctx, "contracts", name, Private)
		}
		for what, err := range refused {
			if !errors.Is(err, ErrReservedPath) {
				t.Errorf("%s %q: %v, want ErrReservedPath", what, p, err)
			}
		}
	}
	if dir, err := svc.GetDirectory(ctx, ".private-probe", Private); err != nil || len(dir.UntrackedFiles) != 0 {
		t.Errorf("the boot check's folder was browsable: %+v %v", dir.UntrackedFiles, err)
	}
	if err := svc.DeleteDirectory(ctx, ".private-probe", true, Private); err != nil {
		t.Fatal(err)
	}
	for _, k := range append(theirs, mine.ObjectKey) {
		if !vault.objects[k] {
			t.Errorf("%s is gone from the private store", k)
		}
	}
	if len(vault.signed) != 0 {
		t.Errorf("uploads were signed into paths the library doesn't own: %v", vault.signed)
	}
}

func TestACrossingThatCannotRemoveThePublicCopyIsUndone(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	a := seedIn(repo, open, Public, "banners", "a.jpg")
	open.failDelete[a.ObjectKey] = true
	private := Private
	if _, err := svc.MoveAsset(context.Background(), MoveAssetInput{ID: a.ID, TargetPath: "banners", Visibility: &private}); !errors.Is(err, ErrStillPublic) {
		t.Fatalf("got %v, want ErrStillPublic", err)
	}
	if got := repo.assets[a.ID]; got.Visibility != Public || got.ObjectKey != a.ObjectKey {
		t.Errorf("the row was left as %s at %s, want it public where it was", got.Visibility, got.ObjectKey)
	}
	if !open.objects[a.ObjectKey] || len(vault.objects) != 0 {
		t.Errorf("public original kept %v, private store now holds %v - want the original only", open.objects[a.ObjectKey], vault.objects)
	}
}

func TestACancelledRequestStillRemovesThePublicCopy(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	a := seedIn(repo, open, Public, "banners", "a.jpg")
	small := "derived/w640-webp/" + a.ObjectKey
	open.objects[small] = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	open.cancelled = true
	private := Private
	moved, err := svc.MoveAsset(ctx, MoveAssetInput{ID: a.ID, TargetPath: "banners", Visibility: &private})
	if err != nil {
		t.Fatal(err)
	}
	if open.objects[a.ObjectKey] || open.objects[small] || !vault.objects[moved.ObjectKey] {
		t.Errorf("after a cancelled request: public original %v, its copy %v, private copy %v", open.objects[a.ObjectKey], open.objects[small], vault.objects[moved.ObjectKey])
	}
}

func TestAWriterOnlyFindsTheFileInTheHalfItWasToldOf(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	deed := seedIn(repo, open, Private, "contracts", "deed.pdf")
	delete(open.objects, deed.ObjectKey)
	vault.objects[deed.ObjectKey] = true
	pending := seedIn(repo, open, Private, "contracts", "upload.pdf")
	pending.Status = AssetStatusPending
	repo.assets[pending.ID] = pending
	public := Public

	x, title := 0.5, "Deed"
	for name, err := range map[string]error{
		"delete":      svc.DeleteAsset(ctx, deed.ID, Public, true),
		"focal point": func() error { _, err := svc.SetFocalPoint(ctx, deed.ID, Public, &x, &x); return err }(),
		"words":       func() error { _, err := svc.UpdateAsset(ctx, UpdateAssetInput{ID: deed.ID, Title: &title}); return err }(),
		"move": func() error {
			_, err := svc.MoveAsset(ctx, MoveAssetInput{ID: deed.ID, TargetPath: "contracts", Visibility: &public})
			return err
		}(),
		"confirm": func() error { _, err := svc.ConfirmUpload(ctx, ConfirmUploadInput{AssetID: pending.ID}); return err }(),
	} {
		if !errors.Is(err, ErrAssetNotFound) {
			t.Errorf("%s: %v, want ErrAssetNotFound", name, err)
		}
	}
	if !vault.objects[deed.ObjectKey] || repo.assets[deed.ID].Visibility != Private || repo.assets[deed.ID].Title != "" || repo.assets[deed.ID].FocalX != nil || repo.assets[pending.ID].Status != AssetStatusPending {
		t.Error("a writer told of the wrong half changed the file anyway")
	}
}

func TestNoWriterGivesAPrivateFileAnAddress(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	ctx := context.Background()
	deed := seedIn(repo, open, Private, "contracts", "deed.jpg")
	delete(open.objects, deed.ObjectKey)
	vault.objects[deed.ObjectKey] = true
	pending := seedIn(repo, open, Private, "contracts", "new.pdf")
	pending.Status, pending.MimeType = AssetStatusPending, "application/pdf"
	repo.assets[pending.ID] = pending
	vault.objects[pending.ObjectKey] = true
	x, title, name := 0.5, "Deed", "deed-2.jpg"

	results := map[string]func() (Asset, error){
		"focal point": func() (Asset, error) { return svc.SetFocalPoint(ctx, deed.ID, Private, &x, &x) },
		"words": func() (Asset, error) {
			return svc.UpdateAsset(ctx, UpdateAssetInput{ID: deed.ID, Visibility: Private, Title: &title})
		},
		"rename": func() (Asset, error) {
			return svc.MoveAsset(ctx, MoveAssetInput{ID: deed.ID, From: Private, TargetPath: "contracts", Filename: &name})
		},
		"confirm": func() (Asset, error) {
			return svc.ConfirmUpload(ctx, ConfirmUploadInput{AssetID: pending.ID, Visibility: Private})
		},
		"read": func() (Asset, error) { return svc.GetAnyAsset(ctx, deed.ID) },
	}
	for what, run := range results {
		got, err := run()
		if err != nil {
			t.Errorf("%s: %v", what, err)
			continue
		}
		if got.URL != "" {
			t.Errorf("%s gave a private file the address %q", what, got.URL)
		}
	}
}

func TestResizedCopiesAreSweptAgainOnceRendersHaveFinished(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	var later []func()
	svc.later = func(f func()) { later = append(later, f) }
	photo := seedIn(repo, open, Public, "banners", "a.jpg")
	deed := seedIn(repo, open, Private, "ids", "b.jpg")
	delete(open.objects, deed.ObjectKey)
	vault.objects[deed.ObjectKey] = true
	ctx := context.Background()

	if err := svc.DeleteAsset(ctx, photo.ID, Public, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAsset(ctx, deed.ID, Private, true); err != nil {
		t.Fatal(err)
	}
	if len(later) != 1 {
		t.Fatalf("%d sweeps scheduled, want one, for the public picture", len(later))
	}
	late := "derived/w640-webp/" + photo.ObjectKey
	open.objects[late] = true
	later[0]()
	if open.objects[late] {
		t.Error("a copy written by a render that finished after the delete is still public")
	}
}

func TestTheOptionUploadSweepUsesEachFilesOwnStore(t *testing.T) {
	svc, open, vault, repo := twoHalves()
	art := seedIn(repo, open, Private, "product-options/p1", "art.png")
	delete(open.objects, art.ObjectKey)
	vault.objects[art.ObjectKey] = true
	n, err := svc.CleanupProductOptionUploads(context.Background(), time.Hour, 10)
	if err != nil || n != 1 || vault.objects[art.ObjectKey] {
		t.Errorf("swept %d (%v), and the private store still holds it: %v", n, err, vault.objects[art.ObjectKey])
	}
}
