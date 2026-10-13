package download

import (
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// The object storage a grant's file may live in - a file there is streamed, so its URL never reaches the buyer and the use cap holds
type FileStore interface {
	// The storage key behind a URL this store serves, false for anything hosted elsewhere
	KeyFor(url string) (string, bool)
	Open(ctx context.Context, key string) (body io.ReadCloser, size int64, contentType string, err error)
}

// Serves /download/<token>: one use per hit, a stored file streamed as an attachment, an external one redirected to
// files may be nil, and allow is the per-request rate gate
func (s *Service) Handler(files FileStore, allow func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/download/")
		if token == "" || strings.Contains(token, "/") {
			http.NotFound(w, r)
			return
		}
		if allow != nil && !allow(r) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		grant, err := s.Get(r.Context(), token)
		if err != nil {
			writeGrantError(w, r, err)
			return
		}
		var key string
		stored := false
		if files != nil {
			key, stored = files.KeyFor(grant.URL)
		}
		if !stored {
			if _, err := s.Consume(r.Context(), token); err != nil {
				writeGrantError(w, r, err)
				return
			}
			http.Redirect(w, r, grant.URL, http.StatusFound)
			return
		}
		// Opened before the use is spent, so a missing file doesn't cost the buyer one of their downloads
		body, size, contentType, err := files.Open(r.Context(), key)
		if err != nil {
			log.Printf("[download] open %s for grant %s: %v", key, grant.ID, err)
			http.Error(w, "file unavailable", http.StatusBadGateway)
			return
		}
		defer func() { _ = body.Close() }()
		if _, err := s.Consume(r.Context(), token); err != nil {
			writeGrantError(w, r, err)
			return
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		h := w.Header()
		h.Set("Content-Type", contentType)
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(key)}))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "private, no-store")
		if size > 0 {
			h.Set("Content-Length", strconv.FormatInt(size, 10))
		}
		if _, err := io.Copy(w, body); err != nil {
			log.Printf("[download] stream grant %s: %v", grant.ID, err)
		}
	})
}

func writeGrantError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, ErrExpired) || errors.Is(err, ErrLimitReached):
		http.Error(w, err.Error(), http.StatusGone)
	default:
		log.Printf("[download] %v", err)
		http.Error(w, "download error", http.StatusInternalServerError)
	}
}
