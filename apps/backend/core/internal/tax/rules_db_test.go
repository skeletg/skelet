package tax_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A rule whose effect fails to save must not be left live with its conditions - an active rule with no effects or partial conditions changes what every order pays
func TestARuleSavesWholeOrNotAtAll(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "tax-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	svc := tax.NewService(tax.NewRepository(db.New(conn)))
	svc.SetTx(txrunner.New(conn))

	// Valid to the registry, refused by jsonb - the effect insert fails after the rule and its condition are written
	poisoned := []tax.Effect{{Type: "core.percentage", Params: json.RawMessage(`{"bps":100,"note":"\u0000"}`)}}
	conds := []tax.Condition{{Type: "core.jurisdiction", Params: json.RawMessage(`{"country_code":"DE"}`)}}
	if _, err := svc.CreateRule(ctx, tax.CreateRuleInput{Rule: tax.Rule{ChannelID: ch, Name: "bad", IsActive: true}, Conditions: conds, Effects: poisoned}); err == nil {
		t.Fatal("a rule with an unsavable effect was created")
	}
	if rules, _ := svc.ListRulesByChannelHydrated(ctx, ch); len(rules) != 0 {
		t.Fatalf("a failed create left %d rules behind", len(rules))
	}

	good := []tax.Effect{{Type: "core.percentage", Params: json.RawMessage(`{"bps":1900}`)}}
	rule, err := svc.CreateRule(ctx, tax.CreateRuleInput{Rule: tax.Rule{ChannelID: ch, Name: "de", IsActive: true}, Conditions: conds, Effects: good})
	if err != nil {
		t.Fatal(err)
	}
	none := []tax.Condition{}
	rule.Conditions, rule.Effects = nil, nil
	if _, err := svc.UpdateRule(ctx, tax.UpdateRuleInput{Rule: rule, Conditions: &none, Effects: &poisoned}); err == nil {
		t.Fatal("an update with an unsavable effect succeeded")
	}
	got, err := svc.GetRuleHydrated(ctx, rule.ID)
	if err != nil || len(got.Conditions) != 1 || len(got.Effects) != 1 {
		t.Fatalf("after a failed update the rule has %d conditions and %d effects, want its 1 and 1", len(got.Conditions), len(got.Effects))
	}

	unknown := []tax.Effect{{Type: "core.nope"}}
	_, err = svc.CreateRule(ctx, tax.CreateRuleInput{Rule: tax.Rule{ChannelID: ch, Name: "x"}, Effects: unknown})
	if appErr, ok := apperr.AsAppError(err); !ok || appErr.Class != apperr.ClassValidation || appErr.Message == "" {
		t.Errorf("an unknown effect type = %v, want a validation error with its reason", err)
	}
}
