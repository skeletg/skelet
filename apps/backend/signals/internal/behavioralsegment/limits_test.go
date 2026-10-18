package behavioralsegment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDefinitionsAreBounded(t *testing.T) {
	leaf := Node{Kind: KindEvent, Event: "order_placed", Count: 1}
	wide := Node{Kind: KindGroup, Op: OpOr}
	for range maxNodes {
		wide.Children = append(wide.Children, leaf)
	}
	deep := leaf
	for range maxDepth {
		deep = Node{Kind: KindGroup, Children: []Node{deep}}
	}
	for name, tc := range map[string]struct {
		root Node
		want error
	}{
		"too many nodes":    {wide, ErrTooLarge},
		"too deep":          {deep, ErrTooLarge},
		"no event name":     {Node{Kind: KindEvent, Count: 1}, ErrBadCondition},
		"negative count":    {Node{Kind: KindEvent, Event: "x", Count: -1}, ErrBadCondition},
		"negative days":     {Node{Kind: KindEvent, Event: "x", Days: -3}, ErrBadCondition},
		"unknown group op":  {Node{Kind: KindGroup, Op: "xor", Children: []Node{leaf}}, ErrBadCondition},
		"unknown kind":      {Node{Kind: "vibes"}, ErrUnknownKind},
		"negative affinity": {Node{Kind: KindAffinity, Affinity: &Affinity{Attr: "a", Value: "v", Days: -1}}, ErrBadCondition},
	} {
		if err := validateTree(&tc.root); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	ok := Node{Kind: KindGroup, Op: OpAnd, Children: []Node{leaf, {Kind: KindTrait, Trait: &Trait{MinEngagement: 1}}}}
	if err := validateTree(&ok); err != nil {
		t.Errorf("an ordinary definition was refused: %v", err)
	}
}

// Every save and refresh evaluates first, so the bound has to hold there - before a single spine query runs
func TestEvaluateRefusesAnOversizedDefinitionBeforeQuerying(t *testing.T) {
	wide := Node{Kind: KindGroup, Op: OpOr}
	for range maxNodes + 1 {
		wide.Children = append(wide.Children, Node{Kind: KindEvent, Event: "x", Count: 1})
	}
	_, err := (&Service{}).Evaluate(context.Background(), Definition{ChannelID: uuid.NewString(), Root: &wide}, time.Now())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}
