package wiring

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// GET <this><asset id> hands an admin a library file's bytes, or with ?w=<pixels> a resized copy. It is the only way to a
// private file, and how the photo editor reads a public one, whose pixels another origin won't let a page read back
const mediaFilePath = "/api/admin/media/file/"

var mediaFileMaxWidth = media.AllowedWidths[len(media.AllowedWidths)-1]

type mediaFiles struct {
	open    func(ctx context.Context, id uuid.UUID) (io.ReadCloser, medialibrary.Asset, error)
	asset   func(ctx context.Context, id uuid.UUID) (medialibrary.Asset, error)
	resize  func(ctx context.Context, a medialibrary.Asset, width int) ([]byte, string, error)
	allowed func(ctx context.Context, a medialibrary.Asset) error
}

// Each half of the library resizes out of, and caches into, its own store - a private picture's copies stay private
func mediaFileHandler(lib *medialibrary.Service, adminSvc auth.ScopeAwareChecker, public, private *media.Transformer) http.Handler {
	serve := serveMediaFile(mediaFiles{
		open:  lib.Open,
		asset: lib.GetAnyAsset,
		allowed: func(ctx context.Context, a medialibrary.Asset) error {
			return medialibrary.RequireHalf(ctx, adminSvc, medialibrary.PermPrivateView, a.Visibility)
		},
		resize: func(ctx context.Context, a medialibrary.Asset, width int) ([]byte, string, error) {
			t := public
			if a.Visibility == medialibrary.Private {
				t = private
			}
			if t == nil {
				return nil, "", medialibrary.ErrNoPrivateStore
			}
			return t.Transform(ctx, a.ObjectKey, width, 0, 0.5, 0.5, media.CanEncodeWebP)
		},
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.RequirePermission(r.Context(), adminSvc, "media.view"); err != nil {
			refuse(w, err)
			return
		}
		serve(w, r)
	})
}

func serveMediaFile(files mediaFiles) http.HandlerFunc {
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
		width := 0
		if raw := r.URL.Query().Get("w"); raw != "" {
			if width, err = strconv.Atoi(raw); err != nil || width <= 0 || width > mediaFileMaxWidth {
				http.Error(w, "w must be a width in pixels", http.StatusBadRequest)
				return
			}
		}
		asset, err := files.asset(r.Context(), id)
		switch {
		case errors.Is(err, medialibrary.ErrAssetNotFound):
			http.Error(w, "not found", http.StatusNotFound)
			return
		case err != nil:
			slog.Error("media file", "asset", id, "err", err)
			http.Error(w, "could not read the file", http.StatusBadGateway)
			return
		}
		if err := files.allowed(r.Context(), asset); err != nil {
			refuse(w, err)
			return
		}
		// Uploaded bytes on the API's own origin: never sniffed into or run as anything else, and no copy kept by the browser
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("Cache-Control", "private, no-store")
		if width > 0 {
			serveResized(w, r, files, asset, width)
			return
		}

		body, opened, err := files.open(r.Context(), id)
		switch {
		case errors.Is(err, medialibrary.ErrAssetNotFound), err == nil && opened.Visibility != asset.Visibility:
			if err == nil {
				_ = body.Close() // it changed halves since the check, so the check no longer stands for it
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		case err != nil:
			slog.Error("media file", "asset", id, "err", err)
			http.Error(w, "could not read the file", http.StatusBadGateway)
			return
		}
		defer func() { _ = body.Close() }()
		h.Set("Content-Type", asset.MimeType)
		h.Set("Content-Disposition", attachment(sanitizeFilename(asset.Filename)))
		_, _ = io.Copy(w, body)
	}
}

func serveResized(w http.ResponseWriter, r *http.Request, files mediaFiles, asset medialibrary.Asset, width int) {
	if asset.Status != medialibrary.AssetStatusReady || !media.CanResize(asset.MimeType) {
		http.Error(w, "that file has no smaller copy", http.StatusUnsupportedMediaType)
		return
	}
	data, contentType, err := files.resize(r.Context(), asset, width)
	if err != nil {
		slog.Error("media file resize", "asset", asset.ID, "err", err)
		http.Error(w, "could not make a smaller copy", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
