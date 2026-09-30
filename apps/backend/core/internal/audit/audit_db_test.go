package audit

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestRecordStoresASecretSettingSaveRedacted(t *testing.T) {
	conn := testdb.Require(t)
	svc := NewService(db.New(conn))
	ctx := context.Background()
	action := "AuditTest" + uuid.NewString()[:8]

	svc.Record(ctx, Entry{Action: action, Detail: map[string]any{"path": testSecretPath, "value": "sk-live-123"}, Success: true})
	rows, total, err := svc.List(ctx, nil, &action, 10, -5) // a negative offset reads from the start instead of failing
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("List = (%d rows, total %d, %v)", len(rows), total, err)
	}
	if strings.Contains(string(rows[0].Detail), "sk-live-123") {
		t.Errorf("stored detail %s holds the secret", rows[0].Detail)
	}
}

// Rows written before the redaction existed get cleaned by migration 000014 - run it again over a fresh plaintext row
func TestMigrationRedactsSecretSettingRowsWrittenBefore(t *testing.T) {
	conn := testdb.Require(t)
	sqlText, err := os.ReadFile("../db/migrations/000014_audit_redact_secret_settings.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	insert := func(path, value string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO audit_log (action, detail) VALUES ('SetConfigValue', jsonb_build_object('path', $1::text, 'value', $2::text)) RETURNING id`, path, value).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM audit_log WHERE id = $1`, id) })
		return id
	}
	secret, plain := insert("ai/llm/api_key", "sk-live-123"), insert("general/contact/email_from_name", "Skelet")
	if _, err := conn.Exec(string(sqlText)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	value := func(id uuid.UUID) (v string) {
		_ = conn.QueryRow(`SELECT detail->>'value' FROM audit_log WHERE id = $1`, id).Scan(&v)
		return v
	}
	if got := value(secret); got != "[REDACTED]" {
		t.Errorf("the API key row still reads %q", got)
	}
	if got := value(plain); got != "Skelet" {
		t.Errorf("an ordinary setting row changed to %q", got)
	}
}
