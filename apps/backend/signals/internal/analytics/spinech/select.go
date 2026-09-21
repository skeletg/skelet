package spinech

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// The spine driver from SIGNALS_CH_URL - the suite has no other engine, so an empty value fails boot here rather than at the first beacon
func FromEnv(ctx context.Context) (store *Store, cleanup func(), err error) {
	dsn := os.Getenv("SIGNALS_CH_URL")
	if dsn == "" {
		return nil, nil, errors.New("SIGNALS_CH_URL is not set - the spine lives in ClickHouse, and the suite has no other engine for it")
	}
	st, err := New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("clickhouse spine: %w", err)
	}
	return st, func() { _ = st.Close() }, nil
}
