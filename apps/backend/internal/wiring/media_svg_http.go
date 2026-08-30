package wiring

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/skeletg/skelet/internal/admin"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/media"
	"github.com/skeletg/skelet/internal/medialibrary"
)

// SVG skips the GraphQL presign every other format uses, because the bytes must pass THROUGH the
// server to be sanitized - see media.SanitizeSVG and medialibrary.UploadSVG for the reasoning
const mediaSVGPath = "/api/admin/media/svg"

const maxSVGUpload = 2 << 20 // generous for a logo, and the storage cap still applies underneath

func mediaSVGHandler(lib *medialibrary.Service, adminSvc *admin.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// the same grant the GraphQL upload mutation wants - this is another door to one action
		if err := auth.RequirePermission(r.Context(), adminSvc, "media.manage"); err != nil {
			forbidden(w)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxSVGUpload)
		if err := r.ParseMultipartForm(64 << 10); err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, "that SVG is over the 2 MB limit", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "expected a multipart upload", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file in the upload", http.StatusBadRequest)
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "could not read the upload", http.StatusBadRequest)
			return
		}

		asset, err := lib.UploadSVG(r.Context(), medialibrary.UploadSVGInput{
			Path:     r.FormValue("path"),
			Filename: header.Filename,
			Data:     raw,
			Title:    strings.TrimSpace(r.FormValue("title")),
			AltText:  strings.TrimSpace(r.FormValue("altText")),
		})
		if err != nil {
			// a rejected SVG describes the uploader's own file, so the reason is safe to echo back
			if errors.Is(err, medialibrary.ErrInvalidMimeType) || errors.Is(err, media.ErrNotSVG) ||
				errors.Is(err, media.ErrSVGUnsanitized) || errors.Is(err, medialibrary.ErrFileTooLarge) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": svgRejectionMessage(err)})
				return
			}
			slog.Error("svg upload", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store the file"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":  asset.ID.String(),
			"url": asset.URL,
		})
	})
}

func svgRejectionMessage(err error) string {
	switch {
	case errors.Is(err, medialibrary.ErrFileTooLarge):
		return "that SVG is too large"
	case errors.Is(err, media.ErrNotSVG):
		return "that file is not an SVG"
	default:
		return "that SVG could not be read - re-export it as plain SVG and try again"
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
