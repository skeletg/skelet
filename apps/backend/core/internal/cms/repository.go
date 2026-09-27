package cms

import (
	"database/sql"
	"encoding/json"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) *postgresRepository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

func emptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataToJSON(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataFromJSON(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return map[string]string{}
	}
	return m
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
