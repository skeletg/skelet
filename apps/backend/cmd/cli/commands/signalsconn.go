package commands

import (
	"database/sql"
	"fmt"
	"os"

	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
)

func signalsConn() (*sql.DB, error) {
	url := os.Getenv("SIGNALS_DB_URL") // no DATABASE_URL fallback - the commerce schema carries no Signals tables, so it would just hit missing ones
	if url == "" {
		return nil, fmt.Errorf("SIGNALS_DB_URL is not set - Signals lives in its own database (a second database on the commerce instance is fine)")
	}
	conn, err := dbconn.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect to signals database: %w", err)
	}
	return conn, nil
}
