// Package mediahttp serves the image-transform endpoint - its own package because two binaries mount it, cmd/server for a single-process deploy and cmd/media for the standalone image tier, which needs only object storage and a focal lookup
package mediahttp

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/internal/media"
)

// The focal point (0..1) for an original key, only hit for cover crops that miss the cache, so the common warm or fit request costs no lookup - false means centre it
type FocalResolver func(r *http.Request, key string) (x, y float64, ok bool)

type Options struct {
	Prefix             string                     // mount prefix incl trailing slash, default "/media/t/"
	Focal              FocalResolver              // per-asset focal points, nil = always centre
	AllowRateLimit     func(r *http.Request) bool // may the request proceed, nil = unlimited
	ClientCacheSeconds int                        // max-age on generated responses, default 30d
	// The max-age on a warm-hit redirect, shorter than the object's own lifetime so replacing an original can't be masked by a long-lived 302 - defaults to 1h
	RedirectCacheSeconds int
}

// Serves /media/t/w640/<key> for fit and w640h640/<key> for a focal cover crop - warm derivatives get a redirect to object storage rather than bytes through this process, so only a cold miss touches image bytes
// With no public read base there's nowhere to redirect, so it streams instead
func NewTransformHandler(t *media.Transformer, opts Options) http.Handler {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "/media/t/"
	}
	clientCache := opts.ClientCacheSeconds
	if clientCache <= 0 {
		clientCache = 2592000 // 30d
	}
	redirectCache := opts.RedirectCacheSeconds
	if redirectCache <= 0 {
		redirectCache = 3600 // 1h
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.AllowRateLimit != nil && !opts.AllowRateLimit(r) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		spec, key, ok := parseTransformPath(prefix, r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		// Format is negotiated, not in the URL - one URL per derivative, webp for clients that advertise it, jpeg/png for the rest (old crawlers, Outlook's renderer)
		webp := media.CanEncodeWebP && acceptsWebP(r)
		if media.CanEncodeWebP {
			// On every cacheable response, hit or miss - a shared cache serving the miss path a webp would break non-webp clients
			w.Header().Set("Vary", "Accept")
		}

		// Warm hit - the client goes straight to object storage, so one HEAD, no body through this process, no DB lookup
		if url := t.WarmURL(r.Context(), key, spec.w, spec.h, webp); url != "" {
			w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(redirectCache))
			http.Redirect(w, r, url, http.StatusFound)
			return
		}

		// Cold miss - focal only affects cover crops, so a fit request skips the lookup
		focalX, focalY := 0.5, 0.5
		if spec.h > 0 && opts.Focal != nil {
			if x, y, ok := opts.Focal(r, key); ok {
				focalX, focalY = x, y
			}
		}

		data, ctype, err := t.Transform(r.Context(), key, spec.w, spec.h, focalX, focalY, webp)
		switch {
		case err == nil:
		case errors.Is(err, media.ErrSourceTooLarge):
			// Not retryable - the original itself is out of bounds
			slog.Warn("media transform rejected", "key", key, "err", err)
			http.Error(w, "source image too large", http.StatusUnprocessableEntity)
			return
		case errors.Is(err, media.ErrBusy):
			w.Header().Set("Retry-After", "2")
			http.Error(w, "image render capacity saturated", http.StatusServiceUnavailable)
			return
		case r.Context().Err() != nil:
			// Client went away mid-render, nothing to say
			return
		case errors.Is(err, media.ErrObjectNotFound):
			slog.Warn("media transform: original missing", "key", key)
			http.NotFound(w, r)
			return
		default:
			// A storage outage answered 404, so the CDN cached "this image doesn't exist" and the picture stayed
			// gone long after the store came back - 502 with no-store keeps the failure uncacheable
			slog.Error("media transform failed", "key", key, "w", spec.w, "h", spec.h, "err", err)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "image unavailable", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(clientCache)+", immutable")
		_, _ = w.Write(data)
	})
}

// Presence beats q-value parsing here - no browser sends image/webp with q=0, and image Accept headers list only what the client renders
func acceptsWebP(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "image/webp")
}

type transformSpec struct{ w, h int }

// Splits "/media/t/w640h640/some/key.jpg" into spec plus storage key - rejects malformed specs and derived/ keys, so no derivatives of derivatives
func parseTransformPath(prefix, p string) (transformSpec, string, bool) {
	rest, ok := strings.CutPrefix(p, prefix)
	if !ok {
		return transformSpec{}, "", false
	}
	specStr, key, ok := strings.Cut(rest, "/")
	if !ok || key == "" || strings.HasPrefix(key, "derived/") || strings.Contains(key, "..") {
		return transformSpec{}, "", false
	}
	var spec transformSpec
	s, ok := strings.CutPrefix(specStr, "w")
	if !ok {
		return transformSpec{}, "", false
	}
	wPart, hPart, hasH := strings.Cut(s, "h")
	wv, err := strconv.Atoi(wPart)
	if err != nil || wv <= 0 {
		return transformSpec{}, "", false
	}
	spec.w = wv
	if hasH {
		hv, err := strconv.Atoi(hPart)
		if err != nil || hv <= 0 {
			return transformSpec{}, "", false
		}
		spec.h = hv
	}
	return spec, key, true
}

// The liveness handler for the standalone media tier - it answers without touching object storage, so a storage blip can't get the container restarted out from under a working derivative cache
func Health() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
}
