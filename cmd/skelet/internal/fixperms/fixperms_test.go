package fixperms

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanListsWhatIsntYours(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps", "dashboard", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apps", "dashboard", "src", "x.ts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if strays := scan(root, os.Getuid()); len(strays) != 0 {
		t.Errorf("your own files reported: %v", strays)
	}
	// Under another uid everything is someone else's - the tree, its directories and files, by path from the root
	strays := scan(root, os.Getuid()+1)
	if len(strays) != 4 || strays[0] != "apps" || strays[3] != filepath.Join("apps", "dashboard", "src", "x.ts") {
		t.Errorf("strays %v", strays)
	}
	if code := Run(root, []string{"--bogus"}); code != 2 {
		t.Errorf("an unknown mode: exit %d", code)
	}
}
