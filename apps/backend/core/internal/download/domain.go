// Package download owns digital-delivery grants - placing an order with a downloadable product turns each of the product's downloads into a per-buyer grant, an unguessable token with use and expiry limits
// The storefront resolves a token to the file and increments the use count under a row lock, so a capped download can't be exceeded by concurrent requests
package download

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Grant struct {
	ID         uuid.UUID
	Token      string
	OrderID    *uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	ProductID  uuid.UUID
	DownloadID uuid.UUID
	// Set instead of the product and its download when the shop shared a library file with the customer directly
	AssetID       *uuid.UUID
	Title         string
	URL           string
	DownloadsUsed int32
	MaxDownloads  *int32
	ExpiresAt     *time.Time
	CreatedAt     time.Time
}

type GrantInput struct {
	Token        string
	OrderID      *uuid.UUID
	CustomerID   *uuid.UUID
	Email        string
	ProductID    uuid.UUID
	DownloadID   uuid.UUID
	Title        string
	URL          string
	MaxDownloads *int32
	ExpiresAt    *time.Time
}

type ShareInput struct {
	AssetID      uuid.UUID
	CustomerID   uuid.UUID
	Email        string
	Title        string
	MaxDownloads *int32
	ExpiresAt    *time.Time
}

var (
	ErrNotFound         = errors.New("download grant not found")
	ErrExpired          = errors.New("download has expired")
	ErrLimitReached     = errors.New("download limit reached")
	ErrInvalidShare     = errors.New("a share needs a download limit above zero and an expiry in the future, or neither")
	ErrInactiveCustomer = errors.New("that customer's account is closed, so nothing can be shared with it")
)

type Repository interface {
	WithTx(tx *sql.Tx) Repository
	// Reports false when the order already holds a grant for this file
	CreateGrant(ctx context.Context, in GrantInput) (bool, error)
	RevokeForOrder(ctx context.Context, orderID uuid.UUID) (int64, error)
	GetByToken(ctx context.Context, token string) (Grant, error)
	// Row-locks the grant so the use-count check is serialized against concurrent downloads
	GetByTokenForUpdate(ctx context.Context, token string) (Grant, error)
	IncrementUse(ctx context.Context, id uuid.UUID) (Grant, error)
	ListByOrder(ctx context.Context, orderID uuid.UUID) ([]Grant, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Grant, error)
	ShareAsset(ctx context.Context, token string, in ShareInput) (Grant, error)
	ListByAsset(ctx context.Context, assetID uuid.UUID) ([]Grant, error)
	DeleteShare(ctx context.Context, id uuid.UUID) (bool, error)
	DeleteSharesWith(ctx context.Context, customerID uuid.UUID) error
}
