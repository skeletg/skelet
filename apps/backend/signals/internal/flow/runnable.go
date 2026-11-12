package flow

import (
	"fmt"
	"strings"
)

var stepNames = map[string]string{
	NodeSendEmail:  "a send-email step",
	NodeSendSMS:    "a send-SMS step",
	NodeSplit:      "a split",
	NodeAddSegment: "an add-to-segment step",
	NodeWebhook:    "a webhook step",
}

// runnable is the first thing that would go wrong if the flow were switched on as it stands. A save takes a
// half-built flow; activating is where it has to be whole, whoever asks
func (g Graph) runnable() error {
	trig, ok := g.triggerNode()
	if !ok {
		return fmt.Errorf("%w: it needs a trigger node before it can be activated", ErrInvalid)
	}
	if _, ok := g.next(trig.ID); !ok {
		return fmt.Errorf("%w: nothing follows the trigger, so nobody would be enrolled", ErrInvalid)
	}
	const walking, done = 1, 2
	seen := map[string]int{}
	var walk func(id string) error
	walk = func(id string) error {
		switch seen[id] {
		case walking:
			return fmt.Errorf("%w: it loops back on itself, so a customer would never finish it", ErrInvalid)
		case done:
			return nil
		}
		n, ok := g.node(id)
		if !ok {
			return fmt.Errorf("%w: a connection leads to a step that is no longer there", ErrInvalid)
		}
		if missing := n.missing(); missing != "" {
			return fmt.Errorf("%w: %s %s", ErrInvalid, stepNames[n.Type], missing)
		}
		seen[id] = walking
		for _, next := range g.ways(n) {
			if err := walk(next); err != nil {
				return err
			}
		}
		seen[id] = done
		return nil
	}
	return walk(trig.ID)
}

func (g Graph) ways(n Node) []string {
	var out []string
	if n.Type == NodeSplit {
		for _, handle := range []string{"yes", "no"} {
			if next, ok := g.branch(n.ID, handle); ok {
				out = append(out, next)
			}
		}
		return out
	}
	if next, ok := g.next(n.ID); ok {
		out = append(out, next)
	}
	return out
}

// What a step still needs before it can do its job, or "" - a step no run can reach is never asked
func (n Node) missing() string {
	blank := func(s string) bool { return strings.TrimSpace(s) == "" }
	switch n.Type {
	case NodeSendEmail:
		if blank(n.Data.EmailID) && len(n.Data.Blocks) == 0 {
			return "has no email picked"
		}
	case NodeSendSMS:
		if blank(n.Data.SMSBody) {
			return "has no message"
		}
	case NodeSplit:
		c := n.Data.Condition
		if c == nil || ((c.Kind == "segment" || c.Kind == conditionInSegmentAlias) && blank(c.SegmentID)) {
			return "has no segment to branch on"
		}
	case NodeAddSegment:
		if blank(n.Data.SegmentID) {
			return "has no segment picked"
		}
	case NodeWebhook:
		if blank(n.Data.URL) {
			return "has no URL to call"
		}
	}
	return ""
}
