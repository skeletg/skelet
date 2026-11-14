package medialibrary

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
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
	// The private half asks for these on top of the two above, and only a grant on all channels counts
	permission.Register(permission.Definition{
		Group:       "media",
		Slug:        PermPrivateView,
		Name:        "View private media",
		Description: "See and download the files in the media library's private half. Needs View media library as well, and counts only when granted on all channels.",
		SortOrder:   3,
	})
	permission.Register(permission.Definition{
		Group:       "media",
		Slug:        PermPrivateManage,
		Name:        "Manage private media",
		Description: "Upload to, organise and delete in the private half, and move a file between the halves. Making a private file public gives it an address anyone can open, so grant it separately from Manage media library. Counts only when granted on all channels.",
		SortOrder:   4,
	})
	permission.Register(permission.Definition{
		Group:       "media",
		Slug:        PermShare,
		Name:        "Share media with customers",
		Description: "Hand a customer a private file through their account's downloads, and take it back. A file leaves the shop on this permission. Also needs View Customers for the customer's channel, and counts only when granted on all channels.",
		SortOrder:   5,
	})
}

const (
	PermPrivateView   = "media.private.view"
	PermPrivateManage = "media.private.manage"
	PermShare         = "media.share"
)

type AssetStatus string

const (
	AssetStatusPending AssetStatus = "pending"
	AssetStatusReady   AssetStatus = "ready"
)

// A private file is kept in the private store, has no address, and is read only through a route that checks who is asking
type Visibility string

const (
	Public  Visibility = "public"
	Private Visibility = "private"
)

// The zero value is public, which is what every caller that predates the private half means
func (v Visibility) OrPublic() Visibility {
	if v == Private {
		return Private
	}
	return Public
}

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
	Visibility      Visibility
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
	Path       string
	Visibility Visibility
	Filename   string
	MimeType   string
	SizeBytes  int64
	Title      string
	AltText    string
	Caption    string
	Notes      string
}

type RequestUploadResult struct {
	Asset     Asset
	UploadURL string
	ExpiresAt time.Time
}

// Visibility on a by-id writer is the half the caller was checked against, public when left out - a file in the other isn't found
type ConfirmUploadInput struct {
	AssetID    uuid.UUID
	Visibility Visibility
	Width      *int32
	Height     *int32
}

type UpdateAssetInput struct {
	ID         uuid.UUID
	Visibility Visibility
	Title      *string
	AltText    *string
	Caption    *string
	Notes      *string
}

type MoveAssetInput struct {
	ID         uuid.UUID
	From       Visibility
	TargetPath string
	Filename   *string
	Visibility *Visibility
}

type SearchInput struct {
	Query      *string
	Path       *string
	Visibility Visibility
	MimeGroup  *MimeGroup
	// newest (the default), oldest, name or largest
	Sort   string
	Limit  int32
	Offset int32
}

type Repository interface {
	CreateAsset(ctx context.Context, asset Asset) (Asset, error)
	SetFocalPoint(ctx context.Context, id uuid.UUID, x, y *float64) (Asset, error)
	GetByObjectKey(ctx context.Context, key string) (Asset, bool, error)
	GetAssetByID(ctx context.Context, id uuid.UUID) (Asset, error)
	GetAssetsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Asset, error)
	ListAssetsByDirectory(ctx context.Context, path string, in Visibility) ([]Asset, error)
	ListAssetsByPathRecursive(ctx context.Context, path string, in Visibility) ([]Asset, error)
	ListOrphanProductOptionAssets(ctx context.Context, cutoff time.Time, maxRows int32) ([]Asset, error)
	SearchAssets(ctx context.Context, input SearchInput) (SearchResult, error)
	ConfirmAssetUpload(ctx context.Context, id uuid.UUID, width, height *int32, updatedBy *uuid.UUID) (Asset, error)
	UpdateAssetMetadata(ctx context.Context, input UpdateAssetInput, updatedBy *uuid.UUID) (Asset, error)
	UpdateAssetLocation(ctx context.Context, id uuid.UUID, path, filename, objectKey string, in Visibility, updatedBy *uuid.UUID) (Asset, error)
	DeleteAsset(ctx context.Context, id uuid.UUID) error
	DeleteAssetsByDirectoryPrefix(ctx context.Context, path string, in Visibility) error
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
	// Deleting the root recursively empties the whole library - or with no root prefix, the whole bucket
	ErrRootDeleteNotAllowed = errors.New("the library root cannot be deleted")
	ErrReservedPath         = errors.New("this path belongs to the platform, not the media library")
	ErrInvalidSize          = errors.New("file size must be positive")
	ErrInvalidDimensions    = errors.New("width and height are required for images")
	ErrInvalidFocalPoint    = errors.New("a focal point needs both x and y within 0..1, or neither to clear it")
	ErrNotEditable          = errors.New("only a finished JPEG, PNG or WebP upload can be edited")
	ErrNoPrivateStore       = errors.New("this install has no private store, so the library has no private half")
	ErrPrivateAsset         = errors.New("a private file can't be used here - it has no address, and it isn't sent anywhere outside the shop")
	ErrSharePublic          = errors.New("only a private file is shared - a public one already has an address anyone can open")
	ErrStillPublic          = errors.New("the file couldn't be made private: its public copy couldn't be removed, so it can still be opened at its address - try again")
)
