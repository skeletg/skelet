package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

const (
	// presignTTL caps how long an upload URL stays valid - Long enough for a realistic upload (slow connections, big files) but short enough that a leaked URL is low-value
	presignTTL          = 15 * time.Minute
	s3DeleteTimeout     = 30 * time.Second
	s3DeleteConcurrency = 8
)

// RequestMediaUpload validates the request, inserts a pending product_media row and returns a presigned PUT URL for the client
// Validation is the MIME allow-list, the storage's MaxUploadBytes and the product existing
func (s *mediaService) RequestMediaUpload(ctx context.Context, input RequestMediaUploadInput) (RequestMediaUploadResult, error) {
	if s.media == nil {
		return RequestMediaUploadResult{}, ErrMediaStorageUnavailable
	}
	if !media.IsAllowedImage(input.MimeType) {
		return RequestMediaUploadResult{}, fmt.Errorf("%w: %s", ErrInvalidMimeType, input.MimeType)
	}
	if input.SizeBytes <= 0 {
		return RequestMediaUploadResult{}, ErrInvalidSize
	}
	if input.SizeBytes > s.media.MaxUploadBytes() {
		return RequestMediaUploadResult{}, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, input.SizeBytes, s.media.MaxUploadBytes())
	}
	if _, err := s.repo.GetProductByID(ctx, input.ProductID); err != nil {
		return RequestMediaUploadResult{}, err
	}

	mediaID, err := uuid.NewV7()
	if err != nil {
		return RequestMediaUploadResult{}, fmt.Errorf("generate media id: %w", err)
	}
	ext := media.ExtensionForMime(input.MimeType)
	// Per-media subdirectory leaves room for future processed variants (thumb.jpg, large.webp) without schema churn
	typeSegment := string(MediaUploadTargetGallery)
	if input.Target != nil {
		typeSegment = string(*input.Target)
	} else if input.Role != nil {
		switch *input.Role {
		case MediaRoleMain:
			typeSegment = "main"
		case MediaRoleThumbnail:
			typeSegment = "thumbnail"
		case MediaRoleOGImage:
			typeSegment = "og"
		}
	}
	s3Key := fmt.Sprintf("catalog/products/%s/%s/%s%s", input.ProductID, typeSegment, mediaID, ext)

	url, err := s.media.PresignUpload(ctx, s3Key, input.MimeType, input.SizeBytes, presignTTL)
	if err != nil {
		return RequestMediaUploadResult{}, fmt.Errorf("presign: %w", err)
	}

	// Create the DB row AFTER successful presigning - no point inserting a pending row if presigning fails - Pass the pre-computed mediaID so the DB row and S3 key share the same ID
	m, err := s.repo.CreateProductMedia(ctx, mediaID, input.ProductID, s3Key, input.MimeType, input.SizeBytes, input.AltText, input.SortOrder)
	if err != nil {
		return RequestMediaUploadResult{}, err
	}

	m.URL = s.media.PublicURL(m.S3Key)
	return RequestMediaUploadResult{
		Media:     m,
		UploadURL: url,
		ExpiresAt: time.Now().Add(presignTTL),
	}, nil
}

// ConfirmMediaUpload verifies the client has actually uploaded the object to S3, then flips the media row to "ready" with the client-reported width/height - If the S3 object is missing (upload never happened or failed silently), returns ErrUploadNotReceived and the row stays in pending for a future orphan-cleanup job to delete
func (s *mediaService) ConfirmMediaUpload(ctx context.Context, input ConfirmMediaUploadInput) (ProductMedia, error) {
	m, err := s.repo.GetProductMediaByID(ctx, input.MediaID)
	if err != nil {
		return ProductMedia{}, err
	}
	if m.Status != MediaStatusPending {
		return ProductMedia{}, fmt.Errorf("%w: status=%s", ErrMediaNotPending, m.Status)
	}

	_, _, exists, err := s.media.Head(ctx, m.S3Key)
	if err != nil {
		return ProductMedia{}, fmt.Errorf("head s3 object: %w", err)
	}
	if !exists {
		return ProductMedia{}, ErrUploadNotReceived
	}

	if input.Width <= 0 || input.Height <= 0 {
		return ProductMedia{}, ErrInvalidDimensions
	}

	updated, err := s.repo.MarkProductMediaReady(ctx, input.MediaID, input.Width, input.Height)
	if err != nil {
		return ProductMedia{}, err
	}
	s.publishProductUpdatedByID(ctx, updated.ProductID, "media")
	updated.URL = s.media.PublicURL(updated.S3Key)
	return updated, nil
}

func (s *mediaService) GetProductMedia(ctx context.Context, id uuid.UUID) (ProductMedia, error) {
	m, err := s.repo.GetProductMediaByID(ctx, id)
	if err != nil {
		return ProductMedia{}, err
	}
	if s.media != nil {
		m.URL = s.media.PublicURL(m.S3Key)
	}
	return m, nil
}

func (s *mediaService) ListProductMedia(ctx context.Context, productID uuid.UUID) ([]ProductMedia, error) {
	items, err := s.repo.ListProductMediaByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	if s.media != nil {
		for i := range items {
			items[i].URL = s.media.PublicURL(items[i].S3Key)
		}
	}
	return items, nil
}

func (s *mediaService) UpdateProductMedia(ctx context.Context, input UpdateProductMediaInput) (ProductMedia, error) {
	m, err := s.repo.UpdateProductMediaMetadata(ctx, input)
	if err != nil {
		return ProductMedia{}, err
	}
	s.publishProductUpdatedByID(ctx, m.ProductID, "media")
	if s.media != nil {
		m.URL = s.media.PublicURL(m.S3Key)
	}
	return m, nil
}

// DeleteProductMedia removes the DB row immediately - When deleteFromStorage is true it also launches an async goroutine to delete the S3 object - If the S3 delete fails the orphan is logged - the DB row is already gone so manual cleanup (lifecycle rules, reconciliation cron) handles stragglers
func (s *mediaService) DeleteProductMedia(ctx context.Context, id uuid.UUID, deleteFromStorage bool) error {
	m, err := s.repo.GetProductMediaByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteProductMedia(ctx, id); err != nil {
		return err
	}
	s.publishProductUpdatedByID(ctx, m.ProductID, "media")
	if deleteFromStorage && s.media != nil {
		s.asyncDeleteS3(m.S3Key)
	}
	return nil
}

// DeleteProductMediaBulk removes multiple media rows belonging to a product - IDs that don't belong to the product are silently skipped - When deleteFromStorage is true the S3 objects are async-deleted - Returns the number of rows actually deleted
func (s *mediaService) DeleteProductMediaBulk(ctx context.Context, productID uuid.UUID, ids []uuid.UUID, deleteFromStorage bool) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	deleted, err := s.repo.DeleteProductMediaByIDs(ctx, productID, ids)
	if err != nil {
		return 0, err
	}
	if len(deleted) > 0 {
		s.publishProductUpdatedByID(ctx, productID, "media")
	}
	if deleteFromStorage && s.media != nil {
		for _, row := range deleted {
			s.asyncDeleteS3(row.S3Key)
		}
	}
	return len(deleted), nil
}

func (s *mediaService) asyncDeleteS3(key string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s3DeleteTimeout)
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				applog.From(ctx).Error("media: async s3 delete panicked", "key", key, "panic", fmt.Sprint(rec))
			}
		}()
		if err := s.media.Delete(ctx, key); err != nil {
			applog.From(ctx).Error("media: orphan s3 object after db delete", "key", key, "err", err)
		}
	}()
}

// ListProductMediaByProductIDs batch-fetches media for multiple products, returning a map keyed by product ID - Used by list resolvers to avoid N+1
func (s *mediaService) ListProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error) {
	byProduct, err := s.repo.ListProductMediaByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	if s.media != nil {
		for pid, items := range byProduct {
			for i := range items {
				items[i].URL = s.media.PublicURL(items[i].S3Key)
			}
			byProduct[pid] = items
		}
	}
	return byProduct, nil
}

// CleanupPendingMedia deletes pending media rows older than olderThan and async-removes their orphaned S3 objects - Called by the background ticker in main - errors are logged, not returned, so a single failed cleanup does not abort the ticker loop
func (s *mediaService) CleanupPendingMedia(ctx context.Context, olderThan time.Duration) {
	if s.media == nil {
		return
	}
	rows, err := s.repo.DeletePendingMediaOlderThan(ctx, time.Now().Add(-olderThan))
	if err != nil {
		applog.From(ctx).Error("media: cleanup delete pending rows", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	applog.From(ctx).Info("media: cleanup removed orphaned pending rows", "count", len(rows))
	// Bounded fan-out: a large sweep could otherwise open hundreds of S3 connections at once - Cap concurrency and wait for the batch so the work is visible to the caller's lifecycle (the cleanup ticker, which graceful shutdown drains) rather than leaking detached goroutines
	sem := make(chan struct{}, s3DeleteConcurrency)
	var wg sync.WaitGroup
	for _, row := range rows {
		key := row.S3Key
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if rec := recover(); rec != nil {
					applog.From(ctx).Error("media: prune delete panicked", "key", key, "panic", fmt.Sprint(rec))
				}
			}()
			// Detach from the ticker ctx's cancellation so an in-flight delete finishes, but keep its values and bound it with a hard timeout
			delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s3DeleteTimeout)
			defer cancel()
			if err := s.media.Delete(delCtx, key); err != nil {
				applog.From(delCtx).Error("media: cleanup orphan s3 delete failed", "key", key, "err", err)
			}
		}()
	}
	wg.Wait()
}

// ListProductMediaRolesByProductIDs batch-fetches role assignments for multiple products - Used by the list resolver to avoid N+1
func (s *mediaService) ListProductMediaRolesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]map[MediaRole]uuid.UUID, error) {
	return s.repo.GetMediaRolesByProductIDs(ctx, productIDs)
}

// SetProductMediaRole assigns a role (main, thumbnail, og_image) to a media item - The media must belong to the product and be in "ready" status
func (s *mediaService) SetProductMediaRole(ctx context.Context, productID, mediaID uuid.UUID, role MediaRole) (Product, error) {
	m, err := s.repo.GetProductMediaByID(ctx, mediaID)
	if err != nil {
		return Product{}, err
	}
	if m.ProductID != productID {
		return Product{}, ErrMediaWrongProduct
	}
	if m.Status != MediaStatusReady {
		return Product{}, fmt.Errorf("%w: media must be ready before assigning a role", ErrMediaNotPending)
	}
	if err := s.repo.SetMediaRole(ctx, productID, mediaID, role); err != nil {
		return Product{}, err
	}
	s.publishProductUpdatedByID(ctx, productID, "media")
	return s.repo.GetProductWithAttributes(ctx, productID)
}

// ClearProductMediaRole removes a role assignment from a product
func (s *mediaService) ClearProductMediaRole(ctx context.Context, productID uuid.UUID, role MediaRole) (Product, error) {
	if err := s.repo.ClearMediaRole(ctx, productID, role); err != nil {
		return Product{}, err
	}
	s.publishProductUpdatedByID(ctx, productID, "media")
	return s.repo.GetProductWithAttributes(ctx, productID)
}

// ReorderProductMedia rewrites sort_order in list order in one tx - a duplicate keeps its first position, a foreign id rejects the call, and media left out is untouched
func (s *mediaService) ReorderProductMedia(ctx context.Context, productID uuid.UUID, mediaIDs []uuid.UUID) ([]ProductMedia, error) {
	existing, err := s.repo.ListProductMediaByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	owned := make(map[uuid.UUID]struct{}, len(existing))
	for _, m := range existing {
		owned[m.ID] = struct{}{}
	}
	ordered := make([]uuid.UUID, 0, len(mediaIDs))
	seen := make(map[uuid.UUID]struct{}, len(mediaIDs))
	for _, id := range mediaIDs {
		if _, ok := owned[id]; !ok {
			return nil, fmt.Errorf("%w: media %s on product %s", ErrMediaWrongProduct, id, productID)
		}
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			ordered = append(ordered, id)
		}
	}
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for i, id := range ordered {
			if err := r.SetProductMediaSortOrder(ctx, id, int32(i)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.publishProductUpdatedByID(ctx, productID, "media")
	return s.ListProductMedia(ctx, productID)
}

func (s *mediaService) ListReadyProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error) {
	byProduct, err := s.repo.ListReadyProductMediaByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	if s.media != nil {
		for _, items := range byProduct {
			for i := range items {
				items[i].URL = s.media.PublicURL(items[i].S3Key)
			}
		}
	}
	return byProduct, nil
}
