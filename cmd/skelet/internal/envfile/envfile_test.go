package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func file(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.env")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func content(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSet(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"replaces the live line", "A=1\nKEY=old\nB=2\n", "A=1\nKEY=new\nB=2\n"},
		{"replaces every live line, so no later duplicate wins", "KEY=a\n# note\nKEY=b\n", "KEY=new\n# note\nKEY=new\n"},
		{"uncomments the first commented line when none is live", "# KEY=example\n#KEY=other\n", "KEY=new\n#KEY=other\n"},
		{"leaves a commented line alone when a live one exists", "# KEY=example\nKEY=old\n", "# KEY=example\nKEY=new\n"},
		{"appends, mending a missing trailing newline", "A=1", "A=1\nKEY=new\n"},
		{"does not take a longer key for this one", "KEYS=1\nMY_KEY=2\n", "KEYS=1\nMY_KEY=2\nKEY=new\n"},
		{"keeps indentation-tolerant matches", "  KEY=old\n", "KEY=new\n"},
	}
	for _, c := range cases {
		p := file(t, c.in, 0o640)
		if err := Set(p, "KEY", "new"); err != nil {
			t.Fatal(err)
		}
		if got := content(t, p); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestGetIsWhatComposeReads(t *testing.T) {
	p := file(t, "# KEY=commented\nKEY=first\nKEY=last\nEMPTY=\n", 0o644)
	if v, ok, _ := Get(p, "KEY"); !ok || v != "last" {
		t.Errorf("Get = %q, %v - want the last live line", v, ok)
	}
	if v, ok, _ := Get(p, "EMPTY"); !ok || v != "" {
		t.Errorf("an empty value is present: %q, %v", v, ok)
	}
	if _, ok, _ := Get(p, "MISSING"); ok {
		t.Error("a missing key reported present")
	}
	if _, ok, err := Get(filepath.Join(t.TempDir(), "none"), "KEY"); ok || err != nil {
		t.Errorf("missing file: %v, %v", ok, err)
	}
}

func TestDelDropsLiveAndCommentedLines(t *testing.T) {
	p := file(t, "A=1\n# KEY=doc\nKEY=x\nB=2\n", 0o644)
	if err := Del(p, "KEY"); err != nil {
		t.Fatal(err)
	}
	if got := content(t, p); got != "A=1\nB=2\n" {
		t.Errorf("got %q", got)
	}
}

func TestWritesKeepTheModeAndSkipNoOps(t *testing.T) {
	p := file(t, "KEY=same\n", 0o640)
	before, _ := os.Stat(p)
	if err := Set(p, "KEY", "same"); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(p); !os.SameFile(before, after) {
		t.Error("an unchanged value rewrote the file")
	}
	if err := Set(p, "KEY", "changed"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %v, want 0640 kept", fi.Mode().Perm())
	}

	fresh := filepath.Join(t.TempDir(), "new.env")
	if err := Set(fresh, "SECRET", "x"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(fresh); fi.Mode().Perm() != 0o600 {
		t.Errorf("a new file is %v, want 0600 - these hold secrets", fi.Mode().Perm())
	}
}

func TestSymlinkedFileIsWrittenThrough(t *testing.T) {
	real := file(t, "KEY=old\n", 0o644)
	link := filepath.Join(t.TempDir(), "link.env")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := Set(link, "KEY", "new"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a file")
	}
	if got := content(t, real); got != "KEY=new\n" {
		t.Errorf("target holds %q", got)
	}
}
