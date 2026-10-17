package db

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
)

// Every stream reads as dbmate files with no version taken twice - two branches that numbered a migration alike merge cleanly in git and only fail here
func TestMigrationFilesAreWellFormed(t *testing.T) {
	for _, dir := range []string{
		"migrations",
		"../../../signals/internal/db/migrations",
		"../../../signals/internal/analytics/spinech/migrations",
	} {
		files, err := migfile.List(dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if len(files) == 0 {
			t.Errorf("%s holds no migrations", dir)
		}
		for _, f := range files {
			if len(f.Downs) == 0 {
				t.Errorf("%s has no -- migrate:down section", f.Path)
			}
		}
	}
}
