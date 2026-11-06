package spinech

import (
	"context"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
)

func TestQueriesRunUnderTheLimitsAndADSNOverridesThem(t *testing.T) {
	ctx := context.Background()
	dsn := chtest.DSN(t)
	setting := func(dsn, name string) string {
		t.Helper()
		st, err := New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		var v string
		if err := st.conn.QueryRow(ctx, "SELECT toString(getSetting(?))", name).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := setting(dsn, "max_execution_time"); got != "60" {
		t.Errorf("max_execution_time = %s, want the default 60", got)
	}
	if got := setting(dsn, "max_memory_usage"); got != "4294967296" {
		t.Errorf("max_memory_usage = %s, want the default 4 GiB", got)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	if got := setting(dsn+sep+"max_execution_time=5", "max_execution_time"); got != "5" {
		t.Errorf("with the DSN's own value, max_execution_time = %s, want 5", got)
	}
}
