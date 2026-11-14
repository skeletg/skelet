package wiring

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// GET <this>/<submission id>/files/<field key> - a shopper's upload, which lives in the private store, so this route is the only way to it
const webformFilePath = "/api/admin/webforms/submissions/"

type webformFiles interface {
	GetSubmission(ctx context.Context, id uuid.UUID) (webform.Submission, error)
	Get(ctx context.Context, id uuid.UUID) (webform.Form, error)
	OpenSubmissionFile(ctx context.Context, sub webform.Submission, fieldKey string) (io.ReadCloser, webform.SubmissionFile, error)
}

func webformFileHandler(svc *webform.Service, adminSvc *admin.Service) http.Handler {
	serve := serveWebformFile(svc, func(r *http.Request, channelID uuid.UUID) error {
		return auth.RequireChannelPermission(r.Context(), adminSvc, "webforms.submissions_view", channelID, nil)
	})
	// Checked before the submission is looked up, so a caller with no session learns nothing of which ids exist
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.RequirePermission(r.Context(), adminSvc, "webforms.submissions_view"); err != nil {
			refuse(w, err)
			return
		}
		serve(w, r)
	})
}

func unknownOrFailed(w http.ResponseWriter, err, missing error) {
	if errors.Is(err, missing) {
		forbidden(w)
		return
	}
	slog.Error("form upload lookup", "err", err)
	http.Error(w, "could not look that up", http.StatusBadGateway)
}

func serveWebformFile(svc webformFiles, allowed func(r *http.Request, channelID uuid.UUID) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rawID, fieldKey, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, webformFilePath), "/files/")
		id, err := uuid.Parse(rawID)
		if !ok || err != nil || fieldKey == "" || strings.Contains(fieldKey, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// One answer for "no such submission" and "not yours to see", so ids can't be told apart by probing
		sub, err := svc.GetSubmission(r.Context(), id)
		if err != nil {
			unknownOrFailed(w, err, webform.ErrSubmissionNotFound)
			return
		}
		form, err := svc.Get(r.Context(), sub.FormID)
		if err != nil {
			unknownOrFailed(w, err, webform.ErrFormNotFound)
			return
		}
		if err := allowed(r, form.ChannelID); err != nil {
			refuse(w, err)
			return
		}
		body, file, err := svc.OpenSubmissionFile(r.Context(), sub, fieldKey)
		switch {
		case errors.Is(err, webform.ErrFileNotFound):
			http.Error(w, "that file isn't there", http.StatusNotFound)
			return
		case err != nil:
			slog.Error("form upload", "submission", id, "field", fieldKey, "err", err)
			http.Error(w, "could not read the file", http.StatusBadGateway)
			return
		}
		defer body.Close()

		contentType := file.MimeType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		h := w.Header()
		h.Set("Content-Type", contentType)
		h.Set("Content-Disposition", attachment(sanitizeFilename(file.Name)))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("Cache-Control", "private, no-store")
		_, _ = io.Copy(w, body)
	}
}
