// Package objstore wraps S3, or any S3-compatible store like Garage - PresignUpload hands clients PUT URLs, the rest are server-side
// Two endpoints exist because a presigned URL embeds a hostname: in dev the server reaches Garage by its network name while the browser needs localhost
package objstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type Config struct {
	Bucket string
	Region string
	// The server-side client (Head / Delete) - empty for real AWS S3, set to the S3-compatible URL in dev or on-prem
	InternalEndpoint string
	// Baked into presigned URLs, so it's what the browser PUTs to - empty means use InternalEndpoint - In compose dev that's http://localhost:9900 while internal is http://garage:3900
	PresignEndpoint string
	// Prefix for public read URLs - a CDN origin, or the Garage web-endpoint vhost - Trailing slash optional, normalized internally
	PublicURLBase string
	// Path-style (host/bucket/key) rather than virtual-hosted (bucket.host/key) - Garage and MinIO want true, real AWS S3 wants false
	UsePathStyle bool
	// Override the SDK's default credential chain when both are set - the server leaves them empty and uses env vars or an IAM role, while media-migrate sets them to reach a second store
	AccessKeyID     string
	SecretAccessKey string
	// Defaults to 10MB when zero - enforced at the application layer AND signed into each
	// presigned PUT as an exact Content-Length, so a client can't understate the size
	MaxUploadBytes int64
}

// Safe for concurrent use
type Storage struct {
	cfg            Config
	internalClient *s3.Client
	presignClient  *s3.PresignClient
}

// Credentials come from the SDK's default chain (env, shared config, IAM role), so a prod deployment with a role attached needs no explicit creds
func New(ctx context.Context, cfg Config) (*Storage, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("media: S3_BUCKET_NAME is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = 10 * 1024 * 1024
	}
	if cfg.PresignEndpoint == "" {
		cfg.PresignEndpoint = cfg.InternalEndpoint
	}
	cfg.PublicURLBase = strings.TrimRight(cfg.PublicURLBase, "/")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("media: load aws config: %w", err)
	}

	// Explicit per-Config credentials beat the default chain - used when one process talks to two stores, as media-migrate does
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")
	} else if _, derr := awsCfg.Credentials.Retrieve(ctx); derr != nil && os.Getenv("APP_ENV") == "development" {
		// DEV ONLY - accept loose Garage defaults when the chain has no creds, so local dev works on a host with no AWS profile
		// In prod a missing-credentials misconfiguration must surface loudly rather than silently fall back to a well-known dev key
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider("skeletdev", "skeletdev-secret-not-for-prod", "")
	}

	// WhenRequired = the pre-2025 SDK defaults: the newer always-on CRC32 protections misvalidate multipart objects on
	// Garage-style stores (a GetObject body errors at EOF with a checksum mismatch even though the bytes are intact)
	tuneClient := func(endpoint string) func(o *s3.Options) {
		return func(o *s3.Options) {
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
			o.UsePathStyle = cfg.UsePathStyle
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		}
	}
	internalClient := s3.NewFromConfig(awsCfg, tuneClient(cfg.InternalEndpoint))
	presignSource := s3.NewFromConfig(awsCfg, tuneClient(cfg.PresignEndpoint))
	presignClient := s3.NewPresignClient(presignSource)

	return &Storage{
		cfg:            cfg,
		internalClient: internalClient,
		presignClient:  presignClient,
	}, nil
}

func (s *Storage) MaxUploadBytes() int64 { return s.cfg.MaxUploadBytes }

func (s *Storage) Bucket() string { return s.cfg.Bucket }

// Content-Type is locked into the presigned URL - the client MUST send the exact same header on the upload PUT or S3 rejects it A positive sizeBytes is signed in as the exact Content-Length too, so the store rejects any other body size - without it a client could declare 1 byte and PUT gigabytes ttl bounds how long the URL is usable; 15 minutes suits interactive admin uploads
func (s *Storage) PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error) {
	in := &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}
	if sizeBytes > 0 {
		in.ContentLength = aws.Int64(sizeBytes)
	}
	req, err := s.presignClient.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("media: presign upload: %w", err)
	}
	return req.URL, nil
}

// Returns (0, "", false, nil) for a missing object rather than an error - ConfirmMediaUpload uses it to verify the client finished uploading
func (s *Storage) Head(ctx context.Context, key string) (int64, string, bool, error) {
	out, err := s.internalClient.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("media: head: %w", err)
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	var ct string
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return size, ct, true, nil
}

// Idempotent - a missing object returns nil
func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.internalClient.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("media: delete: %w", err)
	}
	return nil
}

// Server-side rather than presigned - used for server-owned blobs like Lens replay chunks and DOM snapshots, with contentType defaulting to application/octet-stream
func (s *Storage) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.internalClient.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("media: put bytes: %w", err)
	}
	return nil
}

// Streams an object of unknown length, multipart under the hood - the writer never has to know the size or buffer it
func (s *Storage) UploadStream(ctx context.Context, key, contentType string, body io.Reader) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// manager is deprecated in favour of feature/s3/transfermanager, but that's still v0.x with no stability guarantee - migrate once it reaches v1
	uploader := manager.NewUploader(s.internalClient, func(u *manager.Uploader) { //nolint:staticcheck
		// The default 5MB parts hit the 10k-part cap at ~49GB - a large DB dump exceeds
		// that; 64MB parts raise the ceiling to ~640GB
		u.PartSize = 64 * 1024 * 1024
	})
	// Strip everything but Read - pipe-backed *os.Files advertise Seek but fail with ESPIPE at runtime, and the uploader prefers seeking when the body claims to support it
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{ //nolint:staticcheck
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		Body:        struct{ io.Reader }{body},
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("media: upload stream: %w", err)
	}
	return nil
}

// The caller owns the ReadCloser, so a large object streams through rather than being buffered in memory
func (s *Storage) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.internalClient.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("media: get stream: %w", err)
	}
	return out.Body, nil
}

// A missing object is an error - the caller treats it as data loss, not an empty blob It's ErrObjectNotFound specifically, so callers can tell "this key is gone" from "the store is down" - the transform handler answers 404 for the first and 502 for the second, and a CDN must not cache an outage as a 404
func (s *Storage) GetBytes(ctx context.Context, key string) ([]byte, error) {
	out, err := s.internalClient.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
		}
		return nil, fmt.Errorf("media: get bytes: %w", err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("media: read body: %w", err)
	}
	return b, nil
}

func (s *Storage) PutEmptyObject(ctx context.Context, key string) error {
	_, err := s.internalClient.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(nil),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("media: put empty object: %w", err)
	}
	return nil
}

func (s *Storage) Copy(ctx context.Context, fromKey, toKey string) error {
	source := s.cfg.Bucket + "/" + escapeKey(fromKey)
	_, err := s.internalClient.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.cfg.Bucket),
		Key:        aws.String(toKey),
		CopySource: aws.String(source),
	})
	if err != nil {
		return fmt.Errorf("media: copy object: %w", err)
	}
	return nil
}

func (s *Storage) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	keys := []string{}
	var token *string
	for {
		out, err := s.internalClient.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.cfg.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("media: list prefix: %w", err)
		}
		for _, obj := range out.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	return keys, nil
}

func (s *Storage) ListImmediatePrefixes(ctx context.Context, prefix string) ([]string, error) {
	seen := map[string]struct{}{}
	outPrefixes := []string{}
	var token *string
	for {
		out, err := s.internalClient.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.cfg.Bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("media: list immediate prefixes: %w", err)
		}
		for _, cp := range out.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			if _, ok := seen[*cp.Prefix]; ok {
				continue
			}
			seen[*cp.Prefix] = struct{}{}
			outPrefixes = append(outPrefixes, *cp.Prefix)
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	return outPrefixes, nil
}

type StorageObject struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
}

// Immediate subdirectory prefixes and immediate objects in one paginated call (Delimiter=/), avoiding a second round-trip
func (s *Storage) ListDirectoryContents(ctx context.Context, prefix string) (prefixes []string, objects []StorageObject, err error) {
	seenPfx := map[string]struct{}{}
	var token *string
	for {
		out, err := s.internalClient.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.cfg.Bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("media: list directory contents: %w", err)
		}
		for _, cp := range out.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			if _, ok := seenPfx[*cp.Prefix]; ok {
				continue
			}
			seenPfx[*cp.Prefix] = struct{}{}
			prefixes = append(prefixes, *cp.Prefix)
		}
		for _, obj := range out.Contents {
			if obj.Key == nil {
				continue
			}
			so := StorageObject{Key: *obj.Key, ContentType: "application/octet-stream"}
			if obj.Size != nil {
				so.Size = *obj.Size
			}
			if obj.LastModified != nil {
				so.LastModified = *obj.LastModified
			}
			objects = append(objects, so)
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	return prefixes, objects, nil
}

// PublicURL always returns something, but its fallback is container-network only - check this before handing a URL to a browser
func (s *Storage) HasPublicBase() bool { return s.cfg.PublicURLBase != "" }

// Honors PublicURLBase, falling back to a URL built from InternalEndpoint
func (s *Storage) PublicURL(key string) string {
	base := s.cfg.PublicURLBase
	if base == "" {
		base = s.cfg.InternalEndpoint
		if base != "" {
			// Path-style fallback: {endpoint}/{bucket}/{key}
			base = strings.TrimRight(base, "/") + "/" + s.cfg.Bucket
		}
	}
	if base == "" {
		base = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", s.cfg.Bucket, s.cfg.Region)
	}
	return base + "/" + escapeKey(key)
}

// S3 keys allow most characters, but URL-embedding needs percent-encoding for some
func escapeKey(k string) string {
	// url.PathEscape over-escapes slashes, so go segment by segment
	parts := strings.Split(k, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// ErrObjectNotFound is a key that isn't in the bucket, as opposed to a store that wouldn't answer
var ErrObjectNotFound = errors.New("objstore: object not found")

// Detects NoSuchKey / 404 across the wrapping variants the SDK returns
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*types.NotFound](err); ok {
		return true
	}
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}
	if ae, ok := errors.AsType[smithy.APIError](err); ok {
		switch ae.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}
	return false
}
