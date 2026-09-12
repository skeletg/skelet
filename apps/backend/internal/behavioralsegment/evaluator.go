package behavioralsegment

// The boolean-tree evaluator: AND groups intersect their children, OR groups union them, and a
// negated event leaf takes the complement against the channel's customers

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
)

// universe is fetched at most once, and only when a complement actually needs it
type nodeEvaluator struct {
	rec      *analytics.Recorder
	cid      uuid.UUID
	now      time.Time
	universe map[uuid.UUID]bool
}

func (ev *nodeEvaluator) channelUniverse(ctx context.Context) (map[uuid.UUID]bool, error) {
	if ev.universe != nil {
		return ev.universe, nil
	}
	ids, err := ev.rec.ChannelCustomerIDs(ctx, ev.cid)
	if err != nil {
		return nil, err
	}
	ev.universe = sliceToSet(ids)
	return ev.universe, nil
}

func (ev *nodeEvaluator) eval(ctx context.Context, n *Node) (map[uuid.UUID]bool, error) {
	switch n.Kind {
	case KindGroup:
		return ev.evalGroup(ctx, n)
	case KindEvent:
		ids, err := ev.rec.EventActors(ctx, ev.cid, n.Event, n.Count, n.Days, ev.now)
		if err != nil {
			return nil, err
		}
		set := sliceToSet(ids)
		if n.Negate {
			uni, err := ev.channelUniverse(ctx)
			if err != nil {
				return nil, err
			}
			return complementSet(uni, set), nil
		}
		return set, nil
	case KindTrait:
		ids, err := ev.rec.TraitActors(ctx, ev.cid, n.Trait.Rule())
		if err != nil {
			return nil, err
		}
		return sliceToSet(ids), nil
	case KindAffinity:
		if !n.Affinity.Active() {
			return map[uuid.UUID]bool{}, nil
		}
		ids, err := ev.rec.AffinityActors(ctx, ev.cid, n.Affinity.Attr, n.Affinity.Value, ev.now.AddDate(0, 0, -n.Affinity.Window()))
		if err != nil {
			return nil, err
		}
		return sliceToSet(ids), nil
	default:
		return nil, ErrUnknownKind
	}
}

func (ev *nodeEvaluator) evalGroup(ctx context.Context, n *Node) (map[uuid.UUID]bool, error) {
	if len(n.Children) == 0 {
		return nil, ErrEmptyGroup
	}
	or := strings.EqualFold(n.Op, OpOr)
	var acc map[uuid.UUID]bool
	for i := range n.Children {
		child := n.Children[i]
		set, err := ev.eval(ctx, &child)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			acc = set
			continue
		}
		if or {
			unionInto(acc, set)
		} else {
			acc = intersectSets(acc, set)
		}
	}
	return acc, nil
}

func sliceToSet(ids []uuid.UUID) map[uuid.UUID]bool {
	s := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

func setToSlice(s map[uuid.UUID]bool) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(s))
	for id := range s {
		out = append(out, id)
	}
	return out
}

func unionInto(dst, src map[uuid.UUID]bool) {
	for id := range src {
		dst[id] = true
	}
}

func intersectSets(a, b map[uuid.UUID]bool) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool)
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for id := range small {
		if large[id] {
			out[id] = true
		}
	}
	return out
}

func complementSet(universe, exclude map[uuid.UUID]bool) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool, len(universe))
	for id := range universe {
		if !exclude[id] {
			out[id] = true
		}
	}
	return out
}
