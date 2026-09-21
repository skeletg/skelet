package wiring

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Files move over HTTP rather than GraphQL: a multipart upload streams straight into the object store, and a
// download streams straight back out, neither ever becoming a base64 string in a JSON document

const dataImportPath = "/api/admin/data/import" // exempt from the global body cap in http.go - that cap is sized for JSON payloads, and this store's own export is well past it

const maxImportUpload = 200 << 20 // bigger than any sane spreadsheet, small enough that a mis-click can't fill the disk

const multipartOverhead = 1 << 20 // headroom for the part headers and boundaries wrapping the file itself

var errUploadTooLarge = errors.New("upload exceeds the size limit")

// Fails past the cap rather than truncating - io.LimitReader would hand the importer a file cut mid-row
func capped(r io.Reader, limit int64) io.Reader {
	return io.MultiReader(io.LimitReader(r, limit), failAfter{r})
}

type failAfter struct{ r io.Reader }

func (f failAfter) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if n > 0 {
		return 0, errUploadTooLarge
	}
	if errors.Is(err, io.EOF) {
		return 0, io.EOF
	}
	return 0, err
}

func forbidden(w http.ResponseWriter) { http.Error(w, "forbidden", http.StatusForbidden) }

func holdsAnywhere(r *http.Request, adminSvc *admin.Service, slug string) bool {
	return auth.HoldsAnywhere(r.Context(), adminSvc, slug)
}

func requireTransferPermission(r *http.Request, adminSvc *admin.Service, slug string, channelID uuid.UUID, global bool) error {
	return auth.RequireScopedOrGlobal(r.Context(), adminSvc, slug, channelID, global)
}

func dataImportHandler(svc *datajob.Service, adminSvc *admin.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := auth.RequirePermission(r.Context(), adminSvc, "data.view"); err != nil {
			forbidden(w)
			return
		}
		// This route is exempt from the global body cap, so bound it here: ParseMultipartForm spills non-file parts past its memory budget to disk, and without a ceiling a request made entirely of form FIELDS (no file part, so `capped` never sees it) fills the disk
		r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload+multipartOverhead)
		// Only the non-file parts are buffered - the file itself stays a stream, so a 200MB upload is not 200MB of RAM
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, fmt.Sprintf("that upload is over the %d MB limit - split it", maxImportUpload>>20), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "expected a multipart upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		slug := r.FormValue("resource")
		res, err := svc.Registry().Get(slug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !res.CanImport() {
			http.Error(w, fmt.Sprintf("%s cannot be imported", res.Label), http.StatusBadRequest)
			return
		}
		channelID := uuid.Nil
		if raw := strings.TrimSpace(r.FormValue("channelId")); raw != "" {
			if channelID, err = uuid.Parse(raw); err != nil {
				http.Error(w, "invalid channelId", http.StatusBadRequest)
				return
			}
		}
		if err := requireTransferPermission(r, adminSvc, res.ImportPermission, channelID, res.RequireGlobalPermission); err != nil {
			forbidden(w)
			return
		}
		claims, err := auth.RequireAuth(r.Context())
		if err != nil {
			forbidden(w)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file in the upload", http.StatusBadRequest)
			return
		}
		defer file.Close()

		format := dataio.Format(strings.TrimSpace(r.FormValue("format")))
		if format == "" {
			format = formatFromFilename(header.Filename)
		}
		job, err := svc.QueueImport(r.Context(), datajob.ImportRequest{
			Resource:  slug,
			Format:    format,
			Options:   formOptions(res, r),
			ChannelID: channelID,
			Locale:    strings.TrimSpace(r.FormValue("locale")),
			AdminID:   claims.AdminID,
			DryRun:    isTruthy(r.FormValue("dryRun")),
			Filename:  header.Filename,
		}, capped(file, maxImportUpload))
		if errors.Is(err, errUploadTooLarge) {
			http.Error(w, fmt.Sprintf("that file is over the %d MB limit - split it", maxImportUpload>>20), http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": job.ID.String(), "status": string(job.Status)})
	})
}

// Options arrive as opt.<name> form fields, so a resource declaring a new one needs no change here
func formOptions(res dataio.Resource, r *http.Request) map[string]string {
	out := map[string]string{}
	for _, def := range res.Options {
		if v := strings.TrimSpace(r.FormValue("opt." + def.Name)); v != "" {
			out[def.Name] = v
		}
	}
	return out
}

func formatFromFilename(name string) dataio.Format {
	for _, c := range dataio.Formats() {
		if strings.HasSuffix(strings.ToLower(name), "."+c.Extension()) {
			return c.Format()
		}
	}
	return dataio.FormatCSV
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Streamed through the app rather than presigned - a whole-customer-table CSV must not sit behind a link that outlives its session
func dataDownloadHandler(svc *datajob.Service, adminSvc *admin.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.RequirePermission(r.Context(), adminSvc, "data.view"); err != nil {
			forbidden(w)
			return
		}
		id, err := uuid.Parse(jobIDFromPath(r.URL.Path))
		if err != nil {
			http.Error(w, "invalid job id", http.StatusBadRequest)
			return
		}
		job, err := svc.Get(r.Context(), id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		res, err := svc.Registry().Get(job.Resource)
		if err != nil {
			forbidden(w)
			return
		}
		if err := requireTransferPermission(r, adminSvc, res.ViewPermission, job.ChannelID, res.RequireGlobalPermission); err != nil {
			forbidden(w)
			return
		}
		body, _, err := svc.OpenArtifact(r.Context(), id)
		if err != nil {
			http.Error(w, "that file has passed its retention window", http.StatusNotFound)
			return
		}
		defer body.Close()

		contentType := "application/octet-stream"
		if codec, cerr := dataio.CodecFor(job.Format); cerr == nil {
			contentType = codec.ContentType()
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(job.Filename)+`"`)
		w.Header().Set("Cache-Control", "no-store")
		// A mid-stream error can no longer change the status line - the truncated file is the signal
		_, _ = io.Copy(w, body)
	})
}

// A header-only file with one example row, so an admin never has to guess the columns
func dataTemplateHandler(svc *datajob.Service, adminSvc *admin.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.RequirePermission(r.Context(), adminSvc, "data.view"); err != nil {
			forbidden(w)
			return
		}
		slug := r.URL.Query().Get("resource")
		res, err := svc.Registry().Get(slug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !res.CanImport() {
			http.Error(w, fmt.Sprintf("%s cannot be imported", res.Label), http.StatusBadRequest)
			return
		}
		if !holdsAnywhere(r, adminSvc, res.ImportPermission) {
			forbidden(w)
			return
		}
		format := dataio.Format(r.URL.Query().Get("format"))
		if format == "" {
			format = dataio.FormatCSV
		}
		codec, err := dataio.CodecFor(format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", codec.ContentType())
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-template.%s"`, slug, codec.Extension()))
		w.Header().Set("Cache-Control", "no-store")
		_ = svc.Registry().Template(r.Context(), slug, format, w)
	})
}

func jobIDFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "jobs" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// Quotes and control characters in a Content-Disposition let a crafted filename forge extra headers
func sanitizeFilename(name string) string {
	if name == "" {
		return "export.csv"
	}
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == '"' || r == '\\' || r == '/' {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
