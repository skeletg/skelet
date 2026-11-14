package medialibrary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

const presignTTL = 15 * time.Minute

var allowedDocumentMimeTypes = []string{
	"application/pdf",
}

var filenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// The object storage the library manages - objstore.Storage in production
type Storage interface {
	PublicURL(key string) string
	HasPublicBase() bool
	MaxUploadBytes() int64
	PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error)
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	PutEmptyObject(ctx context.Context, key string) error
	Head(ctx context.Context, key string) (int64, string, bool, error)
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
	UploadStream(ctx context.Context, key, contentType string, body io.Reader) error
	Copy(ctx context.Context, fromKey, toKey string) error
	Delete(ctx context.Context, key string) error
	ListPrefix(ctx context.Context, prefix string) ([]string, error)
	ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error)
	ListDirectoryContents(ctx context.Context, prefix string) ([]string, []objstore.StorageObject, error)
}

// What the rest of the platform writes into the same bucket - product, category and collection images, customers' form uploads, data export and import files, image derivatives
// With no root prefix the library spans the whole bucket, so without this a media.view admin browsed customer exports and one recursive delete took every product image with it
var reservedTopLevel = []string{media.CatalogRoot, media.FormsRoot, media.DataJobsRoot, strings.TrimSuffix(media.DerivedRoot, "/")}

type Service struct {
	repo       Repository
	storage    Storage
	private    Storage
	rootPrefix string
	cfg        media.KeyConfig
	thumbs     thumbMemo
	later      func(func())
}

// A render already under way when a picture's resized copies go can write one back, so they are swept again after this
var lateSweep = media.RenderTimeout + 5*time.Second

func NewService(repo Repository, storage Storage, rootPrefix string) *Service {
	rootPrefix = strings.Trim(strings.TrimSpace(rootPrefix), "/")
	return &Service{repo: repo, storage: storage, rootPrefix: rootPrefix, later: func(f func()) { time.AfterFunc(lateSweep, f) }}
}

// The file as the caller was checked against it: one that has since gone to the other half isn't the one they may touch
func (s *Service) assetIn(ctx context.Context, id uuid.UUID, in Visibility) (Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, id)
	if err != nil {
		return Asset{}, err
	}
	if asset.Visibility.OrPublic() != in.OrPublic() {
		return Asset{}, ErrAssetNotFound
	}
	return asset, nil
}

func (s *Service) SetConfig(cfg media.KeyConfig) { s.cfg = cfg }

// Only a store that isn't the media one makes a private half - "private" files in the public bucket would be a lie
func (s *Service) SetPrivateStore(st Storage) {
	if st != nil && st != s.storage {
		s.private = st
	}
}

func (s *Service) HasPrivate() bool { return s.private != nil }

func (s *Service) store(in Visibility) (Storage, error) {
	if in != Private {
		return s.storage, nil
	}
	if s.private == nil {
		return nil, ErrNoPrivateStore
	}
	return s.private, nil
}

func (s *Service) addressOf(a Asset) string {
	if a.Visibility == Private {
		return ""
	}
	return s.storage.PublicURL(a.ObjectKey)
}

// PublicURL is a storage pass-through, exposed so callers without a Storage ref (e.g. review resolvers) can build URLs
func (s *Service) PublicURL(objectKey string) string {
	return s.storage.PublicURL(objectKey)
}

// MaxUploadBytes mirrors the storage cap, surfaced so callers can size-check before RequestUpload
func (s *Service) MaxUploadBytes() int64 {
	return s.storage.MaxUploadBytes()
}

// A library path the platform owns - only possible when the library spans the whole bucket
func (s *Service) reserved(cleanPath string) bool {
	if s.rootPrefix != "" {
		return false
	}
	top, _, _ := strings.Cut(cleanPath, "/")
	return slices.Contains(reservedTopLevel, top)
}

// sanitizeDirectoryPath plus the reserved-prefix refusal every library operation shares
func (s *Service) libraryPath(dirPath string) (string, error) {
	p, err := sanitizeDirectoryPath(dirPath)
	if err != nil {
		return "", err
	}
	if s.reserved(p) {
		return "", fmt.Errorf("%w: %s", ErrReservedPath, p)
	}
	return p, nil
}

func (s *Service) GetDirectory(ctx context.Context, dirPath string, in Visibility) (Directory, error) {
	st, err := s.store(in)
	if err != nil {
		return Directory{}, err
	}
	cleanPath, err := s.libraryPath(dirPath)
	if err != nil {
		return Directory{}, err
	}
	objPrefix := s.objectPrefix(cleanPath)
	assets, err := s.repo.ListAssetsByDirectory(ctx, objPrefix, in)
	if err != nil {
		return Directory{}, err
	}
	trackedKeys := make(map[string]struct{}, len(assets))
	for i := range assets {
		assets[i].URL = s.addressOf(assets[i])
		trackedKeys[assets[i].ObjectKey] = struct{}{}
	}

	prefixes, s3Objects, err := st.ListDirectoryContents(ctx, objPrefix)
	if err != nil {
		return Directory{}, fmt.Errorf("list directory contents: %w", err)
	}

	dirs := make([]DirectoryChild, 0, len(prefixes))
	seen := map[string]struct{}{}
	for _, prefix := range prefixes {
		childPath := strings.TrimSuffix(strings.TrimPrefix(prefix, s.rootPrefix+"/"), "/")
		if childPath == "" || childPath == cleanPath || s.reserved(childPath) || strings.HasPrefix(path.Base(childPath), ".") {
			continue
		}
		if _, ok := seen[childPath]; ok {
			continue
		}
		seen[childPath] = struct{}{}
		dirs = append(dirs, DirectoryChild{Path: childPath, Name: path.Base(childPath)})
	}
	slices.SortFunc(dirs, func(a, b DirectoryChild) int { return strings.Compare(a.Name, b.Name) })

	var untracked []UntrackedFile
	for _, obj := range s3Objects {
		filename := path.Base(obj.Key)
		if filename == ".keep" {
			continue
		}
		if _, tracked := trackedKeys[obj.Key]; tracked {
			continue
		}
		mimeType := mime.TypeByExtension(path.Ext(filename))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		untracked = append(untracked, UntrackedFile{
			ObjectKey:    obj.Key,
			URL:          s.addressOf(Asset{ObjectKey: obj.Key, Visibility: in}),
			Filename:     filename,
			MimeType:     mimeType,
			SizeBytes:    obj.Size,
			LastModified: obj.LastModified,
		})
	}

	return Directory{
		Path:           cleanPath,
		Name:           directoryName(cleanPath),
		Directories:    dirs,
		Assets:         assets,
		UntrackedFiles: untracked,
	}, nil
}

func (s *Service) SearchAssets(ctx context.Context, input SearchInput) (SearchResult, error) {
	if _, err := s.store(input.Visibility); err != nil {
		return SearchResult{}, err
	}
	if input.Limit <= 0 {
		input.Limit = 50
	}
	input.Limit = min(input.Limit, 200)
	if input.Offset < 0 {
		input.Offset = 0
	}
	if input.Path != nil {
		p, err := s.libraryPath(*input.Path)
		if err != nil {
			return SearchResult{}, err
		}
		prefix := s.objectPrefix(p)
		input.Path = &prefix
	}
	result, err := s.repo.SearchAssets(ctx, input)
	if err != nil {
		return SearchResult{}, err
	}
	for i := range result.Items {
		result.Items[i].URL = s.addressOf(result.Items[i])
	}
	return result, nil
}

func (s *Service) CreateDirectory(ctx context.Context, parentPath, name string, in Visibility) (DirectoryChild, error) {
	st, err := s.store(in)
	if err != nil {
		return DirectoryChild{}, err
	}
	parentPath, err = s.libraryPath(parentPath)
	if err != nil {
		return DirectoryChild{}, err
	}
	name = sanitizeDirectoryName(name)
	if name == "" {
		return DirectoryChild{}, ErrInvalidPath
	}
	childPath := name
	if parentPath != "" {
		childPath = parentPath + "/" + name
	}
	if s.reserved(childPath) {
		return DirectoryChild{}, fmt.Errorf("%w: %s", ErrReservedPath, childPath)
	}
	if err := st.PutEmptyObject(ctx, s.markerKey(childPath)); err != nil {
		return DirectoryChild{}, fmt.Errorf("create directory marker: %w", err)
	}
	return DirectoryChild{Path: childPath, Name: name}, nil
}

func (s *Service) RequestUpload(ctx context.Context, input RequestUploadInput) (RequestUploadResult, error) {
	st, err := s.store(input.Visibility)
	if err != nil {
		return RequestUploadResult{}, err
	}
	cleanPath, err := s.libraryPath(input.Path)
	if err != nil {
		return RequestUploadResult{}, err
	}
	filename, err := sanitizeFilename(input.Filename)
	if err != nil {
		return RequestUploadResult{}, err
	}
	if !isAllowedMime(input.MimeType) {
		return RequestUploadResult{}, fmt.Errorf("%w: %s", ErrInvalidMimeType, input.MimeType)
	}
	if input.SizeBytes <= 0 {
		return RequestUploadResult{}, ErrInvalidSize
	}
	if input.SizeBytes > st.MaxUploadBytes() {
		return RequestUploadResult{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, input.SizeBytes, st.MaxUploadBytes())
	}
	assetID, err := uuid.NewV7()
	if err != nil {
		return RequestUploadResult{}, fmt.Errorf("generate asset id: %w", err)
	}
	objectKey := s.buildObjectKey(ctx, cleanPath, assetID, filename, input.MimeType, time.Now())
	uploadURL, err := st.PresignUpload(ctx, objectKey, input.MimeType, input.SizeBytes, presignTTL)
	if err != nil {
		return RequestUploadResult{}, fmt.Errorf("presign upload: %w", err)
	}
	adminID := adminIDFromContext(ctx)
	asset, err := s.repo.CreateAsset(ctx, Asset{
		ID:              assetID,
		DirectoryPrefix: cleanPath,
		Filename:        filename,
		ObjectKey:       objectKey,
		Visibility:      input.Visibility.OrPublic(),
		MimeType:        input.MimeType,
		SizeBytes:       input.SizeBytes,
		Status:          AssetStatusPending,
		Title:           strings.TrimSpace(input.Title),
		AltText:         strings.TrimSpace(input.AltText),
		Caption:         strings.TrimSpace(input.Caption),
		Notes:           strings.TrimSpace(input.Notes),
		CreatedBy:       adminID,
		UpdatedBy:       adminID,
	})
	if err != nil {
		return RequestUploadResult{}, err
	}
	asset.URL = s.addressOf(asset)
	return RequestUploadResult{
		Asset:     asset,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().Add(presignTTL),
	}, nil
}

// MimeSVG never takes the presigned route. Every other upload goes browser -> S3 directly and the backend only hears about it afterwards, which is fine for a JPEG and useless for a format that has to be rewritten before anyone can be allowed to fetch it - a sanitize-on-confirm design would leave the raw bytes readable at a public key for as long as the uploader chose never to confirm
const MimeSVG = "image/svg+xml"

type UploadSVGInput struct {
	Path       string
	Visibility Visibility
	Filename   string
	Data       []byte
	Title      string
	AltText    string
}

// UploadSVG sanitizes an SVG and stores the RESULT, returning a ready asset. The bytes the caller supplied are never written anywhere - only what SanitizeSVG produced is, so there is no window in which the original is fetchable
func (s *Service) UploadSVG(ctx context.Context, input UploadSVGInput) (Asset, error) {
	st, err := s.store(input.Visibility)
	if err != nil {
		return Asset{}, err
	}
	cleanPath, err := s.libraryPath(input.Path)
	if err != nil {
		return Asset{}, err
	}
	filename, err := sanitizeFilename(input.Filename)
	if err != nil {
		return Asset{}, err
	}
	if int64(len(input.Data)) > st.MaxUploadBytes() {
		return Asset{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, len(input.Data), st.MaxUploadBytes())
	}
	clean, err := media.SanitizeSVG(input.Data)
	if err != nil {
		return Asset{}, fmt.Errorf("%w: %w", ErrInvalidMimeType, err)
	}
	assetID, err := uuid.NewV7()
	if err != nil {
		return Asset{}, fmt.Errorf("generate asset id: %w", err)
	}
	objectKey := s.buildObjectKey(ctx, cleanPath, assetID, filename, MimeSVG, time.Now())
	if err := st.PutBytes(ctx, objectKey, clean, MimeSVG); err != nil {
		return Asset{}, fmt.Errorf("store sanitized svg: %w", err)
	}
	adminID := adminIDFromContext(ctx)
	// Straight to ready: the object is already in place, so there is nothing left to confirm
	asset, err := s.repo.CreateAsset(ctx, Asset{
		ID:              assetID,
		DirectoryPrefix: cleanPath,
		Filename:        filename,
		ObjectKey:       objectKey,
		Visibility:      input.Visibility.OrPublic(),
		MimeType:        MimeSVG,
		SizeBytes:       int64(len(clean)),
		Status:          AssetStatusReady,
		Title:           strings.TrimSpace(input.Title),
		AltText:         strings.TrimSpace(input.AltText),
		CreatedBy:       adminID,
		UpdatedBy:       adminID,
	})
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.addressOf(asset)
	return asset, nil
}

func (s *Service) ConfirmUpload(ctx context.Context, input ConfirmUploadInput) (Asset, error) {
	asset, err := s.assetIn(ctx, input.AssetID, input.Visibility)
	if err != nil {
		return Asset{}, err
	}
	if asset.Status != AssetStatusPending {
		return Asset{}, fmt.Errorf("%w: status=%s", ErrAssetNotPending, asset.Status)
	}
	st, err := s.store(asset.Visibility)
	if err != nil {
		return Asset{}, err
	}
	_, _, exists, err := st.Head(ctx, asset.ObjectKey)
	if err != nil {
		return Asset{}, fmt.Errorf("head uploaded object: %w", err)
	}
	if !exists {
		return Asset{}, ErrUploadNotReceived
	}
	if asset.MimeGroup() == MimeGroupImage && (input.Width == nil || input.Height == nil || *input.Width <= 0 || *input.Height <= 0) {
		return Asset{}, ErrInvalidDimensions
	}
	asset, err = s.repo.ConfirmAssetUpload(ctx, input.AssetID, input.Width, input.Height, adminIDFromContext(ctx))
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.addressOf(asset)
	return asset, nil
}

// A public file. A private one isn't found here, so a caller that never thought about the private half can't hand one out
func (s *Service) GetAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	asset, err := s.GetAnyAsset(ctx, id)
	if err != nil {
		return Asset{}, err
	}
	if asset.Visibility == Private {
		return Asset{}, ErrAssetNotFound
	}
	return asset, nil
}

// A file from either half - only for a caller that goes on to check who is asking
func (s *Service) GetAnyAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, id)
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.addressOf(asset)
	return asset, nil
}

func (s *Service) Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, id)
	if err != nil {
		return nil, Asset{}, err
	}
	if asset.Status != AssetStatusReady {
		return nil, Asset{}, ErrAssetNotFound
	}
	st, err := s.store(asset.Visibility)
	if err != nil {
		return nil, Asset{}, err
	}
	body, err := st.GetStream(ctx, asset.ObjectKey)
	if err != nil {
		return nil, Asset{}, fmt.Errorf("open media asset: %w", err)
	}
	return body, asset, nil
}

// GetAssetsByIDs is the batched (DataLoader) form of GetAsset - URL filled in per asset
func (s *Service) GetAssetsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Asset, error) {
	assets, err := s.repo.GetAssetsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, a := range assets {
		if a.Visibility == Private {
			delete(assets, id)
			continue
		}
		a.URL = s.addressOf(a)
		assets[id] = a
	}
	return assets, nil
}

func (s *Service) UpdateAsset(ctx context.Context, input UpdateAssetInput) (Asset, error) {
	if _, err := s.assetIn(ctx, input.ID, input.Visibility); err != nil {
		return Asset{}, err
	}
	asset, err := s.repo.UpdateAssetMetadata(ctx, input, adminIDFromContext(ctx))
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.addressOf(asset)
	return asset, nil
}

func (s *Service) MoveAsset(ctx context.Context, input MoveAssetInput) (Asset, error) {
	asset, err := s.assetIn(ctx, input.ID, input.From)
	if err != nil {
		return Asset{}, err
	}
	targetPath, err := s.libraryPath(input.TargetPath)
	if err != nil {
		return Asset{}, err
	}
	filename := asset.Filename
	if input.Filename != nil {
		filename, err = sanitizeFilename(*input.Filename)
		if err != nil {
			return Asset{}, err
		}
	}
	was, now := asset.Visibility, asset.Visibility
	if input.Visibility != nil {
		now = input.Visibility.OrPublic()
	}
	from, err := s.store(was)
	if err != nil {
		return Asset{}, err
	}
	to, err := s.store(now)
	if err != nil {
		return Asset{}, err
	}
	oldKey := asset.ObjectKey
	newKey := s.buildObjectKey(ctx, targetPath, asset.ID, filename, asset.MimeType, asset.CreatedAt)
	moved := newKey != oldKey || now != was
	// Copy, repoint the row, THEN drop the old object - deleting first left a failed row update pointing at a key that no longer existed
	switch {
	case now != was:
		if err := carry(ctx, from, to, oldKey, newKey, asset.MimeType); err != nil {
			return Asset{}, err
		}
	case moved:
		if err := from.Copy(ctx, oldKey, newKey); err != nil {
			return Asset{}, fmt.Errorf("copy media asset: %w", err)
		}
	}
	before := asset
	asset, err = s.repo.UpdateAssetLocation(ctx, asset.ID, targetPath, filename, newKey, now, adminIDFromContext(ctx))
	if err != nil {
		if moved {
			dropCopy(ctx, to, newKey)
		}
		return Asset{}, err
	}
	if moved {
		if err := s.dropOld(ctx, from, oldKey, asset.MimeType); err != nil {
			if was != Public || now != Private {
				slog.Warn("media library: old object left behind", "key", oldKey, "err", err)
			} else {
				// Still at its public address, so not private yet - it goes back where it was, for the admin to try again
				if _, back := s.repo.UpdateAssetLocation(context.WithoutCancel(ctx), before.ID, before.DirectoryPrefix, before.Filename, oldKey, was, adminIDFromContext(ctx)); back != nil {
					slog.Error("media library: a file made private is still in the public store", "key", oldKey, "err", err, "rollback", back)
				} else {
					dropCopy(ctx, to, newKey)
				}
				return Asset{}, fmt.Errorf("%w: %w", ErrStillPublic, err)
			}
		}
	}
	asset.URL = s.addressOf(asset)
	return asset, nil
}

// A copy nothing points at - left in the public store it would be a private file at an address
func dropCopy(ctx context.Context, st Storage, key string) {
	if err := st.Delete(context.WithoutCancel(ctx), key); err != nil {
		slog.Warn("media library: copy left behind by a failed move", "key", key, "err", err)
	}
}

func carry(ctx context.Context, from, to Storage, oldKey, newKey, contentType string) error {
	body, err := from.GetStream(ctx, oldKey)
	if err != nil {
		return fmt.Errorf("read media asset to move it: %w", err)
	}
	defer func() { _ = body.Close() }()
	if err := to.UploadStream(ctx, newKey, contentType, body); err != nil {
		return fmt.Errorf("write media asset into the other store: %w", err)
	}
	return nil
}

func (s *Service) DeleteAsset(ctx context.Context, id uuid.UUID, in Visibility, deleteFromStorage bool) error {
	asset, err := s.assetIn(ctx, id, in)
	if err != nil {
		return err
	}
	// Object first, the way the orphan sweep does it - dropping the row first leaked the object for good when the storage delete then failed, with nothing left to retry from
	if deleteFromStorage {
		st, err := s.store(asset.Visibility)
		if err != nil {
			return err
		}
		if err := st.Delete(ctx, asset.ObjectKey); err != nil {
			return fmt.Errorf("delete storage object: %w", err)
		}
		if media.CanResize(asset.MimeType) {
			s.dropDerived(ctx, st, asset.ObjectKey)
		}
	}
	return s.repo.DeleteAsset(ctx, id)
}

// The superseded copy after a move and its resized copies, gone even if the request is - a public copy of a private file is still at its address
func (s *Service) dropOld(ctx context.Context, st Storage, key, mime string) error {
	ctx = context.WithoutCancel(ctx)
	if err := st.Delete(ctx, key); err != nil {
		return err
	}
	if !media.CanResize(mime) {
		return nil
	}
	if err := s.dropResizedCopies(ctx, st, key); err != nil {
		return err
	}
	s.sweepAgain(st, key)
	return nil
}

// Resized copies sit under derived/<size>/<the file's key>, and left behind they'd still be there to open
func (s *Service) dropDerived(ctx context.Context, st Storage, keyOrPrefix string) {
	if err := s.dropResizedCopies(context.WithoutCancel(ctx), st, keyOrPrefix); err != nil {
		slog.Warn("media library: resized copies left behind", "of", keyOrPrefix, "err", err)
	}
	s.sweepAgain(st, keyOrPrefix)
}

func (s *Service) sweepAgain(st Storage, keyOrPrefix string) {
	if st == s.storage {
		s.later(func() { _ = s.dropResizedCopies(context.Background(), st, keyOrPrefix) })
	}
}

func (s *Service) dropResizedCopies(ctx context.Context, st Storage, keyOrPrefix string) error {
	sizes, err := st.ListImmediatePrefixes(ctx, media.DerivedRoot)
	if err != nil {
		return err
	}
	var failed []error
	for _, size := range sizes {
		copies := []string{size + keyOrPrefix}
		if strings.HasSuffix(keyOrPrefix, "/") {
			if copies, err = st.ListPrefix(ctx, size+keyOrPrefix); err != nil {
				failed = append(failed, err)
				continue
			}
		}
		for _, key := range copies {
			if err := st.Delete(ctx, key); err != nil {
				failed = append(failed, fmt.Errorf("%s: %w", key, err))
			}
		}
	}
	return errors.Join(failed...)
}

// CleanupProductOptionUploads deletes customer option uploads that never attached to a cart line (or whose line was removed) - presigned uploads land here with no confirm step, so without this sweep abandoned artwork accumulates forever. Assets referenced by any cart or order line are kept
func (s *Service) CleanupProductOptionUploads(ctx context.Context, olderThan time.Duration, maxRows int32) (int, error) {
	orphans, err := s.repo.ListOrphanProductOptionAssets(ctx, time.Now().Add(-olderThan), maxRows)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, a := range orphans {
		st, err := s.store(a.Visibility)
		if err != nil {
			return deleted, err
		}
		// object first - if the row delete then fails the next sweep still sees the row and retries, whereas a deleted row would leak the object forever
		if err := st.Delete(ctx, a.ObjectKey); err != nil {
			return deleted, fmt.Errorf("delete orphan option object %q: %w", a.ObjectKey, err)
		}
		if err := s.repo.DeleteAsset(ctx, a.ID); err != nil {
			return deleted, fmt.Errorf("delete orphan option upload %s: %w", a.ID, err)
		}
		deleted++
	}
	return deleted, nil
}

func (s *Service) DeleteDirectory(ctx context.Context, dirPath string, recursive bool, in Visibility) error {
	st, err := s.store(in)
	if err != nil {
		return err
	}
	cleanPath, err := s.libraryPath(dirPath)
	if err != nil {
		return err
	}
	if cleanPath == "" && recursive {
		return ErrRootDeleteNotAllowed
	}
	objectPrefix := s.objectPrefix(cleanPath)
	if !recursive {
		assets, err := s.repo.ListAssetsByPathRecursive(ctx, objectPrefix, in)
		if err != nil {
			return err
		}
		if len(assets) > 0 {
			return ErrDirectoryNotEmpty
		}
		children, err := st.ListImmediatePrefixes(ctx, objectPrefix)
		if err != nil {
			return fmt.Errorf("list child directories: %w", err)
		}
		for _, child := range children {
			if strings.TrimSuffix(child, "/") != strings.TrimSuffix(objectPrefix, "/") {
				return ErrDirectoryNotEmpty
			}
		}
		return st.Delete(ctx, s.markerKey(cleanPath))
	}
	keys, err := st.ListPrefix(ctx, objectPrefix)
	if err != nil {
		return fmt.Errorf("list directory objects: %w", err)
	}
	for _, key := range keys {
		if err := st.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete directory object %s: %w", key, err)
		}
	}
	if objectPrefix != "" {
		if err := s.repo.DeleteAssetsByDirectoryPrefix(ctx, objectPrefix, in); err != nil {
			return err
		}
		s.dropDerived(ctx, st, objectPrefix)
	}
	return nil
}

// ListAllDirectories walks the directory prefixes breadth-first - listing every key to derive them read the whole bucket, every image derivative included, on each call
func (s *Service) ListAllDirectories(ctx context.Context, in Visibility) ([]DirectoryChild, error) {
	st, err := s.store(in)
	if err != nil {
		return nil, err
	}
	var dirs []DirectoryChild
	queue := []string{s.objectPrefix("")}
	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]
		children, err := st.ListImmediatePrefixes(ctx, prefix)
		if err != nil {
			return nil, fmt.Errorf("list directories: %w", err)
		}
		for _, child := range children {
			rel := strings.TrimSuffix(strings.TrimPrefix(child, s.objectPrefix("")), "/")
			if rel == "" || s.reserved(rel) || strings.HasPrefix(path.Base(rel), ".") {
				continue
			}
			dirs = append(dirs, DirectoryChild{Path: rel, Name: path.Base(rel)})
			queue = append(queue, child)
		}
	}
	slices.SortFunc(dirs, func(a, b DirectoryChild) int { return strings.Compare(a.Path, b.Path) })
	return dirs, nil
}

func (s *Service) RenameDirectory(ctx context.Context, dirPath, newName string, in Visibility) (DirectoryChild, error) {
	st, err := s.store(in)
	if err != nil {
		return DirectoryChild{}, err
	}
	oldPath, err := s.libraryPath(dirPath)
	if err != nil {
		return DirectoryChild{}, err
	}
	if oldPath == "" {
		return DirectoryChild{}, ErrRootRenameNotAllow
	}
	newName = sanitizeDirectoryName(newName)
	if newName == "" {
		return DirectoryChild{}, ErrInvalidPath
	}
	parent := path.Dir(oldPath)
	if parent == "." {
		parent = ""
	}
	newPath := newName
	if parent != "" {
		newPath = parent + "/" + newName
	}
	if s.reserved(newPath) {
		return DirectoryChild{}, fmt.Errorf("%w: %s", ErrReservedPath, newPath)
	}
	oldPrefix := s.objectPrefix(oldPath)
	newPrefix := s.objectPrefix(newPath)
	// Renaming onto a directory that already exists merged the two and overwrote whatever shared a key
	if existing, err := st.ListPrefix(ctx, newPrefix); err != nil {
		return DirectoryChild{}, fmt.Errorf("check rename target: %w", err)
	} else if len(existing) > 0 {
		return DirectoryChild{}, fmt.Errorf("%w: %s", ErrPathTaken, newPath)
	}
	keys, err := st.ListPrefix(ctx, oldPrefix)
	if err != nil {
		return DirectoryChild{}, fmt.Errorf("list directory objects: %w", err)
	}
	// Copy everything, repoint the rows, then drop the originals - deleting as it went left a mid-rename failure with rows pointing at keys already gone
	for _, key := range keys {
		if err := st.Copy(ctx, key, newPrefix+strings.TrimPrefix(key, oldPrefix)); err != nil {
			return DirectoryChild{}, fmt.Errorf("copy object during directory rename: %w", err)
		}
	}
	assets, err := s.repo.ListAssetsByPathRecursive(ctx, oldPrefix, in)
	if err != nil {
		return DirectoryChild{}, err
	}
	adminID := adminIDFromContext(ctx)
	for _, asset := range assets {
		relSuffix := strings.TrimPrefix(asset.ObjectKey, oldPrefix)
		newKey := newPrefix + relSuffix
		newKeyDir := path.Dir(newKey)
		if newKeyDir == "." {
			newKeyDir = ""
		}
		newDirPrefix := strings.TrimPrefix(strings.TrimPrefix(newKeyDir, s.rootPrefix), "/")
		if _, err := s.repo.UpdateAssetLocation(ctx, asset.ID, newDirPrefix, asset.Filename, newKey, in, adminID); err != nil {
			return DirectoryChild{}, err
		}
	}
	for _, key := range keys {
		if err := st.Delete(ctx, key); err != nil {
			slog.Warn("media library: old object left behind", "key", key, "err", err)
		}
	}
	s.dropDerived(ctx, st, oldPrefix)
	return DirectoryChild{Path: newPath, Name: newName}, nil
}

func (s *Service) objectPrefix(dirPath string) string {
	if s.rootPrefix == "" {
		if dirPath == "" {
			return ""
		}
		return dirPath + "/"
	}
	if dirPath == "" {
		return s.rootPrefix + "/"
	}
	return s.rootPrefix + "/" + dirPath + "/"
}

func (s *Service) markerKey(dirPath string) string {
	return s.objectPrefix(dirPath) + ".keep"
}

// at is when the file was uploaded, so a later move keeps a dated name
func (s *Service) buildObjectKey(ctx context.Context, dirPath string, id uuid.UUID, filename, mimeType string, at time.Time) string {
	name := media.LibraryFileKey.Template(ctx, s.cfg).Key(media.KeyValues{ID: id, Name: filename, Ext: extensionForMime(mimeType), At: at})
	return s.objectPrefix(dirPath) + name
}

func sanitizeDirectoryPath(dirPath string) (string, error) {
	raw := strings.Trim(strings.TrimSpace(dirPath), "/")
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeDirectoryName(part)
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return "", ErrInvalidPath
		}
		out = append(out, part)
	}
	return strings.Join(out, "/"), nil
}

func sanitizeDirectoryName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.Trim(name, "/")
	name = filenameSanitizer.ReplaceAllString(name, "-")
	return strings.Trim(name, "-.")
}

func sanitizeFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" {
		return "", ErrInvalidFilename
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext) // trim before lowercasing, or a phone's IMG_1.JPG keeps its extension and gains a second one
	ext = strings.ToLower(ext)
	base = filenameSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-.")
	if base == "" || ext == "" {
		return "", ErrInvalidFilename
	}
	return base + ext, nil
}

func extensionForMime(mimeType string) string {
	if ext := media.ExtensionForMime(mimeType); ext != "" {
		return ext
	}
	if mimeType == "application/pdf" {
		return ".pdf"
	}
	if mimeType == MimeSVG {
		return ".svg"
	}
	return ""
}

func UploadTypes() []string {
	return slices.Concat(media.AllowedImageMimeTypes, []string{MimeSVG}, media.AllowedVideoMimeTypes, allowedDocumentMimeTypes)
}

func isAllowedMime(mimeType string) bool {
	if media.IsAllowedImage(mimeType) {
		return true
	}
	if media.IsAllowedVideo(mimeType) {
		return true
	}
	return slices.Contains(allowedDocumentMimeTypes, mimeType)
}

func directoryName(p string) string {
	if p == "" {
		return "Library"
	}
	return path.Base(p)
}

func adminIDFromContext(ctx context.Context) *uuid.UUID {
	claims, ok := authpkg.AdminFromContext(ctx)
	if !ok {
		return nil
	}
	id := claims.AdminID
	return &id
}

// SetFocalPoint stores (or clears) an asset's smart-crop focal point,
// validating the 0..1 range and requiring both or neither coordinate
func (s *Service) SetFocalPoint(ctx context.Context, id uuid.UUID, in Visibility, x, y *float64) (Asset, error) {
	if (x == nil) != (y == nil) {
		return Asset{}, ErrInvalidFocalPoint
	}
	if x != nil && (*x < 0 || *x > 1 || *y < 0 || *y > 1) {
		return Asset{}, ErrInvalidFocalPoint
	}
	if _, err := s.assetIn(ctx, id, in); err != nil {
		return Asset{}, err
	}
	a, err := s.repo.SetFocalPoint(ctx, id, x, y)
	if err != nil {
		return Asset{}, err
	}
	a.URL = s.addressOf(a)
	return a, nil
}

// AssetByObjectKey resolves an active asset by storage key (transform
// endpoint focal lookup) - Fail-soft: (zero, false) when unknown
func (s *Service) AssetByObjectKey(ctx context.Context, key string) (Asset, bool) {
	a, ok, err := s.repo.GetByObjectKey(ctx, key)
	if err != nil || !ok {
		return Asset{}, false
	}
	return a, true
}
