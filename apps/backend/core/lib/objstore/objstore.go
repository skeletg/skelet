package objstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DriverS3         = "s3"
	DriverFilesystem = "filesystem"
)

// Storage is the object store every file goes through - an S3-compatible service, or a directory on the local disk
type Storage interface {
	MaxUploadBytes() int64
	Bucket() string
	PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error)
	Head(ctx context.Context, key string) (int64, string, bool, error)
	Delete(ctx context.Context, key string) error
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	UploadStream(ctx context.Context, key, contentType string, body io.Reader) error
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
	// ErrObjectNotFound for a missing key, so a caller can tell "gone" from "the store is down"
	GetBytes(ctx context.Context, key string) ([]byte, error)
	GetBytesTyped(ctx context.Context, key string) ([]byte, string, error)
	PutEmptyObject(ctx context.Context, key string) error
	Copy(ctx context.Context, fromKey, toKey string) error
	ListPrefix(ctx context.Context, prefix string) ([]string, error)
	ListObjects(ctx context.Context, prefix string) ([]StorageObject, error)
	ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error)
	ListDirectoryContents(ctx context.Context, prefix string) (prefixes []string, objects []StorageObject, err error)
	HasPublicBase() bool
	PublicURL(key string) string
}

func New(ctx context.Context, cfg Config) (Storage, error) {
	switch cfg.Driver {
	case "", DriverS3:
		s, err := newS3(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return s, nil
	case DriverFilesystem:
		s, err := newFileStore(cfg)
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	return nil, fmt.Errorf("media: STORAGE_DRIVER %q is neither %q nor %q", cfg.Driver, DriverS3, DriverFilesystem)
}

// Where core serves the filesystem driver's objects - GET and HEAD read one, a PUT signed by PresignUpload writes it
const FilesPath = "/files/"

// Development only - envcheck refuses it in production
const devSigningKey = "skelet-dev-storage-signing-key-not-for-prod"

// Every process builds its store from the same variables, so core, the media tier, the CLI and Signals can't disagree
func ConfigFromEnv(getenv func(string) string) Config {
	cfg := Config{Driver: strings.TrimSpace(getenv("STORAGE_DRIVER")), MaxUploadBytes: envInt64(getenv, "STORAGE_MAX_UPLOAD_BYTES")}
	if cfg.Driver != DriverFilesystem {
		cfg.Bucket = getenv("S3_BUCKET_NAME")
		cfg.Region = getenv("AWS_REGION")
		cfg.InternalEndpoint = getenv("S3_ENDPOINT_URL")
		cfg.PresignEndpoint = getenv("S3_PRESIGN_ENDPOINT_URL")
		cfg.PublicURLBase = getenv("S3_PUBLIC_URL_BASE")
		cfg.UsePathStyle = getenv("S3_USE_PATH_STYLE") == "true"
		return cfg
	}
	cfg.Dir = strings.TrimSpace(getenv("STORAGE_DIR"))
	if core := strings.TrimRight(strings.TrimSpace(getenv("STORAGE_UPLOAD_URL")), "/"); core != "" {
		cfg.PresignEndpoint = core + strings.TrimSuffix(FilesPath, "/")
		cfg.PublicURLBase = cfg.PresignEndpoint
	}
	if pub := strings.TrimRight(strings.TrimSpace(getenv("STORAGE_PUBLIC_URL")), "/"); pub != "" {
		cfg.PublicURLBase = pub
	}
	cfg.SigningKey = strings.TrimSpace(getenv("STORAGE_SIGNING_KEY"))
	if cfg.SigningKey == "" && getenv("APP_ENV") == "development" {
		cfg.SigningKey = devSigningKey
	}
	return cfg
}

func FromEnv() Config { return ConfigFromEnv(os.Getenv) }

func envInt64(getenv func(string) string, name string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(getenv(name)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
