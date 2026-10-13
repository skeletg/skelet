package wiring

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// Only a URL under the bucket's own public base streams - anything else is someone else's file and keeps redirecting
func TestAStoredDownloadIsRecognisedByItsPublicBase(t *testing.T) {
	st, err := objstore.New(context.Background(), objstore.Config{Bucket: "media", PublicURLBase: "https://cdn.example.test/media", AccessKeyID: "k", SecretAccessKey: "s"})
	if err != nil {
		t.Fatal(err)
	}
	files := storedDownloads{st}
	for u, want := range map[string]string{
		st.PublicURL("books/the book.pdf"):           "books/the book.pdf",
		"https://cdn.example.test/media/a/b.zip?v=2": "a/b.zip",
		"https://cdn.example.test/mediaX/a.zip":      "",
		"https://elsewhere.example.test/media/a.zip": "",
		"https://cdn.example.test/media/":            "",
	} {
		key, ok := files.KeyFor(u)
		if key != want || ok != (want != "") {
			t.Errorf("KeyFor(%q) = %q, %v - want %q", u, key, ok, want)
		}
	}
}
