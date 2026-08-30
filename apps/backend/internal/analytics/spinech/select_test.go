package spinech

import (
	"context"
	"testing"
)

func TestSelectFromEnv_DefaultIsPostgres(t *testing.T) {
	t.Setenv("SIGNALS_ENGINE", "")
	store, cleanup, err := SelectFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if store != nil {
		t.Fatalf("want nil store (postgres default), got %T", store)
	}
	cleanup() // must be safe to call
}

func TestSelectFromEnv_ClickhouseRequiresURL(t *testing.T) {
	t.Setenv("SIGNALS_ENGINE", "clickhouse")
	t.Setenv("SIGNALS_CH_URL", "")
	if _, _, err := SelectFromEnv(context.Background()); err == nil {
		t.Fatal("expected error when SIGNALS_CH_URL is empty")
	}
}
