package objstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"
)

type uploadGrant struct {
	Key         string `json:"k"`
	ContentType string `json:"c"`
	Size        int64  `json:"s,omitempty"`
	Expires     int64  `json:"e"`
}

func (s *FileStore) PresignUpload(_ context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error) {
	if s.cfg.SigningKey == "" {
		return "", fmt.Errorf("media: presign upload: STORAGE_SIGNING_KEY is not set")
	}
	if s.cfg.PresignEndpoint == "" {
		return "", fmt.Errorf("media: presign upload: STORAGE_UPLOAD_URL is not set, so a browser has nowhere to upload to")
	}
	if err := checkKey(key); err != nil {
		return "", fmt.Errorf("media: presign upload: %w", err)
	}
	body, err := json.Marshal(uploadGrant{Key: key, ContentType: contentType, Size: sizeBytes, Expires: time.Now().Add(ttl).Unix()})
	if err != nil {
		return "", fmt.Errorf("media: presign upload: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	return s.cfg.PresignEndpoint + "/" + escapeKey(key) + "?t=" + payload + "." + s.sign(payload), nil
}

func (s *FileStore) sign(payload string) string {
	m := hmac.New(sha256.New, []byte(s.cfg.SigningKey))
	m.Write([]byte("objstore-upload\x00" + payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

var errBadGrant = errors.New("objstore: upload URL is invalid or expired")

func (s *FileStore) verify(token string, now time.Time) (uploadGrant, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || s.cfg.SigningKey == "" || !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return uploadGrant{}, errBadGrant
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return uploadGrant{}, errBadGrant
	}
	var g uploadGrant
	if json.Unmarshal(body, &g) != nil || now.Unix() > g.Expires || checkKey(g.Key) != nil {
		return uploadGrant{}, errBadGrant
	}
	return g, nil
}

// Handler is mounted at FilesPath on core - one path per object, like S3, so an upload URL is the object's own plus a token
func (s *FileStore) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.URL.Path, FilesPath)
		switch {
		case !ok:
			http.NotFound(w, r)
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			s.serveObject(w, r, key)
		case r.Method == http.MethodPut:
			s.serveUpload(w, r, key)
		default:
			w.Header().Set("Allow", "GET, HEAD, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *FileStore) serveUpload(w http.ResponseWriter, r *http.Request, key string) {
	g, err := s.verify(r.URL.Query().Get("t"), time.Now())
	if err != nil || g.Key != key {
		http.Error(w, errBadGrant.Error(), http.StatusForbidden)
		return
	}
	if r.Header.Get("Content-Type") != g.ContentType {
		http.Error(w, "Content-Type does not match the signed upload", http.StatusForbidden)
		return
	}
	want := g.Size
	if want <= 0 {
		want = -1
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	}
	if err := s.write(g.Key, g.ContentType, r.Body, want); err != nil {
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig || errors.Is(err, errSizeMismatch) {
			http.Error(w, "body size does not match the signed upload", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Uploaded files share core's origin, so nothing in them may run there - non-media types download instead of rendering
func (s *FileStore) serveObject(w http.ResponseWriter, r *http.Request, key string) {
	f, err := s.open(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := s.contentType(key)
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "public, max-age=3600")
	if !inlineSafe(ct) {
		h.Set("Content-Disposition", "attachment")
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func inlineSafe(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case mt == "image/svg+xml":
		return false
	case strings.HasPrefix(mt, "image/"), strings.HasPrefix(mt, "video/"), strings.HasPrefix(mt, "audio/"), mt == "application/pdf":
		return true
	}
	return false
}
