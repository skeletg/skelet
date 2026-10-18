package audit

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestRecordStoresASecretSettingSaveRedacted(t *testing.T) {
	conn := testdb.Require(t)
	svc := NewService(db.New(conn))
	ctx := context.Background()
	action := "AuditTest" + uuid.NewString()[:8]

	svc.Record(ctx, Entry{Action: action, Detail: map[string]any{"path": testSecretPath, "value": "sk-live-123"}, Success: true})
	rows, total, err := svc.List(ctx, nil, nil, &action, 10, -5) // a negative offset reads from the start instead of failing
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("List = (%d rows, total %d, %v)", len(rows), total, err)
	}
	if strings.Contains(string(rows[0].Detail), "sk-live-123") {
		t.Errorf("stored detail %s holds the secret", rows[0].Detail)
	}
}

func TestRecordAttributesAnIntegration(t *testing.T) {
	conn := testdb.Require(t)
	svc := NewService(db.New(conn))
	var tokID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO integrations (name, token_hash) VALUES ('ERP sync', $1) RETURNING id`, uuid.NewString()).Scan(&tokID); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithApiToken(context.Background(), &auth.ApiTokenClaims{TokenID: tokID, Name: "ERP sync"})
	e := Entry{Action: "UpdateProduct", Success: true}
	Identify(ctx, &e)
	svc.Record(ctx, e)

	rows, total, err := svc.List(context.Background(), nil, &tokID, nil, 10, 0)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("List by integration = (%d rows, total %d, %v)", len(rows), total, err)
	}
	r := rows[0]
	if r.AdminID != nil || r.IntegrationID == nil || *r.IntegrationID != tokID || r.AdminEmail != "integration:ERP sync" {
		t.Errorf("row = admin %v, integration %v, email %q", r.AdminID, r.IntegrationID, r.AdminEmail)
	}
}
