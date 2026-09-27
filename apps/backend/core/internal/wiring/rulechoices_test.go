package wiring

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type scopedTo struct{ ch uuid.UUID }

func (scopedTo) HasGlobalPermission(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (s scopedTo) ListPermittedChannelIDs(context.Context, uuid.UUID, string) ([]uuid.UUID, error) {
	return []uuid.UUID{s.ch}, nil
}

func TestTheRulePickerOffersOnlyTheAdminsOwnChannelsRules(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New()})
	rules := []promotion.Rule{{ID: uuid.New(), Name: "a", ChannelID: &a}, {ID: uuid.New(), Name: "b", ChannelID: &b}, {ID: uuid.New(), Name: "global"}}
	got, err := ruleChoices(ctx, scopedTo{ch: a}, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "a" {
		t.Fatalf("picker offered %v, want only channel A's rule", got)
	}
}
