package front

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The storefront has no admin behind it, so it never names the private half or calls a getter that returns a private file
var privateReads = regexp.MustCompile(`\.GetAnyAsset\(|MediaLibrary(Svc)?\.Open\(|medialibrary\.Private\b|RequireHalf\(`)

func TestTheStorefrontNeverReadsAPrivateFile(t *testing.T) {
	scanned := 0
	for _, dir := range []string{".", "dataloaders"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			scanned++
			if hit := privateReads.Find(src); hit != nil {
				t.Errorf("%s calls %s - a shopper would be handed a private file", filepath.Join(dir, name), strings.TrimSuffix(string(hit), "("))
			}
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned only %d files - the storefront's resolvers were not found", scanned)
	}
}
