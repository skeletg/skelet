package db

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/google/uuid"

	sqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func mediaRig(t *testing.T, keys ...string) (*sqlc.Queries, string) {
	t.Helper()
	tx, err := testdb.Require(t).Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	root := "liketest-" + uuid.NewString()[:8] + "/"
	for _, k := range keys {
		if _, err := tx.Exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status) VALUES ($1, $2, $3, 'image/png', 1, 'ready')`,
			uuid.New(), k, root+k); err != nil {
			t.Fatal(err)
		}
	}
	return sqlc.New(tx), root
}

func objectKeys(rows []sqlc.MediaAsset, root string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ObjectKey[len(root):]
	}
	slices.Sort(out)
	return out
}

// A folder named a_b is not a pattern - deleting it must leave axb's rows, whose files the bucket still holds
func TestAFolderDeleteSparesItsWildcardTwin(t *testing.T) {
	q, root := mediaRig(t, "a_b/one.png", "a_b/deep/two.png", "axb/three.png")
	ctx := context.Background()
	if err := q.DeleteMediaAssetsByDirectoryPrefix(ctx, root+"a_b/"); err != nil {
		t.Fatal(err)
	}
	left, err := q.ListMediaAssetsByPathRecursive(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := objectKeys(left, root); !slices.Equal(got, []string{"axb/three.png"}) {
		t.Fatalf("left %v, want only the twin folder's file", got)
	}
}

func TestAFolderListsOnlyItsOwnFiles(t *testing.T) {
	q, root := mediaRig(t, "a_b/one.png", "a_b/deep/two.png", "axb/three.png", "top.png")
	ctx := context.Background()
	rows, err := q.ListMediaAssetsByDirectory(ctx, root+"a_b/")
	if err != nil {
		t.Fatal(err)
	}
	if got := objectKeys(rows, root); !slices.Equal(got, []string{"a_b/one.png"}) {
		t.Fatalf("a_b/ listed %v", got)
	}
	if rows, err = q.ListMediaAssetsByDirectory(ctx, root); err != nil {
		t.Fatal(err)
	}
	if got := objectKeys(rows, root); !slices.Equal(got, []string{"top.png"}) {
		t.Fatalf("the root listed %v", got)
	}
}

func TestATypedWildcardMatchesItself(t *testing.T) {
	q, root := mediaRig(t, "50%-off.png", "500-off.png", "a_b.png", "axb.png")
	ctx := context.Background()
	for term, want := range map[string][]string{"50%": {"50%-off.png"}, "a_b": {"a_b.png"}, "off": {"50%-off.png", "500-off.png"}} {
		rows, err := q.SearchMediaAssets(ctx, sqlc.SearchMediaAssetsParams{Column1: root, Column3: term, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got := objectKeys(rows, root); !slices.Equal(got, want) {
			t.Errorf("search %q found %v, want %v", term, got, want)
		}
	}
}

func TestEscapeLikeNeutralisesEveryMetacharacter(t *testing.T) {
	conn := testdb.Require(t)
	for _, tc := range []struct {
		value, term string
		want        bool
	}{{`a_b`, `a_b`, true}, {`axb`, `a_b`, false}, {`5%`, `5%`, true}, {`50`, `5%`, false}, {`c:\dir`, `\d`, true}, {`c:dir`, `\d`, false}} {
		var got sql.NullBool
		if err := conn.QueryRow(`SELECT $1::text LIKE '%' || escape_like($2::text) || '%'`, tc.value, tc.term).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got.Bool != tc.want {
			t.Errorf("%q LIKE escaped %q = %v, want %v", tc.value, tc.term, got.Bool, tc.want)
		}
	}
}

// The transform endpoint reads the focal point through this lookup - a status the table can't hold made every crop ignore it
func TestAReadyAssetResolvesByObjectKey(t *testing.T) {
	q, root := mediaRig(t, "hero.png")
	if _, err := q.GetMediaAssetByObjectKey(context.Background(), root+"hero.png"); err != nil {
		t.Fatalf("a ready asset did not resolve: %v", err)
	}
}
