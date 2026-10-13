package remoteprovider

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func testSealer(t *testing.T) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The signing secret and the config's third-party API keys never sit in the column as plaintext, and still read back whole
func TestSecretsAndConfigAreSealedAtRest(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn), testSealer(t))
	svc := NewService(repo)
	svc.AllowPrivateTargets = true
	res, err := svc.Create(ctx, CreateInput{Kind: KindTaxProvider, Slug: "sealed-" + strings.ToLower(t.Name()[:6]), Name: "T", TargetURL: "https://tax.example.test/hook", Config: []byte(`{"apiKey":"sk_live_123"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var secret, cfg string
	if err := conn.QueryRow(`SELECT signing_secret, config FROM remote_providers WHERE id = $1`, res.Provider.ID).Scan(&secret, &cfg); err != nil {
		t.Fatal(err)
	}
	if !secrets.IsEncrypted(secret) || !secrets.IsEncrypted(cfg) || strings.Contains(cfg, "sk_live_123") {
		t.Errorf("stored secret %q / config %q - want both sealed", secret, cfg)
	}
	got, err := repo.GetSecret(ctx, res.Provider.ID)
	if err != nil || got != res.RawSecret {
		t.Errorf("secret read back as %q, %v", got, err)
	}
	p, err := repo.GetByID(ctx, res.Provider.ID)
	if err != nil || string(p.Config) != `{"apiKey":"sk_live_123"}` {
		t.Errorf("config read back as %s, %v", p.Config, err)
	}
	if _, err := NewRepository(db.New(conn), nil).GetSecret(ctx, res.Provider.ID); !errors.Is(err, ErrSealedWithoutKey) {
		t.Errorf("reading a sealed secret without the key: %v, want ErrSealedWithoutKey", err)
	}
}

// Rows written before the key was set are sealed by the boot sweep, and a second sweep has nothing left to do
func TestTheSweepSealsLegacyPlaintext(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	plain := NewService(NewRepository(db.New(conn), nil))
	plain.AllowPrivateTargets = true
	res, err := plain.Create(ctx, CreateInput{Kind: KindTaxProvider, Slug: "legacy-sweep", Name: "T", TargetURL: "https://tax.example.test/hook", Config: []byte(`{"apiKey":"k"}`)})
	if err != nil {
		t.Fatal(err)
	}
	sealed := NewRepository(db.New(conn), testSealer(t))
	if n, err := sealed.SealPlaintext(ctx); err != nil || n < 1 {
		t.Fatalf("sweep sealed %d, %v", n, err)
	}
	var secret, cfg string
	if err := conn.QueryRow(`SELECT signing_secret, config FROM remote_providers WHERE id = $1`, res.Provider.ID).Scan(&secret, &cfg); err != nil {
		t.Fatal(err)
	}
	if !secrets.IsEncrypted(secret) || !secrets.IsEncrypted(cfg) {
		t.Errorf("after the sweep: secret %q, config %q", secret, cfg)
	}
	if got, err := sealed.GetSecret(ctx, res.Provider.ID); err != nil || got != res.RawSecret {
		t.Errorf("sealed secret reads back as %q, %v", got, err)
	}
	if n, err := sealed.SealPlaintext(ctx); err != nil || n != 0 {
		t.Errorf("second sweep rewrote %d rows, %v", n, err)
	}
}
