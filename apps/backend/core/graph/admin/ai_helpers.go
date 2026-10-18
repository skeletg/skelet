package admin // free helpers for ai.resolvers.go - kept out of the resolvers file so gqlgen doesn't evict them on regeneration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The backend fetches the bytes itself because Anthropic can't reach internal/dev object-storage URLs
func fetchImageBytes(ctx context.Context, url string) ([]byte, error) {
	const maxBytes = 5 << 20 // 5 MiB
	cl := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	if len(b) > maxBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxBytes)
	}
	return b, nil
}
