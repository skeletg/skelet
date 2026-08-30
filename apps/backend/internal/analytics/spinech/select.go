package spinech

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/skeletg/skelet/internal/analytics"
)

func EngineIsClickHouse() bool { // for callers that only need to branch on the engine, without dialing it
	return strings.ToLower(strings.TrimSpace(os.Getenv("SIGNALS_ENGINE"))) == "clickhouse"
}

// Builds the ClickHouse spine driver when SIGNALS_ENGINE=clickhouse (DSN from SIGNALS_CH_URL), else returns a nil store meaning keep the default Postgres driver - functionally complete either way, Postgres is just the cheaper engine at small scale
func SelectFromEnv(ctx context.Context) (store analytics.SpineStore, cleanup func(), err error) {
	if !EngineIsClickHouse() {
		return nil, func() {}, nil
	}
	dsn := os.Getenv("SIGNALS_CH_URL")
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("SIGNALS_ENGINE=clickhouse but SIGNALS_CH_URL is empty")
	}
	st, err := New(ctx, dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("clickhouse spine: %w", err)
	}
	return st, func() { _ = st.Close() }, nil
}
