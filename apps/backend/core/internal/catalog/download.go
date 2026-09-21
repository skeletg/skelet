package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type ProductDownload struct { // catalog-side config only for one downloadable file or link - purchase grants access to each, MaxDownloads caps uses per grant (nil = unlimited); token issuance at placement and /download/:token live in internal/download
	ID           uuid.UUID
	ProductID    uuid.UUID
	Title        string
	URL          string
	MaxDownloads *int32
	SortOrder    int32
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AddProductDownloadInput struct {
	ProductID    uuid.UUID
	Title        string
	URL          string
	MaxDownloads *int32
	SortOrder    int32
}

type UpdateProductDownloadInput struct {
	ID           uuid.UUID
	Title        *string
	URL          *string
	MaxDownloads *int32
	SortOrder    *int32
}

var (
	ErrProductDownloadNotFound = errors.New("product download not found")
	ErrProductDownloadInvalid  = errors.New("product download requires a title and url")
)
