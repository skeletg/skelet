package boot_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The suite imports core through lib/, sdk/ and the contract and nothing else
// The compiler refuses internal/, this refuses a fifth door opened by convenience
func TestSuiteImportsCoreThroughLibAndTheContractOnly(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	const core = "github.com/skeletg/skelet/apps/backend/core/"
	allowed := []string{"lib/", "sdk/", "signalsapi"}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(p, core) {
				continue
			}
			rest := strings.TrimPrefix(p, core)
			ok := false
			for _, a := range allowed {
				if strings.HasPrefix(rest, a) {
					ok = true
					break
				}
			}
			if !ok {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s imports %s - the suite reaches core through lib/, sdk/ and the contract only", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
