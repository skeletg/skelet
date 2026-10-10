package mediahttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

type oneObject struct {
	key  string
	body []byte
}

func (o oneObject) GetBytes(_ context.Context, key string) ([]byte, error) {
	if key == o.key {
		return o.body, nil
	}
	return nil, objstore.ErrObjectNotFound
}
func (o oneObject) GetBytesTyped(context.Context, string) ([]byte, string, error) {
	return nil, "", objstore.ErrObjectNotFound
}
func (o oneObject) PutBytes(context.Context, string, []byte, string) error { return nil }
func (o oneObject) Head(context.Context, string) (int64, string, bool, error) {
	return 0, "", false, nil
}
func (o oneObject) HasPublicBase() bool       { return false }
func (o oneObject) PublicURL(k string) string { return k }

// A HEIC original is there - this build just can't draw it, so it is not a storage fault to 502 on every request
func TestAnUndrawableOriginalAnswers415(t *testing.T) {
	store := oneObject{key: "p/photo.heic", body: []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic")}
	h := NewTransformHandler(media.NewTransformer(store), Options{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/t/w320/p/photo.heic", nil))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d, want 415", rec.Code)
	}
}
