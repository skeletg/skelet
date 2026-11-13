package wiring

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The photo editor reads a picture back off a canvas, which a browser refuses for one from another origin unless that origin allows it - so the original comes from here, not from storage
const mediaFilePath = "/api/admin/media/file/"

type mediaFileOpener func(r *http.Request, id uuid.UUID) (io.ReadCloser, medialibrary.Asset, error)

func mediaFileHandler(lib *medialibrary.Service, adminSvc *admin.Service) http.Handler {
	serve := serveMediaFile(func(r *http.Request, id uuid.UUID) (io.ReadCloser, medialibrary.Asset, error) {
		return lib.OpenForEditing(r.Context(), id)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.RequirePermission(r.Context(), adminSvc, "media.view"); err != nil {
			forbidden(w)
			return
		}
		serve(w, r)
	})
}

func serveMediaFile(open mediaFileOpener) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, mediaFilePath))
		if err != nil {
			http.Error(w, "invalid asset id", http.StatusBadRequest)
			return
		}
		body, asset, err := open(r, id)
		switch {
		case errors.Is(err, medialibrary.ErrAssetNotFound):
			http.Error(w, "not found", http.StatusNotFound)
			return
		case errors.Is(err, medialibrary.ErrNotEditable):
			http.Error(w, "that file can't be edited", http.StatusUnsupportedMediaType)
			return
		case err != nil:
			slog.Error("media file", "asset", id, "err", err)
			http.Error(w, "could not read the file", http.StatusBadGateway)
			return
		}
		defer body.Close()

		// Uploaded bytes on the API's own origin: a download that can't be sniffed into, or run as, anything else
		h := w.Header()
		h.Set("Content-Type", asset.MimeType)
		h.Set("Content-Disposition", attachment(sanitizeFilename(asset.Filename)))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("Cache-Control", "private, no-store")
		_, _ = io.Copy(w, body)
	}
}
