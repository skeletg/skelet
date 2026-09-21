package spinech

import (
	"context"
	"testing"
)

func TestFromEnvRequiresURL(t *testing.T) {
	t.Setenv("SIGNALS_CH_URL", "")
	if _, _, err := FromEnv(context.Background()); err == nil {
		t.Fatal("expected an error when SIGNALS_CH_URL is empty")
	}
}
