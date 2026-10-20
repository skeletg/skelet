package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func seedChannel(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ch := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'promo', 'USD')`, ch, "promo-"+ch.String()[:8]); err != nil {
		t.Fatal(err)
	}
	return ch
}

func seedRule(t *testing.T, repo Repository, channelID *uuid.UUID) Rule {
	t.Helper()
	r, err := repo.CreateRule(context.Background(), Rule{Slug: "r-" + uuid.NewString()[:8], Name: "R", IsActive: true, StopAfterApply: StopNone, ChannelID: channelID})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type failingActions struct{ Repository }

func (f failingActions) WithTx(t *sql.Tx) Repository { return failingActions{f.Repository.WithTx(t)} }
func (failingActions) ReplaceRuleActions(context.Context, uuid.UUID, []Action) error {
	return errors.New("actions write failed")
}

// Every cart evaluation reads a rule's conditions - an edit that cleared them and then failed left "gold tier only, 50% off" open to everyone
func TestARuleEditIsAllOrNothing(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	rule := seedRule(t, repo, nil)
	if err := repo.ReplaceRuleConditions(ctx, rule.ID, []Condition{{Type: "core.customer_group", Params: json.RawMessage(`{"slugs":["gold"]}`)}}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(failingActions{repo})
	svc.SetTx(txrunner.New(conn))
	none, noActions := []Condition{}, []Action{}
	if _, err := svc.UpdateRule(ctx, UpdateRuleInput{Rule: rule, Conditions: &none, Actions: &noActions}); err == nil {
		t.Fatal("a failed actions write reported success")
	}
	conds, err := repo.ListConditionsByRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conds) != 1 {
		t.Errorf("the rule has %d conditions after a failed edit, want its original 1", len(conds))
	}
}

// A guest has no account to count against, so the order email is the shopper - a once-per-customer code was reusable by checking out as a guest
func TestShopperRedemptionsCountAGuestByOrderEmail(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	ch := seedChannel(t, conn)
	rule := seedRule(t, repo, &ch)
	code, err := repo.CreateCode(ctx, Code{RuleID: rule.ID, Code: "once-" + uuid.NewString()[:6], IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	var orderID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, email) VALUES ($1, 'USD', ' Guest@Example.test ') RETURNING id`, ch).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordRedemption(ctx, rule.ID, &code.ID, nil, orderID, 500); err != nil {
		t.Fatal(err)
	}
	byRule, byCode, err := repo.ShopperRedemptions(ctx, NewShopper(nil, "guest@example.TEST"), []uuid.UUID{rule.ID}, []uuid.UUID{code.ID})
	if err != nil {
		t.Fatal(err)
	}
	if byRule[rule.ID] != 1 || byCode[code.ID] != 1 {
		t.Errorf("guest counts rule=%d code=%d, want 1 and 1", byRule[rule.ID], byCode[code.ID])
	}
	stranger := uuid.New()
	byRule, _, err = repo.ShopperRedemptions(ctx, NewShopper(&stranger, "other@example.test"), []uuid.UUID{rule.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if byRule[rule.ID] != 0 {
		t.Errorf("another shopper counted %d of the guest's redemptions", byRule[rule.ID])
	}
}

// Paged in SQL - filtering a page after LIMIT left a scoped admin an empty first page with their rules unreachable behind it
func TestListRulesPagesThroughOnlyTheScope(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	a, b := seedChannel(t, conn), seedChannel(t, conn)
	mine := []uuid.UUID{seedRule(t, repo, &a).ID, seedRule(t, repo, &a).ID}
	other := seedRule(t, repo, &b).ID
	global := seedRule(t, repo, nil).ID

	page, total, err := repo.ListRules(ctx, RuleFilter{ChannelIDs: []uuid.UUID{a}}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(page) != 1 || !slices.Contains(mine, page[0].ID) {
		t.Errorf("scoped page 2: total %d, %d rows - want 2 and one of the channel's own", total, len(page))
	}
	withGlobal, _, err := repo.ListRules(ctx, RuleFilter{ChannelIDs: []uuid.UUID{a}, IncludeGlobal: true}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, len(withGlobal))
	for i, r := range withGlobal {
		ids[i] = r.ID
	}
	if !slices.Contains(ids, global) || slices.Contains(ids, other) {
		t.Errorf("channel + global listing: global %v, other channel %v - want true, false", slices.Contains(ids, global), slices.Contains(ids, other))
	}
}

// ApplyCode uppercases what the shopper types, so every write has to store that form or the code can never be redeemed
func TestCodeWritesStoreTheRedeemableForm(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	rule := seedRule(t, repo, nil)
	sfx := uuid.NewString()[:6]
	first, err := repo.CreateCode(ctx, Code{RuleID: rule.ID, Code: "  welcome-" + sfx + " ", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Code != normalizeCode("WELCOME-"+sfx) {
		t.Errorf("stored %q", first.Code)
	}
	second, err := repo.CreateCode(ctx, Code{RuleID: rule.ID, Code: "other-" + sfx, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	second.Code = "welcome-" + sfx
	if _, err := repo.UpdateCode(ctx, second); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("renaming onto a taken code: %v, want ErrCodeTaken", err)
	}

	// the import counts a row as created when it reads it, so a code another rule owns comes back as that row's error
	other := seedRule(t, repo, nil)
	im := &codeImporter{svc: NewService(repo), rule: other}
	for i, c := range []string{"welcome-" + sfx, "fresh-" + sfx} {
		row := dataio.NewRow()
		row.Line = i + 2
		row.Set(colCode, c)
		if _, err := im.Row(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	errs, err := im.Finish(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Line != 2 {
		t.Errorf("import errors %+v, want one for line 2, the taken code", errs)
	}
}

// The codes table pages on the server, so its search and Active filter must narrow the total too - not just the page
func TestCodeSearchNarrowsTheTotal(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	rule := seedRule(t, repo, nil)
	for _, c := range []Code{{Code: "SPRING-1", IsActive: true}, {Code: "SPRING-2", IsActive: false}, {Code: "AUTUMN-1", IsActive: true}} {
		c.RuleID = rule.ID
		if _, err := repo.CreateCode(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	active := true
	for _, tc := range []struct {
		search string
		active *bool
		want   int32
	}{{"", nil, 3}, {"spring", nil, 2}, {"spring", &active, 1}, {"", &active, 2}, {"winter", nil, 0}} {
		codes, total, err := repo.ListCodesByRule(ctx, rule.ID, tc.search, tc.active, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != tc.want || len(codes) != min(1, int(tc.want)) {
			t.Errorf("search %q active %v: %d rows of %d total, want %d total", tc.search, tc.active, len(codes), total, tc.want)
		}
	}
}
