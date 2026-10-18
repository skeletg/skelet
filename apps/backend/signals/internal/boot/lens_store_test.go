package boot

import "testing"

// A dedicated Lens bucket must never be linked publicly - nothing in it is meant to be fetched from outside
func TestADedicatedLensBucketIsPrivate(t *testing.T) {
	env := map[string]string{"S3_BUCKET_NAME": "media", "S3_PUBLIC_URL_BASE": "https://media.shop", "S3_PRESIGN_ENDPOINT_URL": "https://s3.shop"}
	shared := lensStoreConfig(func(k string) string { return env[k] })
	if shared.Bucket != "media" || shared.PublicURLBase == "" {
		t.Fatalf("without LENS_S3_BUCKET Lens should use the media bucket: %+v", shared)
	}
	env["LENS_S3_BUCKET"] = "lens-private"
	own := lensStoreConfig(func(k string) string { return env[k] })
	if own.Bucket != "lens-private" || own.PublicURLBase != "" || own.PresignEndpoint != "" {
		t.Fatalf("a dedicated bucket kept a public or presign URL: %+v", own)
	}
}
