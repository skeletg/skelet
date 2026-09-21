package catalog

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (s *downloadService) AddProductDownload(ctx context.Context, input AddProductDownloadInput) (ProductDownload, error) {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.URL) == "" {
		return ProductDownload{}, ErrProductDownloadInvalid
	}
	return s.repo.CreateProductDownload(ctx, input)
}

// Feeds admin reads and the download-grant bridge, which needs to know what to grant at order placement
func (s *downloadService) ListProductDownloads(ctx context.Context, productID uuid.UUID) ([]ProductDownload, error) {
	return s.repo.ListProductDownloadsByProductID(ctx, productID)
}

func (s *downloadService) GetProductDownload(ctx context.Context, id uuid.UUID) (ProductDownload, error) {
	return s.repo.GetProductDownloadByID(ctx, id)
}

func (s *downloadService) UpdateProductDownload(ctx context.Context, input UpdateProductDownloadInput) (ProductDownload, error) {
	cur, err := s.repo.GetProductDownloadByID(ctx, input.ID)
	if err != nil {
		return ProductDownload{}, err
	}
	title := cur.Title
	if input.Title != nil {
		title = *input.Title
	}
	url := cur.URL
	if input.URL != nil {
		url = *input.URL
	}
	maxDownloads := cur.MaxDownloads
	if input.MaxDownloads != nil {
		maxDownloads = input.MaxDownloads
	}
	sortOrder := cur.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(url) == "" {
		return ProductDownload{}, ErrProductDownloadInvalid
	}
	return s.repo.UpdateProductDownload(ctx, cur.ID, title, url, maxDownloads, sortOrder)
}

func (s *downloadService) DeleteProductDownload(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetProductDownloadByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteProductDownload(ctx, id)
}
