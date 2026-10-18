package objstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// FileStore is the Storage on a local directory - os.Root confines every operation to it, so no key or symlink reaches past it
type FileStore struct {
	cfg  Config
	root *os.Root
}

const (
	reservedDir = ".skelet"
	metaDir     = reservedDir + "/meta"
	tmpDir      = reservedDir + "/tmp"
)

func newFileStore(cfg Config) (*FileStore, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("media: STORAGE_DIR is required for the filesystem driver")
	}
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = 10 * 1024 * 1024
	}
	cfg.PublicURLBase = strings.TrimRight(cfg.PublicURLBase, "/")
	cfg.PresignEndpoint = strings.TrimRight(cfg.PresignEndpoint, "/")
	if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("media: storage dir: %w", err)
	}
	root, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("media: storage dir: %w", err)
	}
	if err := root.MkdirAll(tmpDir, 0o750); err != nil {
		return nil, fmt.Errorf("media: storage dir: %w", err)
	}
	return &FileStore{cfg: cfg, root: root}, nil
}

var ErrInvalidKey = errors.New("objstore: invalid key")

// S3 takes nearly any string as a key, a directory doesn't - an empty or dot segment, a backslash or NUL, or the reserved tree is refused
func checkKey(key string) error {
	if key == "" || len(key) > 1024 || strings.ContainsAny(key, "\\\x00") {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for seg := range strings.SplitSeq(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	if key == reservedDir || strings.HasPrefix(key, reservedDir+"/") {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return nil
}

func (s *FileStore) MaxUploadBytes() int64 { return s.cfg.MaxUploadBytes }

func (s *FileStore) Bucket() string { return s.cfg.Dir }

func (s *FileStore) Head(_ context.Context, key string) (int64, string, bool, error) {
	if err := checkKey(key); err != nil {
		return 0, "", false, err
	}
	fi, err := s.root.Stat(key)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && fi.IsDir()) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("media: head: %w", err)
	}
	return fi.Size(), s.contentType(key), true, nil
}

func (s *FileStore) contentType(key string) string {
	b, err := s.root.ReadFile(metaDir + "/" + key)
	if err != nil || len(b) == 0 {
		return "application/octet-stream"
	}
	return string(b)
}

// Removes the directories a delete left empty too - S3 has no directories, so a prefix with no objects under it must stop listing
func (s *FileStore) Delete(_ context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := s.root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("media: delete: %w", err)
	}
	_ = s.root.Remove(metaDir + "/" + key)
	s.pruneEmpty(path.Dir(key))
	s.pruneEmpty(path.Dir(metaDir + "/" + key))
	return nil
}

func (s *FileStore) pruneEmpty(dir string) {
	for dir != "." && dir != metaDir && dir != reservedDir {
		if s.root.Remove(dir) != nil {
			return
		}
		dir = path.Dir(dir)
	}
}

func (s *FileStore) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	if err := s.write(key, contentType, bytes.NewReader(data), -1); err != nil {
		return fmt.Errorf("media: put bytes: %w", err)
	}
	return nil
}

func (s *FileStore) UploadStream(ctx context.Context, key, contentType string, body io.Reader) error {
	if err := s.write(key, contentType, body, -1); err != nil {
		return fmt.Errorf("media: upload stream: %w", err)
	}
	return nil
}

func (s *FileStore) PutEmptyObject(ctx context.Context, key string) error {
	if err := s.write(key, "", bytes.NewReader(nil), -1); err != nil {
		return fmt.Errorf("media: put empty object: %w", err)
	}
	return nil
}

var errSizeMismatch = errors.New("objstore: body size does not match the signed size")

// Lands the body in the reserved tmp tree, then renames it into place, so a reader never sees half an object
func (s *FileStore) write(key, contentType string, body io.Reader, want int64) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	tmp, err := s.writeTemp(body, want)
	if err != nil {
		return err
	}
	defer func() { _ = s.root.Remove(tmp) }()
	meta, err := s.writeTemp(strings.NewReader(contentType), -1)
	if err != nil {
		return err
	}
	defer func() { _ = s.root.Remove(meta) }()
	if err := s.place(meta, metaDir+"/"+key); err != nil {
		return err
	}
	return s.place(tmp, key)
}

func (s *FileStore) writeTemp(body io.Reader, want int64) (string, error) {
	var nonce [12]byte
	_, _ = rand.Read(nonce[:])
	name := tmpDir + "/" + hex.EncodeToString(nonce[:])
	f, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", err
	}
	src := body
	if want >= 0 {
		src = io.LimitReader(body, want+1)
	}
	n, err := io.Copy(f, src)
	if err == nil && want >= 0 && n != want {
		err = errSizeMismatch
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = s.root.Remove(name)
		return "", err
	}
	return name, nil
}

// A concurrent delete can prune the parent between MkdirAll and Rename, so a missing parent gets one more try
func (s *FileStore) place(tmp, key string) error {
	var err error
	for range 2 {
		if err = s.root.MkdirAll(path.Dir(key), 0o750); err != nil {
			return err
		}
		if err = s.root.Rename(tmp, key); !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return err
}

func (s *FileStore) GetStream(_ context.Context, key string) (io.ReadCloser, error) {
	f, err := s.open(key)
	if err != nil {
		return nil, fmt.Errorf("media: get stream: %w", err)
	}
	return f, nil
}

func (s *FileStore) open(key string) (*os.File, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	f, err := s.root.Open(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err != nil || fi.IsDir() {
		f.Close()
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	return f, nil
}

func (s *FileStore) GetBytes(ctx context.Context, key string) ([]byte, error) {
	b, _, err := s.GetBytesTyped(ctx, key)
	return b, err
}

func (s *FileStore) GetBytesTyped(_ context.Context, key string) ([]byte, string, error) {
	f, err := s.open(key)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("media: get bytes: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("media: read body: %w", err)
	}
	return b, s.contentType(key), nil
}

func (s *FileStore) Copy(_ context.Context, fromKey, toKey string) error {
	f, err := s.open(fromKey)
	if err != nil {
		return fmt.Errorf("media: copy object: %w", err)
	}
	defer f.Close()
	if err := s.write(toKey, s.contentType(fromKey), f, -1); err != nil {
		return fmt.Errorf("media: copy object: %w", err)
	}
	return nil
}

func (s *FileStore) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	objs, err := s.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("media: list prefix: %w", err)
	}
	keys := make([]string, len(objs))
	for i, o := range objs {
		keys[i] = o.Key
	}
	return keys, nil
}

func (s *FileStore) ListObjects(_ context.Context, prefix string) ([]StorageObject, error) {
	start, _ := splitPrefix(prefix)
	objs := []StorageObject{}
	if start != "." && checkKey(start) != nil {
		return objs, nil
	}
	err := fs.WalkDir(s.root.FS(), start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if p == reservedDir || (p != start && !strings.HasPrefix(p+"/", prefix) && !strings.HasPrefix(prefix, p+"/")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(p, prefix) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		objs = append(objs, StorageObject{Key: p, Size: fi.Size(), ContentType: "application/octet-stream", LastModified: fi.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(objs, func(a, b StorageObject) int { return strings.Compare(a.Key, b.Key) })
	return objs, nil
}

func splitPrefix(prefix string) (dir, name string) {
	i := strings.LastIndex(prefix, "/")
	if i < 0 {
		return ".", prefix
	}
	return prefix[:i], prefix[i+1:]
}

func (s *FileStore) ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error) {
	prefixes, _, err := s.ListDirectoryContents(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("media: list immediate prefixes: %w", err)
	}
	if prefixes == nil {
		prefixes = []string{}
	}
	return prefixes, nil
}

func (s *FileStore) ListDirectoryContents(_ context.Context, prefix string) (prefixes []string, objects []StorageObject, err error) {
	dir, name := splitPrefix(prefix)
	if dir != "." && checkKey(dir) != nil {
		return nil, nil, nil
	}
	entries, err := fs.ReadDir(s.root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("media: list directory contents: %w", err)
	}
	base := ""
	if dir != "." {
		base = dir + "/"
	}
	for _, e := range entries {
		key := base + e.Name()
		if !strings.HasPrefix(e.Name(), name) || key == reservedDir {
			continue
		}
		if e.IsDir() {
			prefixes = append(prefixes, key+"/")
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		objects = append(objects, StorageObject{Key: key, Size: fi.Size(), ContentType: "application/octet-stream", LastModified: fi.ModTime()})
	}
	return prefixes, objects, nil
}

func (s *FileStore) HasPublicBase() bool { return s.cfg.PublicURLBase != "" }

func (s *FileStore) PublicURL(key string) string {
	base := s.cfg.PublicURLBase
	if base == "" {
		base = strings.TrimSuffix(FilesPath, "/")
	}
	return base + "/" + escapeKey(key)
}

func (s *FileStore) Close() error { return s.root.Close() }
