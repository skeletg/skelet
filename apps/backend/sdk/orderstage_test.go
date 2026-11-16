package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRegisterOrderStage(t *testing.T) {
	t.Cleanup(orderStages.reset)
	acme := &Plugin{slug: "acme"}
	acme.RegisterOrderStage(OrderStage{Key: "awaiting_stock", Label: "Awaiting stock", Within: []string{OrderStatusConfirmed}})
	(&Plugin{slug: "other"}).RegisterOrderStage(OrderStage{Key: "awaiting_stock", Label: "Theirs", Within: []string{OrderStatusConfirmed}})
	got := OrderStages()
	if len(got) != 2 || got[0].PluginSlug != "acme" || got[0].Stage.Within[0] != OrderStatusConfirmed || got[0].Key() != "acme:awaiting_stock" || got[1].Key() != "other:awaiting_stock" {
		t.Fatalf("registered %+v", got)
	}
	got[0].Stage.Within[0] = "changed"
	if OrderStages()[0].Stage.Within[0] != OrderStatusConfirmed {
		t.Error("a caller changed the registry through its copy")
	}
	for name, s := range map[string]OrderStage{
		"a taken key":     {Key: "awaiting_stock", Label: "Again", Within: []string{OrderStatusConfirmed}},
		"a bad key":       {Key: "Awaiting stock", Label: "X", Within: []string{OrderStatusConfirmed}},
		"no label":        {Key: "picking", Within: []string{OrderStatusProcessing}},
		"no statuses":     {Key: "picking", Label: "Picking"},
		"a draft":         {Key: "picking", Label: "Picking", Within: []string{"draft"}},
		"an unknown kind": {Key: "picking", Label: "Picking", Within: []string{"shipped"}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.HasPrefix(r, "sdk.RegisterOrderStage") {
					t.Errorf("panic %q", r)
				}
			}()
			acme.RegisterOrderStage(s)
		})
	}
}

func TestSetOrderStageNeedsAHostThatMovesStages(t *testing.T) {
	if err := (&Plugin{slug: "acme"}).SetOrderStage(context.Background(), uuid.New(), "picking"); !errors.Is(err, ErrOrderStagesUnavailable) {
		t.Errorf("without a host: %v", err)
	}
}
