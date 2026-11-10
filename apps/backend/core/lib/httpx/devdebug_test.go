package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheDevDebugWriterLetsAStreamFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	DevDebugMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if !rec.Flushed || rec.Header().Get("X-DB-Queries") == "" {
		t.Errorf("flushed %v with headers %v", rec.Flushed, rec.Header())
	}
}
