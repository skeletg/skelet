package promotion_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func TestACodeRunOnlyReachesARuleInsideItsGatedChannel(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	repo := newFakeRepo()
	onA := promotion.Rule{ID: uuid.New(), Name: "a", ChannelID: &a}
	global := promotion.Rule{ID: uuid.New(), Name: "global"}
	repo.rules = []promotion.Rule{onA, global}
	res := promotion.CodeResource(promotion.NewService(repo))
	run := func(rule promotion.Rule, ch uuid.UUID) error {
		o := dataio.NewOpts(map[string]string{"rule_id": rule.ID.String()})
		o.ChannelID = ch
		_, err := res.NewExporter(context.Background(), o)
		return err
	}
	if err := run(onA, a); err != nil {
		t.Errorf("a run on channel A refused channel A's rule: %v", err)
	}
	if run(onA, b) == nil {
		t.Error("a run gated on channel B reached channel A's rule - codes are effectively money")
	}
	if run(global, b) == nil {
		t.Error("a run gated on one channel reached a global rule, which needs the grant everywhere")
	}
	if run(onA, uuid.Nil) != nil || run(global, uuid.Nil) != nil {
		t.Error("an untargeted run is gated globally and may reach any rule")
	}
}
