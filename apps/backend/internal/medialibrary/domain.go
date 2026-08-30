package medialibrary

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "media",
		Name:      "Media",
		SortOrder: 68,
	})
	permission.Register(permission.Definition{
		Group: "media",
		Slug:  "media.view",
		Name:  "View media library",
	})
	permission.Register(permission.Definition{
		Group: "media",
		Slug:  "media.manage",
		Name:  "Manage media library",
	})
}

type AssetStatus string

const (
	AssetStatusPending AssetStatus = "pending"
	AssetStatusReady   AssetStatus = "ready"
)

type MimeGroup string

const (
	MimeGroupImage    MimeGroup = "image"
	MimeGroupDocument MimeGroup = "document"
)

type Asset struct {
	ID              uuid.UUID
	DirectoryPrefix string
	Filename        string
	ObjectKey       string
	URL             string
	MimeType        string
	SizeBytes       int64
	Width           *int32
	Height          *int32
	// Smart-crop focal point, 0..1 fractions - Nil = center
	FocalX    *float64
	FocalY    *float64
	Status    AssetStatus
	Title     string
	AltText   string
	Caption   string
	Notes     string
	CreatedBy *uuid.UUID
	UpdatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (a Asset) MimeGroup() MimeGroup {
	if len(a.MimeType) >= 6 && a.MimeType[:6] == "image/" {
		return MimeGroupImage
	}
	return MimeGroupDocument
}

func (a Asset) Path() string {
	if a.DirectoryPrefix == "" {
		return a.Filename
	}
	return a.DirectoryPrefix + "/" + a.Filename
}

type Directory struct {
	Path           string
	Name           string
	Directories    []DirectoryChild
	Assets         []Asset
	UntrackedFiles []UntrackedFile
}

// UntrackedFile represents an S3 object that has no corresponding media_assets row - Shown in the browser alongside
// tracked assets so admins can see and act on foreign or orphaned objects
type UntrackedFile struct {
	ObjectKey    string
	URL          string
	Filename     string
	MimeType     string
	SizeBytes    int64
	LastModified time.Time
}

type DirectoryChild struct {
	Path string
	Name string
}

type SearchResult struct {
	Items      []Asset
	TotalCount int32
}

type RequestUploadInput struct {
	Path      string
	Filename  string
	MimeType  string
	SizeBytes int64
	Title     string
	AltText   string
	Caption   string
	Notes     string
}

type RequestUploadResult struct {
	Asset     Asset
	UploadURL string
	ExpiresAt time.Time
}

type ConfirmUploadInput struct {
	AssetID uuid.UUID
	Width   *int32
	Height  *int32
}

type UpdateAssetInput struct {
	ID      uuid.UUID
	Title   *string
	AltText *string
	Caption *string
	Notes   *string
}

type MoveAssetInput struct {
	ID         uuid.UUID
	TargetPath string
	Filename   *string
}

type SearchInput struct {
	Query     *string
	Path      *string
	MimeGroup *MimeGroup
	Limit     int32
	Offset    int32
}

type Repository interface {
	CreateAsset(ctx context.Context, asset Asset) (Asset, error)
	SetFocalPoint(ctx context.Context, id uuid.UUID, x, y *float64) (Asset, error)
	GetByObjectKey(ctx context.Context, key string) (Asset, bool, error)
	GetAssetByID(ctx context.Context, id uuid.UUID) (Asset, error)
	GetAssetsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Asset, error)
	ListAssetsByDirectory(ctx context.Context, path string) ([]Asset, error)
	ListAssetsByPathRecursive(ctx context.Context, path string) ([]Asset, error)
	ListOrphanProductOptionAssets(ctx context.Context, cutoff time.Time, maxRows int32) ([]Asset, error)
	SearchAssets(ctx context.Context, input SearchInput) (SearchResult, error)
	ConfirmAssetUpload(ctx context.Context, id uuid.UUID, width, height *int32, updatedBy *uuid.UUID) (Asset, error)
	UpdateAssetMetadata(ctx context.Context, input UpdateAssetInput, updatedBy *uuid.UUID) (Asset, error)
	UpdateAssetLocation(ctx context.Context, id uuid.UUID, path, filename, objectKey string, updatedBy *uuid.UUID) (Asset, error)
	DeleteAsset(ctx context.Context, id uuid.UUID) error
	DeleteAssetsByDirectoryPrefix(ctx context.Context, path string) error
}

var (
	ErrAssetNotFound      = errors.New("media asset not found")
	ErrPathTaken          = errors.New("a media asset already exists at this path")
	ErrAssetNotPending    = errors.New("media asset is not pending")
	ErrInvalidMimeType    = errors.New("mime type not allowed")
	ErrFileTooLarge       = errors.New("file exceeds max upload size")
	ErrUploadNotReceived  = errors.New("upload not received at storage")
	ErrInvalidPath        = errors.New("invalid media path")
	ErrDirectoryNotEmpty  = errors.New("directory is not empty")
	ErrInvalidFilename    = errors.New("invalid filename")
	ErrRootRenameNotAllow = errors.New("root directory cannot be renamed")
)
