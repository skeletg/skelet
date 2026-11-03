// Package envfile edits KEY=value files - the env templates' copies and the root .env - a line at a time, leaving
// every other line, comment and blank exactly as it was
package envfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Get is the value compose reads: the last live KEY= line. ok is false when the key has none, or the file is missing
func Get(path, key string) (val string, ok bool, err error) {
	lines, _, err := read(path)
	if err != nil {
		return "", false, err
	}
	for _, l := range lines {
		if v, live := liveValue(l, key); live {
			val, ok = v, true
		}
	}
	return val, ok, nil
}

// Set gives every live KEY= line the value, so no duplicate further down can override it. With none, it uncomments
// the first commented-out one, and with no line at all it appends one
func Set(path, key, val string) error {
	lines, mode, err := read(path)
	if err != nil {
		return err
	}
	out := key + "=" + val
	found := false
	for i, l := range lines {
		if _, live := liveValue(l, key); live {
			lines[i], found = out, true
		}
	}
	for i, l := range lines {
		if !found && commented(l, key) {
			lines[i], found = out, true
		}
	}
	if !found {
		lines = append(lines, out)
	}
	return write(path, lines, mode)
}

// Del drops the key's lines, commented-out ones included, for a knob whose default is being absent
func Del(path, key string) error {
	lines, mode, err := read(path)
	if err != nil {
		return err
	}
	kept := lines[:0:0]
	for _, l := range lines {
		if _, live := liveValue(l, key); !live && !commented(l, key) {
			kept = append(kept, l)
		}
	}
	return write(path, kept, mode)
}

var keyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Pairs reads every KEY=value in file order, the way a dotenv reader does rather than a shell: no expansion, an
// export prefix and matching quotes dropped, and a " #" ending an unquoted value. A missing file has none
func Pairs(path string) ([]string, error) {
	lines, _, err := read(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(l, "export "), "=")
		if !ok || !keyName.MatchString(key) {
			continue
		}
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		} else if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		out = append(out, key+"="+val)
	}
	return out, nil
}

func liveValue(line, key string) (string, bool) {
	return strings.CutPrefix(strings.TrimLeft(line, " \t"), key+"=")
}

func commented(line, key string) bool {
	rest := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(rest, "#") && strings.HasPrefix(strings.TrimLeft(rest, "# \t"), key+"=")
}

// A missing file reads as empty, and is created owner-only when written - these hold secrets
func read(path string) ([]string, fs.FileMode, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0o600, nil
	}
	if err != nil {
		return nil, 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	text := strings.TrimSuffix(string(raw), "\n")
	if text == "" {
		return nil, fi.Mode().Perm(), nil
	}
	return strings.Split(text, "\n"), fi.Mode().Perm(), nil
}

func write(path string, lines []string, mode fs.FileMode) error {
	data := ""
	if len(lines) > 0 {
		data = strings.Join(lines, "\n") + "\n"
	}
	if old, err := os.ReadFile(path); err == nil && string(old) == data {
		return nil
	}
	return WriteAtomic(path, []byte(data), mode)
}

// WriteAtomic replaces path through a temp file and a rename, so a crash mid-write leaves the old file whole. A
// symlinked env file is written through, the way a shell redirect would
func WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
