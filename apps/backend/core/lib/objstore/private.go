package objstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The private store is the media store's sibling for files that are never linked to: same driver, endpoint and credentials, its own bucket or directory, no public address
const (
	PrivateBucketEnv = "PRIVATE_S3_BUCKET"
	PrivateDirEnv    = "PRIVATE_STORAGE_DIR"
	// Where core takes a browser's signed upload into a private directory - a PUT and nothing else, so nothing put there can be fetched back
	PrivateUploadPath = "/private-files/"
)

var (
	ErrPrivateIsPublic = errors.New("the private store is the media store, or inside it")
	ErrPrivateReadable = errors.New("a file in the private store can be read without credentials")
)

const (
	probePrefix  = ".private-probe/"
	probeTimeout = 4 * time.Second
	probeBody    = 4 << 10
)

func (c Config) Private(place string) Config {
	if c.Driver == DriverFilesystem {
		c.Dir = place
	} else {
		c.Bucket = place
	}
	c.PublicURLBase, c.PresignEndpoint = "", ""
	return c
}

func PrivateConfigFromEnv(getenv func(string) string) (Config, bool, error) {
	media := ConfigFromEnv(getenv)
	name := PrivateBucketEnv
	if media.Driver == DriverFilesystem {
		name = PrivateDirEnv
	}
	place := strings.TrimSpace(getenv(name))
	if place == "" {
		return Config{}, false, nil
	}
	if !apart(media, place) {
		return Config{}, false, fmt.Errorf("%s=%s: %w", name, place, ErrPrivateIsPublic)
	}
	cfg := media.Private(place)
	cfg.PresignEndpoint = media.PresignEndpoint
	if media.Driver == DriverFilesystem && media.PresignEndpoint != "" {
		cfg.PresignEndpoint = strings.TrimSuffix(media.PresignEndpoint, strings.TrimSuffix(FilesPath, "/")) + strings.TrimSuffix(PrivateUploadPath, "/")
	}
	return cfg, true, nil
}

// Neither directory may hold the other: one inside the media directory is served with it, one around it lets a private key name a public file
func apart(media Config, place string) bool {
	if media.Driver != DriverFilesystem {
		return place != media.Bucket
	}
	a, errA := filepath.Abs(media.Dir)
	b, errB := filepath.Abs(place)
	if errA != nil || errB != nil {
		return false
	}
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return !within(a, b) && !within(b, a)
}

// Writes a marker and asks for it back with no credentials at every address it could have. An address that doesn't answer proves nothing, so it is returned as unchecked
func ProbePrivate(ctx context.Context, private Storage, cfg, media Config, client *http.Client) (unchecked []string, err error) {
	marker, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	key := probePrefix + marker
	if err := private.PutBytes(ctx, key, []byte(marker), "text/plain"); err != nil {
		return nil, fmt.Errorf("the private store can't be written to: %w", err)
	}
	defer func() { _ = private.Delete(context.WithoutCancel(ctx), key) }()

	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	var (
		mu     sync.Mutex
		opened []string
		wg     sync.WaitGroup
	)
	for _, address := range strangerAddresses(cfg, media, key) {
		wg.Go(func() {
			got, reached := fetchAnonymously(ctx, client, address)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case !reached:
				unchecked = append(unchecked, address)
			case got == marker:
				opened = append(opened, address)
			}
		})
	}
	wg.Wait()
	if len(opened) > 0 {
		return unchecked, fmt.Errorf("%w: %s", ErrPrivateReadable, strings.Join(opened, ", "))
	}
	return unchecked, nil
}

// The store's own endpoint, the media store's public address, and that address with the private bucket's name swapped in - where a host-named web endpoint would serve it
func strangerAddresses(cfg, media Config, key string) []string {
	var out []string
	path := "/" + escapeKey(key)
	if cfg.Driver != DriverFilesystem {
		switch {
		case cfg.InternalEndpoint != "":
			out = append(out, strings.TrimRight(cfg.InternalEndpoint, "/")+"/"+cfg.Bucket+path)
		case cfg.Region != "":
			out = append(out, fmt.Sprintf("https://%s.s3.%s.amazonaws.com%s", cfg.Bucket, cfg.Region, path))
		}
	}
	base := strings.TrimRight(media.PublicURLBase, "/")
	if base == "" {
		return out
	}
	out = append(out, base+path)
	if u, err := url.Parse(base); err == nil && media.Bucket != "" && strings.HasPrefix(u.Host, media.Bucket+".") {
		u.Host = cfg.Bucket + strings.TrimPrefix(u.Host, media.Bucket)
		out = append(out, u.String()+path)
	}
	return out
}

func fetchAnonymously(ctx context.Context, client *http.Client, address string) (body string, reached bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", false
	}
	res, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", true
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, probeBody))
	return string(bytes.TrimSpace(raw)), true
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
