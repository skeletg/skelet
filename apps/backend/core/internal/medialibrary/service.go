package medialibrary

import (
	"context"
	"fmt"
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

// The object storage the library manages - *objstore.Storage in production
type Storage interface {
	PublicURL(key string) string
	MaxUploadBytes() int64
	PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error)
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	PutEmptyObject(ctx context.Context, key string) error
	Head(ctx context.Context, key string) (int64, string, bool, error)
	Copy(ctx context.Context, fromKey, toKey string) error
	Delete(ctx context.Context, key string) error
	ListPrefix(ctx context.Context, prefix string) ([]string, error)
	ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error)
	ListDirectoryContents(ctx context.Context, prefix string) ([]string, []objstore.StorageObject, error)
}

// What the rest of the platform writes into the same bucket - product, category and collection images, customers' form uploads, data export and import files, image derivatives
// With no root prefix the library spans the whole bucket, so without this a media.view admin browsed customer exports and one recursive delete took every product image with it
var reservedTopLevel = []string{"catalog", "webforms", "data", "derived"}

type Service struct {
	repo       Repository
	storage    Storage
	rootPrefix string
}

func NewService(repo Repository, storage Storage, rootPrefix string) *Service {
	rootPrefix = strings.Trim(strings.TrimSpace(rootPrefix), "/")
	return &Service{repo: repo, storage: storage, rootPrefix: rootPrefix}
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

func (s *Service) GetDirectory(ctx context.Context, dirPath string) (Directory, error) {
	cleanPath, err := s.libraryPath(dirPath)
	if err != nil {
		return Directory{}, err
	}
	objPrefix := s.objectPrefix(cleanPath)
	assets, err := s.repo.ListAssetsByDirectory(ctx, objPrefix)
	if err != nil {
		return Directory{}, err
	}
	trackedKeys := make(map[string]struct{}, len(assets))
	for i := range assets {
		assets[i].URL = s.storage.PublicURL(assets[i].ObjectKey)
		trackedKeys[assets[i].ObjectKey] = struct{}{}
	}

	prefixes, s3Objects, err := s.storage.ListDirectoryContents(ctx, objPrefix)
	if err != nil {
		return Directory{}, fmt.Errorf("list directory contents: %w", err)
	}

	dirs := make([]DirectoryChild, 0, len(prefixes))
	seen := map[string]struct{}{}
	for _, prefix := range prefixes {
		childPath := strings.TrimSuffix(strings.TrimPrefix(prefix, s.rootPrefix+"/"), "/")
		if childPath == "" || childPath == cleanPath || s.reserved(childPath) {
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
			URL:          s.storage.PublicURL(obj.Key),
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
		result.Items[i].URL = s.storage.PublicURL(result.Items[i].ObjectKey)
	}
	return result, nil
}

func (s *Service) CreateDirectory(ctx context.Context, parentPath, name string) (DirectoryChild, error) {
	parentPath, err := s.libraryPath(parentPath)
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
	if err := s.storage.PutEmptyObject(ctx, s.markerKey(childPath)); err != nil {
		return DirectoryChild{}, fmt.Errorf("create directory marker: %w", err)
	}
	return DirectoryChild{Path: childPath, Name: name}, nil
}

func (s *Service) RequestUpload(ctx context.Context, input RequestUploadInput) (RequestUploadResult, error) {
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
	if input.SizeBytes > s.storage.MaxUploadBytes() {
		return RequestUploadResult{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, input.SizeBytes, s.storage.MaxUploadBytes())
	}
	assetID, err := uuid.NewV7()
	if err != nil {
		return RequestUploadResult{}, fmt.Errorf("generate asset id: %w", err)
	}
	objectKey := s.buildObjectKey(cleanPath, assetID, filename, input.MimeType)
	uploadURL, err := s.storage.PresignUpload(ctx, objectKey, input.MimeType, input.SizeBytes, presignTTL)
	if err != nil {
		return RequestUploadResult{}, fmt.Errorf("presign upload: %w", err)
	}
	adminID := adminIDFromContext(ctx)
	asset, err := s.repo.CreateAsset(ctx, Asset{
		ID:              assetID,
		DirectoryPrefix: cleanPath,
		Filename:        filename,
		ObjectKey:       objectKey,
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
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return RequestUploadResult{
		Asset:     asset,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().Add(presignTTL),
	}, nil
}

// MimeSVG never takes the presigned route. Every other upload goes browser -> S3 directly and the backend only hears about it afterwards, which is fine for a JPEG and useless for a format that has to be rewritten before anyone can be allowed to fetch it - a sanitize-on-confirm design would leave the raw bytes readable at a public key for as long as the uploader chose never to confirm
const MimeSVG = "image/svg+xml"

type UploadSVGInput struct {
	Path     string
	Filename string
	Data     []byte
	Title    string
	AltText  string
}

// UploadSVG sanitizes an SVG and stores the RESULT, returning a ready asset. The bytes the caller supplied are never written anywhere - only what SanitizeSVG produced is, so there is no window in which the original is fetchable
func (s *Service) UploadSVG(ctx context.Context, input UploadSVGInput) (Asset, error) {
	cleanPath, err := s.libraryPath(input.Path)
	if err != nil {
		return Asset{}, err
	}
	filename, err := sanitizeFilename(input.Filename)
	if err != nil {
		return Asset{}, err
	}
	if int64(len(input.Data)) > s.storage.MaxUploadBytes() {
		return Asset{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, len(input.Data), s.storage.MaxUploadBytes())
	}
	clean, err := media.SanitizeSVG(input.Data)
	if err != nil {
		return Asset{}, fmt.Errorf("%w: %w", ErrInvalidMimeType, err)
	}
	assetID, err := uuid.NewV7()
	if err != nil {
		return Asset{}, fmt.Errorf("generate asset id: %w", err)
	}
	objectKey := s.buildObjectKey(cleanPath, assetID, filename, MimeSVG)
	if err := s.storage.PutBytes(ctx, objectKey, clean, MimeSVG); err != nil {
		return Asset{}, fmt.Errorf("store sanitized svg: %w", err)
	}
	adminID := adminIDFromContext(ctx)
	// Straight to ready: the object is already in place, so there is nothing left to confirm
	asset, err := s.repo.CreateAsset(ctx, Asset{
		ID:              assetID,
		DirectoryPrefix: cleanPath,
		Filename:        filename,
		ObjectKey:       objectKey,
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
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return asset, nil
}

func (s *Service) ConfirmUpload(ctx context.Context, input ConfirmUploadInput) (Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return Asset{}, err
	}
	if asset.Status != AssetStatusPending {
		return Asset{}, fmt.Errorf("%w: status=%s", ErrAssetNotPending, asset.Status)
	}
	_, _, exists, err := s.storage.Head(ctx, asset.ObjectKey)
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
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return asset, nil
}

func (s *Service) GetAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, id)
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return asset, nil
}

// GetAssetsByIDs is the batched (DataLoader) form of GetAsset - URL filled in per asset
func (s *Service) GetAssetsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Asset, error) {
	assets, err := s.repo.GetAssetsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, a := range assets {
		a.URL = s.storage.PublicURL(a.ObjectKey)
		assets[id] = a
	}
	return assets, nil
}

func (s *Service) UpdateAsset(ctx context.Context, input UpdateAssetInput) (Asset, error) {
	asset, err := s.repo.UpdateAssetMetadata(ctx, input, adminIDFromContext(ctx))
	if err != nil {
		return Asset{}, err
	}
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return asset, nil
}

func (s *Service) MoveAsset(ctx context.Context, input MoveAssetInput) (Asset, error) {
	asset, err := s.repo.GetAssetByID(ctx, input.ID)
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
	oldKey := asset.ObjectKey
	newKey := s.buildObjectKey(targetPath, asset.ID, filename, asset.MimeType)
	// Copy, repoint the row, THEN drop the old object - deleting first left a failed row update pointing at a key that no longer existed
	if newKey != oldKey {
		if err := s.storage.Copy(ctx, oldKey, newKey); err != nil {
			return Asset{}, fmt.Errorf("copy media asset: %w", err)
		}
	}
	asset, err = s.repo.UpdateAssetLocation(ctx, asset.ID, targetPath, filename, newKey, adminIDFromContext(ctx))
	if err != nil {
		return Asset{}, err
	}
	if newKey != oldKey {
		s.dropOld(ctx, oldKey)
	}
	asset.URL = s.storage.PublicURL(asset.ObjectKey)
	return asset, nil
}

func (s *Service) DeleteAsset(ctx context.Context, id uuid.UUID, deleteFromStorage bool) error {
	asset, err := s.repo.GetAssetByID(ctx, id)
	if err != nil {
		return err
	}
	// Object first, the way the orphan sweep does it - dropping the row first leaked the object for good when the storage delete then failed, with nothing left to retry from
	if deleteFromStorage {
		if err := s.storage.Delete(ctx, asset.ObjectKey); err != nil {
			return fmt.Errorf("delete storage object: %w", err)
		}
	}
	return s.repo.DeleteAsset(ctx, id)
}

// The superseded copy after a move - the row already points at the new key, so a failure here only leaves a stray object for the orphan tooling, never a broken asset
func (s *Service) dropOld(ctx context.Context, key string) {
	if err := s.storage.Delete(ctx, key); err != nil {
		slog.Warn("media library: old object left behind", "key", key, "err", err)
	}
}

// CleanupProductOptionUploads deletes customer option uploads that never attached to a cart line (or whose line was removed) - presigned uploads land here with no confirm step, so without this sweep abandoned artwork accumulates forever. Assets referenced by any cart or order line are kept
func (s *Service) CleanupProductOptionUploads(ctx context.Context, olderThan time.Duration, maxRows int32) (int, error) {
	orphans, err := s.repo.ListOrphanProductOptionAssets(ctx, time.Now().Add(-olderThan), maxRows)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, a := range orphans {
		// object first - if the row delete then fails the next sweep still sees the row and retries, whereas a deleted row would leak the object forever
		if err := s.storage.Delete(ctx, a.ObjectKey); err != nil {
			return deleted, fmt.Errorf("delete orphan option object %q: %w", a.ObjectKey, err)
		}
		if err := s.repo.DeleteAsset(ctx, a.ID); err != nil {
			return deleted, fmt.Errorf("delete orphan option upload %s: %w", a.ID, err)
		}
		deleted++
	}
	return deleted, nil
}

func (s *Service) DeleteDirectory(ctx context.Context, dirPath string, recursive bool) error {
	cleanPath, err := s.libraryPath(dirPath)
	if err != nil {
		return err
	}
	if cleanPath == "" && recursive {
		return ErrRootDeleteNotAllowed
	}
	objectPrefix := s.objectPrefix(cleanPath)
	if !recursive {
		assets, err := s.repo.ListAssetsByPathRecursive(ctx, objectPrefix)
		if err != nil {
			return err
		}
		if len(assets) > 0 {
			return ErrDirectoryNotEmpty
		}
		children, err := s.storage.ListImmediatePrefixes(ctx, objectPrefix)
		if err != nil {
			return fmt.Errorf("list child directories: %w", err)
		}
		for _, child := range children {
			if strings.TrimSuffix(child, "/") != strings.TrimSuffix(objectPrefix, "/") {
				return ErrDirectoryNotEmpty
			}
		}
		return s.storage.Delete(ctx, s.markerKey(cleanPath))
	}
	keys, err := s.storage.ListPrefix(ctx, objectPrefix)
	if err != nil {
		return fmt.Errorf("list directory objects: %w", err)
	}
	for _, key := range keys {
		if err := s.storage.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete directory object %s: %w", key, err)
		}
	}
	if objectPrefix != "" {
		if err := s.repo.DeleteAssetsByDirectoryPrefix(ctx, objectPrefix); err != nil {
			return err
		}
	}
	return nil
}

// ListAllDirectories walks the directory prefixes breadth-first - listing every key to derive them read the whole bucket, every image derivative included, on each call
func (s *Service) ListAllDirectories(ctx context.Context) ([]DirectoryChild, error) {
	var dirs []DirectoryChild
	queue := []string{s.objectPrefix("")}
	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]
		children, err := s.storage.ListImmediatePrefixes(ctx, prefix)
		if err != nil {
			return nil, fmt.Errorf("list directories: %w", err)
		}
		for _, child := range children {
			rel := strings.TrimSuffix(strings.TrimPrefix(child, s.objectPrefix("")), "/")
			if rel == "" || s.reserved(rel) {
				continue
			}
			dirs = append(dirs, DirectoryChild{Path: rel, Name: path.Base(rel)})
			queue = append(queue, child)
		}
	}
	slices.SortFunc(dirs, func(a, b DirectoryChild) int { return strings.Compare(a.Path, b.Path) })
	return dirs, nil
}

func (s *Service) RenameDirectory(ctx context.Context, dirPath, newName string) (DirectoryChild, error) {
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
	if existing, err := s.storage.ListPrefix(ctx, newPrefix); err != nil {
		return DirectoryChild{}, fmt.Errorf("check rename target: %w", err)
	} else if len(existing) > 0 {
		return DirectoryChild{}, fmt.Errorf("%w: %s", ErrPathTaken, newPath)
	}
	keys, err := s.storage.ListPrefix(ctx, oldPrefix)
	if err != nil {
		return DirectoryChild{}, fmt.Errorf("list directory objects: %w", err)
	}
	// Copy everything, repoint the rows, then drop the originals - deleting as it went left a mid-rename failure with rows pointing at keys already gone
	for _, key := range keys {
		if err := s.storage.Copy(ctx, key, newPrefix+strings.TrimPrefix(key, oldPrefix)); err != nil {
			return DirectoryChild{}, fmt.Errorf("copy object during directory rename: %w", err)
		}
	}
	assets, err := s.repo.ListAssetsByPathRecursive(ctx, oldPrefix)
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
		if _, err := s.repo.UpdateAssetLocation(ctx, asset.ID, newDirPrefix, asset.Filename, newKey, adminID); err != nil {
			return DirectoryChild{}, err
		}
	}
	for _, key := range keys {
		s.dropOld(ctx, key)
	}
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

func (s *Service) buildObjectKey(dirPath string, id uuid.UUID, filename, mimeType string) string {
	ext := extensionForMime(mimeType)
	base := strings.TrimSuffix(filename, path.Ext(filename))
	base = strings.ToLower(filenameSanitizer.ReplaceAllString(base, "-"))
	base = strings.Trim(base, "-")
	if base == "" {
		base = "asset"
	}
	return fmt.Sprintf("%s%s-%s%s", s.objectPrefix(dirPath), id.String(), base, ext)
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
func (s *Service) SetFocalPoint(ctx context.Context, id uuid.UUID, x, y *float64) (Asset, error) {
	if (x == nil) != (y == nil) {
		return Asset{}, ErrInvalidFocalPoint
	}
	if x != nil && (*x < 0 || *x > 1 || *y < 0 || *y > 1) {
		return Asset{}, ErrInvalidFocalPoint
	}
	a, err := s.repo.SetFocalPoint(ctx, id, x, y)
	if err != nil {
		return Asset{}, err
	}
	a.URL = s.storage.PublicURL(a.ObjectKey)
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
