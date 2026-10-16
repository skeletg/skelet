package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
)

// Scenario: a third-party system moderates, credits and exports through the admin API with nothing but a token - each write is attributed to the integration, the audit trail names it, and what only a person can do refuses it by name
func TestIntegrationTokenActsLikeAnAdmin(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	f := h.NewStore(t, 10, 1500)
	buyer, _, _ := h.registeredBuyer(t, f)

	created, err := h.D.Integration.Create(h.Ctx, integration.CreateIntegrationInput{
		Name:        "e2e review bot",
		Permissions: []string{"reviews.moderate", "customers.manage", "catalog.view"},
	})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	tokID := created.Integration.ID
	defer func() { _ = h.D.Integration.Delete(h.Ctx, tokID) }()
	tok := created.RawToken

	data, errs := h.adminGqlPost(t, ts, tok, `query { channels { id } }`)
	if len(errs) > 0 || !listHasID(data["channels"], f.Channel.ID) {
		t.Errorf("channels as a token = %v (errors %v), want the store's channel listed", data["channels"], errs)
	}

	var reviewID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `INSERT INTO reviews (product_id, customer_id, channel_id, locale, title, body, is_verified_purchase, status)
		VALUES ($1, $2, $3, 'en', 'Fits well', 'True to size.', false, 'pending') RETURNING id`, f.Product.ID, buyer, f.Channel.ID).Scan(&reviewID); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	data, errs = h.adminGqlPost(t, ts, tok, fmt.Sprintf(`mutation E2EBotApprove { approveReview(id: %q) { status moderatedBy moderatedByIntegration } }`, reviewID))
	if len(errs) > 0 {
		t.Fatalf("approve as a token: %v", errs)
	}
	rev, _ := data["approveReview"].(map[string]any)
	if rev["status"] != "APPROVED" || rev["moderatedBy"] != nil || rev["moderatedByIntegration"] != tokID.String() {
		t.Errorf("approved review = %v, want APPROVED by integration %s and no admin", rev, tokID)
	}

	if _, errs = h.adminGqlPost(t, ts, tok, fmt.Sprintf(`mutation { adjustStoreCredit(customerId: %q, amountCents: 500, reason: "goodwill") { balanceCents } }`, buyer)); len(errs) > 0 {
		t.Fatalf("store credit as a token: %v", errs)
	}
	var creditBy uuid.NullUUID
	var creditAdmin uuid.NullUUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT integration_id, admin_id FROM store_credit_entries WHERE customer_id = $1`, buyer).Scan(&creditBy, &creditAdmin); err != nil {
		t.Fatalf("read credit entry: %v", err)
	}
	if creditBy.UUID != tokID || creditAdmin.Valid {
		t.Errorf("credit entry by integration %v admin %v, want integration %s only", creditBy, creditAdmin, tokID)
	}

	var audited int
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT count(*) FROM audit_log WHERE action = 'E2EBotApprove' AND integration_id = $1 AND admin_id IS NULL`, tokID).Scan(&audited); err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if audited != 1 {
		t.Errorf("audit rows naming the integration for the approval = %d, want 1", audited)
	}

	data, errs = h.adminGqlPost(t, ts, tok, `mutation { startDataExport(input: {resource: "categories", format: "csv"}) { adminId integrationId } }`)
	if len(errs) > 0 {
		t.Fatalf("export as a token: %v", errs)
	}
	if job, _ := data["startDataExport"].(map[string]any); job["integrationId"] != tokID.String() || job["adminId"] != nil {
		t.Errorf("export job = %v, want it attributed to integration %s only", job, tokID)
	}

	for _, q := range []string{`query { me { id } }`, `query { myPreferences { key } }`} {
		if _, errs := h.adminGqlPost(t, ts, tok, q); !errorsContain(errs, "integration tokens can't use it") {
			t.Errorf("%s as a token = %v, want the admin-only refusal", q, errs)
		}
	}

	other := h.NewStore(t, 1, 100)
	pinned, err := h.D.Integration.Create(h.Ctx, integration.CreateIntegrationInput{
		Name: "e2e pinned feed", Permissions: []string{"catalog.view"}, ChannelID: &f.Channel.ID,
	})
	if err != nil {
		t.Fatalf("create pinned integration: %v", err)
	}
	defer func() { _ = h.D.Integration.Delete(h.Ctx, pinned.Integration.ID) }()
	data, errs = h.adminGqlPost(t, ts, pinned.RawToken, `query { channels { id } }`)
	if len(errs) > 0 || !listHasID(data["channels"], f.Channel.ID) || listHasID(data["channels"], other.Channel.ID) {
		t.Errorf("channels as a pinned token = %v (errors %v), want only %s", data["channels"], errs, f.Channel.ID)
	}
}

func listHasID(v any, id uuid.UUID) bool {
	rows, _ := v.([]any)
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok && m["id"] == id.String() {
			return true
		}
	}
	return false
}
