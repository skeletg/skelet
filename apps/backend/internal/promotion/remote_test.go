package promotion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

// buildRemoteStack spins up a remoteprovider service+invoker backed by an
// in-memory repo with one provider pointing at srv
func buildRemoteStack(t *testing.T, kind remoteprovider.Kind, url string, policy remoteprovider.FailurePolicy) (*remoteprovider.Service, *remoteprovider.Invoker, remoteprovider.Provider) {
	t.Helper()
	repo := remoteprovider.NewMemRepo()
	svc := remoteprovider.NewService(repo)
	svc.AllowPrivateTargets = true
	res, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: kind, Slug: "promo-svc", Name: "Promo", TargetURL: url, FailurePolicy: policy,
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return svc, remoteprovider.NewInvoker(svc, nil), res.Provider
}

func testEnv() Environment {
	ch := uuid.New()
	return BuildEnvironment(EnvironmentInput{
		SubtotalCents: 10000, Currency: "USD", CountryCode: "US", ChannelID: &ch,
		Lines: []CartLineView{{LineID: uuid.New(), ProductSKU: "SKU-1", Quantity: 2, UnitPriceCents: 5000, ProductKind: "simple"}},
	})
}

func TestRemoteConditionEvaluate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the invoker wraps our request under the envelope's "payload" key
		var body struct {
			Payload struct {
				Cart remoteCartContext `json:"cart"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		// match only when subtotal >= 5000 - proves the cart context arrived
		fmt.Fprintf(w, `{"match": %v}`, body.Payload.Cart.SubtotalCents >= 5000)
	}))
	defer srv.Close()

	svc, invoker, _ := buildRemoteStack(t, remoteprovider.KindPromotionCondition, srv.URL, remoteprovider.FailOpen)
	cond := &remoteCondition{svc: svc, invoker: invoker}
	params := json.RawMessage(`{"provider": "promo-svc"}`)

	match, err := cond.Evaluate(context.Background(), testEnv(), params)
	if err != nil || !match {
		t.Fatalf("expected match, got match=%v err=%v", match, err)
	}
}

func TestRemoteConditionFailOpen(t *testing.T) {
	svc, invoker, _ := buildRemoteStack(t, remoteprovider.KindPromotionCondition, "http://127.0.0.1:9/down", remoteprovider.FailOpen)
	cond := &remoteCondition{svc: svc, invoker: invoker}
	match, err := cond.Evaluate(context.Background(), testEnv(), json.RawMessage(`{"provider": "promo-svc"}`))
	if err != nil || match {
		t.Fatalf("fail_open condition should be a silent no-match, got match=%v err=%v", match, err)
	}
}

func TestRemoteActionFoldsIntoLedger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"subtotalDiscountCents": 500, "freeShipping": true, "freeGifts": [{"sku": "GIFT", "quantity": 1}]}`))
	}))
	defer srv.Close()

	svc, invoker, _ := buildRemoteStack(t, remoteprovider.KindPromotionAction, srv.URL, remoteprovider.FailOpen)
	action := &remoteAction{svc: svc, invoker: invoker}
	ledger := newLedger()

	if err := action.Apply(context.Background(), testEnv(), json.RawMessage(`{"provider": "promo-svc"}`), ledger, AppliedRuleContext{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if ledger.SubtotalDiscountCents() != 500 {
		t.Fatalf("discount not applied: %d", ledger.SubtotalDiscountCents())
	}
	if !ledger.FreeShipping() {
		t.Fatal("free shipping not set")
	}
	if len(ledger.FreeGifts()) != 1 || ledger.FreeGifts()[0].ProductSKU != "GIFT" {
		t.Fatalf("free gift not added: %+v", ledger.FreeGifts())
	}
}

func TestRemoteActionFailOpenNoDiscount(t *testing.T) {
	svc, invoker, _ := buildRemoteStack(t, remoteprovider.KindPromotionAction, "http://127.0.0.1:9/down", remoteprovider.FailOpen)
	action := &remoteAction{svc: svc, invoker: invoker}
	ledger := newLedger()
	if err := action.Apply(context.Background(), testEnv(), json.RawMessage(`{"provider": "promo-svc"}`), ledger, AppliedRuleContext{}); err != nil {
		t.Fatalf("fail_open action should be a silent no-op, got %v", err)
	}
	if ledger.SubtotalDiscountCents() != 0 {
		t.Fatalf("expected no discount, got %d", ledger.SubtotalDiscountCents())
	}
}

func TestRemoteParamsValidation(t *testing.T) {
	if err := (&remoteCondition{}).ValidateParams(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected provider-required error")
	}
	if err := (&remoteAction{}).ValidateParams(json.RawMessage(`{"provider": "x"}`)); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
}

// Without them a remote condition sees only SKU/name/qty/price/kind, so scoping by category or attribute meant calling back into the API
// per evaluation - on the checkout hot path, on every cart mutation - while an in-process plugin condition got them pre-resolved
func TestRemoteCartContextCarriesCategoriesAndAttributes(t *testing.T) {
	productID, catA, catB := uuid.New(), uuid.New(), uuid.New()
	env := BuildEnvironment(EnvironmentInput{
		SubtotalCents: 10000, Currency: "USD", CountryCode: "US",
		Lines: []CartLineView{{
			LineID: uuid.New(), ProductID: productID, ProductSKU: "SKU-1",
			Quantity: 1, UnitPriceCents: 10000, ProductKind: "simple",
			CategoryIDs:  []uuid.UUID{catA, catB},
			AttributeKVs: map[string]string{"brand": "Acme", "color": "black"},
		}},
	})

	got := buildRemoteCartContext(env)
	if len(got.Lines) != 1 {
		t.Fatalf("lines: got %d, want 1", len(got.Lines))
	}
	line := got.Lines[0]
	if line.ProductID != productID.String() {
		t.Errorf("productId: got %q, want %q", line.ProductID, productID)
	}
	if len(line.CategoryIDs) != 2 || line.CategoryIDs[0] != catA.String() || line.CategoryIDs[1] != catB.String() {
		t.Errorf("categoryIds: got %v, want [%s %s]", line.CategoryIDs, catA, catB)
	}
	if line.Attributes["brand"] != "Acme" || line.Attributes["color"] != "black" {
		t.Errorf("attributes: got %v", line.Attributes)
	}
}

// omitempty keeps the payload unchanged for lines with neither, so existing providers see no diff
func TestRemoteCartContextOmitsEmptyEnrichment(t *testing.T) {
	raw, err := json.Marshal(buildRemoteCartContext(testEnv()))
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"categoryIds", "attributes"} {
		if strings.Contains(string(raw), absent) {
			t.Errorf("empty %s was serialized - existing providers would see a changed payload: %s", absent, raw)
		}
	}
}
