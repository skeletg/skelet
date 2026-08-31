package plugin

// Admin-UI language packs, registered via sdk.RegisterAdminTranslations and served to the
// dashboard over two public endpoints (mounted in wiring under /api/admin-ui/i18n/):
//
//	GET .../locales          the installed languages, for the picker
//	GET .../catalog/<locale> that language's message catalog, ETag-revalidated
//
// Public on purpose: the login screen needs its strings before any session exists, and a
// catalog holds nothing but UI copy. Catalogs only change on deploy, so no-cache + ETag
// makes every repeat load a 304

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

type adminTranslation struct {
	pluginSlug  string
	locale      string
	displayName string
	direction   string
	messages    json.RawMessage
	etag        string
}

// Package-level like healthRegistry - the handler serves requests long after every init()
var adminTranslationsRegistry = struct {
	mu       sync.Mutex
	byLocale map[string]adminTranslation
}{byLocale: map[string]adminTranslation{}}

// The SDK already validated shape; what only the host can see is a collision between packs
func (h *Host) RegisterAdminTranslations(slug string, reg sdk.AdminTranslationsRegistration) error {
	r := &adminTranslationsRegistry
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.byLocale[reg.Locale]; ok {
		return fmt.Errorf("admin translations for %q already registered by plugin %q", reg.Locale, prev.pluginSlug)
	}
	dir := reg.Direction
	if dir == "" {
		dir = "ltr"
	}
	sum := sha256.Sum256(reg.Messages)
	r.byLocale[reg.Locale] = adminTranslation{
		pluginSlug:  slug,
		locale:      reg.Locale,
		displayName: reg.DisplayName,
		direction:   dir,
		messages:    reg.Messages,
		etag:        `"` + hex.EncodeToString(sum[:16]) + `"`,
	}
	return nil
}

type adminLocaleInfo struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Direction   string `json:"direction"`
	Plugin      string `json:"plugin"`
	MessageETag string `json:"messageEtag"`
}

// AdminTranslationsHandler serves the two endpoints above; mount it at a prefix ending in
// /i18n/ so the trailing segments route inside
func AdminTranslationsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, rest, ok := strings.Cut(r.URL.Path, "/i18n/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch {
		case rest == "locales":
			serveAdminLocales(w)
		case strings.HasPrefix(rest, "catalog/"):
			serveAdminCatalog(w, r, strings.TrimPrefix(rest, "catalog/"))
		default:
			http.NotFound(w, r)
		}
	})
}

func serveAdminLocales(w http.ResponseWriter) {
	r := &adminTranslationsRegistry
	r.mu.Lock()
	list := make([]adminLocaleInfo, 0, len(r.byLocale))
	for _, t := range r.byLocale {
		list = append(list, adminLocaleInfo{
			Locale:      t.locale,
			Name:        t.displayName,
			Direction:   t.direction,
			Plugin:      t.pluginSlug,
			MessageETag: t.etag,
		})
	}
	r.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Locale < list[j].Locale })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]any{"locales": list})
}

func serveAdminCatalog(w http.ResponseWriter, req *http.Request, locale string) {
	r := &adminTranslationsRegistry
	r.mu.Lock()
	t, ok := r.byLocale[locale]
	r.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", t.etag)
	if req.Header.Get("If-None-Match") == t.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(t.messages)
}
