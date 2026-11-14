package wiring

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// POST {"path": "<a file route, with its query>"} answers {"url": "<the same>&ticket=..."}, a link the admin's browser can
// open to save that file
const downloadTicketPath = "/api/admin/download-ticket"

const downloadTicketTTL = time.Minute

var ticketRoutes = []string{mediaFilePath, dataJobsPath, dataTemplatePath}

func downloadTicketHandler(keys auth.AdminKeys) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// A person's browser is what opens the link - a token's caller has its own way to the file
		claims, err := auth.RequireAdmin(r.Context())
		if err != nil {
			refuse(w, err)
			return
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			http.Error(w, "expected {\"path\": ...}", http.StatusBadRequest)
			return
		}
		target, err := url.Parse(in.Path)
		if err != nil || target.Scheme != "" || target.Host != "" || path.Clean(target.Path) != strings.TrimSuffix(target.Path, "/") || !ticketable(target.Path) {
			http.Error(w, "that isn't a file the dashboard downloads", http.StatusBadRequest)
			return
		}
		ticket, err := auth.GenerateDownloadTicket(claims, target.Path, keys, downloadTicketTTL)
		if err != nil {
			slog.Error("download ticket", "err", err)
			http.Error(w, "couldn't make a download link", http.StatusInternalServerError)
			return
		}
		q := target.Query()
		q.Set("ticket", ticket)
		target.RawQuery = q.Encode()
		writeJSON(w, http.StatusOK, map[string]string{"url": target.String()})
	}
}

func ticketable(p string) bool {
	for _, route := range ticketRoutes {
		under, isPrefix := strings.CutPrefix(p, route)
		if strings.HasSuffix(route, "/") && isPrefix && under != "" || p == route && !strings.HasSuffix(route, "/") {
			return true
		}
	}
	return false
}
