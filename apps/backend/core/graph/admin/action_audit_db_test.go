package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/action"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type auditedRun struct {
	adminID uuid.NullUUID
	success bool
	detail  map[string]any
}

func auditedRuns(t *testing.T, conn *sql.DB, slug string) []auditedRun {
	t.Helper()
	rows, err := conn.Query(`SELECT admin_id, success, detail FROM audit_log WHERE action = $1 ORDER BY created_at, id`, "admin_action:"+slug)
	if err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []auditedRun
	for rows.Next() {
		var run auditedRun
		var raw []byte
		if err := rows.Scan(&run.adminID, &run.success, &raw); err != nil {
			t.Fatalf("scan audit_log: %v", err)
		}
		if err := json.Unmarshal(raw, &run.detail); err != nil {
			t.Fatalf("detail is not a JSON object: %v", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	return out
}

func TestAdminActionRunsAreAuditedWithTheirRealOutcome(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)

	slug := "test.audit_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	action.Register(action.Definition{
		Slug:       slug,
		Name:       "Audit probe",
		Permission: "cache.manage",
		Arguments: []action.Argument{
			{Name: "mode", Type: action.ArgString},
			{Name: "api_key", Type: action.ArgString},
		},
		Handler: func(_ context.Context, args action.Args) (action.Result, error) {
			if mode, _ := args.String("mode"); mode == "fail" {
				return action.Result{}, errors.New("probe refused")
			}
			return action.Result{Output: "probe ran"}, nil
		},
	})

	r := &mutationResolver{&Resolver{AdminSvc: f.svc, ActionSvc: action.NewService(), AuditSvc: audit.NewService(db.New(conn))}}
	permitted, permittedCtx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "cache.manage"}))
	_, deniedCtx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "audit.view"}))

	if res, err := r.RunAdminAction(permittedCtx, slug, map[string]any{"api_key": "sk_live_hunter2"}); err != nil || !res.Success {
		t.Fatalf("successful run = %+v, %v", res, err)
	}
	if res, err := r.RunAdminAction(permittedCtx, slug, map[string]any{"mode": "fail"}); err != nil || res.Success {
		t.Fatalf("failing run = %+v, %v, want success=false with no GraphQL error", res, err)
	}
	if _, err := r.RunAdminAction(deniedCtx, slug, map[string]any{}); !forbidden(err) {
		t.Fatalf("unpermitted run err = %v, want forbidden", err)
	}

	runs := auditedRuns(t, conn, slug)
	if len(runs) != 3 {
		t.Fatalf("got %d audit rows, want one per attempt (3)", len(runs))
	}
	ok, failed, denied := runs[0], runs[1], runs[2]

	if !ok.success || ok.detail["output"] != "probe ran" || ok.adminID.UUID != permitted {
		t.Errorf("successful run audited as %+v", ok)
	}
	if args, _ := ok.detail["args"].(map[string]any); args["api_key"] != "[REDACTED]" {
		t.Errorf("secret argument stored as %v, want it redacted", args["api_key"])
	}
	if failed.success || failed.detail["output"] != "probe refused" {
		t.Errorf("failed run audited as %+v, want success=false carrying the handler error", failed)
	}
	if denied.success {
		t.Errorf("refused run audited as a success")
	}
}
