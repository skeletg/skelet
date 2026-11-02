package project

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	got, err := ParseArgs([]string{"shop", "--yes", "--version", "v0.3.0", "--apps=dashboard", "--repo=file:///fork", "--force", "--tools", "none"})
	want := Options{Dir: "shop", Version: "v0.3.0", Repo: "file:///fork", Force: true, Setup: []string{"--yes", "--apps=dashboard", "--tools", "none"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v\nwant %+v", got, err, want)
	}
	if got, _ := ParseArgs([]string{"shop"}); got.Repo != Upstream || got.Version != "" || got.Setup != nil {
		t.Errorf("defaults: %+v", got)
	}
	for _, args := range [][]string{{}, {"--yes", "shop"}, {"shop", "--version"}, {"shop", "--repo", "--yes"}, {"shop", "--version="}} {
		if _, err := ParseArgs(args); err == nil || errors.Is(err, ErrHelp) {
			t.Errorf("%q: err %v, want a usage error", args, err)
		}
	}
	if _, err := ParseArgs([]string{"shop", "--yes", "-h"}); !errors.Is(err, ErrHelp) {
		t.Errorf("-h after the directory: %v", err)
	}
}

type fakeSource struct {
	latest, got string
}

func (f *fakeSource) Latest(context.Context) (string, error) { return f.latest, nil }

func (f *fakeSource) Create(_ context.Context, version, dir string) error {
	f.got = version
	return os.MkdirAll(dir, 0o755)
}

func TestCreatePicksTheRelease(t *testing.T) {
	for _, c := range []struct{ flag, own, want string }{
		{"", "v1.2.3", "v1.2.3"},
		{"", "dev", "v9.9.9"},
		{"v0.1.0", "v1.2.3", "v0.1.0"},
	} {
		src := &fakeSource{latest: "v9.9.9"}
		o := Options{Dir: filepath.Join(t.TempDir(), "shop"), Version: c.flag}
		if _, tag, err := Create(context.Background(), src, o, c.own, io.Discard); err != nil || tag != c.want || src.got != c.want {
			t.Errorf("--version %q from %s: tag %q, fetched %q, %v - want %s", c.flag, c.own, tag, src.got, err, c.want)
		}
	}
}

func TestCreateRefusesAFullDirectoryAndANestedProject(t *testing.T) {
	full := t.TempDir()
	if err := os.WriteFile(filepath.Join(full, "notes"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{}
	if _, _, err := Create(context.Background(), src, Options{Dir: full}, "v1.0.0", io.Discard); err == nil || !strings.Contains(err.Error(), "isn't empty") {
		t.Errorf("non-empty target: %v", err)
	}
	empty := t.TempDir()
	if _, _, err := Create(context.Background(), src, Options{Dir: empty}, "v1.0.0", io.Discard); err != nil {
		t.Errorf("an empty directory is a fine target: %v", err)
	}

	root := t.TempDir()
	for _, f := range []string{"skelet", "bin/_lib"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	nested := Options{Dir: filepath.Join(root, "apps", "shop")}
	if _, _, err := Create(context.Background(), src, nested, "v1.0.0", io.Discard); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("nested target: %v", err)
	}
	nested.Force = true
	if _, _, err := Create(context.Background(), src, nested, "v1.0.0", io.Discard); err != nil {
		t.Errorf("nested target with --force: %v", err)
	}
}
