package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// exec.StdoutPipe hands back an *os.File that ADVERTISES Seek but returns ESPIPE, and the S3 upload manager prefers seeking when the body claims to support it
// UploadStream must strip the body down to Read - this uses a real os.Pipe, the exact trigger shape
func TestUploadStreamAcceptsNonSeekablePipe(t *testing.T) {
	var gotBytes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The manager may PUT directly (small stream) or do a multipart dance - accept everything, count PUT payload bytes
		if r.Method == http.MethodPut {
			buf := make([]byte, 32*1024)
			for {
				n, err := r.Body.Read(buf)
				gotBytes += n
				if err != nil {
					break
				}
			}
			w.Header().Set("ETag", `"test-etag"`)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store, err := New(context.Background(), Config{
		Bucket:           "test-bucket",
		Region:           "us-east-1",
		InternalEndpoint: srv.URL,
		UsePathStyle:     true,
		AccessKeyID:      "test-key",
		SecretAccessKey:  "test-secret-at-least-16",
	})
	if err != nil {
		t.Fatalf("media.New: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	payload := make([]byte, 128*1024)
	go func() {
		_, _ = w.Write(payload)
		w.Close()
	}()

	if err := store.UploadStream(context.Background(), "backups/test.dump", "application/octet-stream", r); err != nil {
		t.Fatalf("UploadStream over an os.Pipe failed: %v (seek regression?)", err)
	}
	if gotBytes == 0 {
		t.Fatal("server received no payload bytes")
	}
}
