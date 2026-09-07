package flow

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
)

// validate is the only gate between the flow composer and the run engine: a node the engine can't execute parks every enrolled customer
// mid-journey, and a trigger it never fires for means a flow that looks live in the dashboard and sends nothing

func validator() *Service {
	s := &Service{tmpl: emailtemplate.NewService(nil, nil, nil), conditions: NewConditionRegistry()}
	// validation accepts what is registered, so the suite-backed kinds have to be present for the split cases
	s.RegisterCondition(lifecycleCondition{})
	s.RegisterCondition(consentCondition{})
	s.RegisterCondition(segmentAliasCondition{})
	return s
}

func draft(nodes ...Node) Input {
	return Input{ChannelID: uuid.New(), Name: "Welcome", TriggerType: TriggerCustomerRegistered, Graph: Graph{Nodes: nodes}}
}

func withConfig(in Input, cfg string) Input {
	in.TriggerConfig = json.RawMessage(cfg)
	return in
}

func TestValidateGatesEveryNodeType(t *testing.T) {
	blank := draft()
	blank.Name = "   "
	badTrigger := draft()
	badTrigger.TriggerType = "moon_phase"

	for name, tc := range map[string]struct {
		in      Input
		wantErr bool
	}{
		"a nameless flow":                  {blank, true},
		"a trigger nothing fires":          {badTrigger, true},
		"a node type the engine can't run": {draft(Node{ID: "x", Type: "send_pigeon"}), true},
		"config-free nodes": {draft(
			Node{ID: "t", Type: NodeTrigger},
			Node{ID: "w", Type: NodeWait},
			Node{ID: "s", Type: NodeAddSegment},
			Node{ID: "h", Type: NodeWebhook},
		), false},
		"a split with no condition":        {draft(Node{ID: "s", Type: NodeSplit}), true},
		"a split on an unknown kind":       {draft(Node{ID: "s", Type: NodeSplit, Data: NodeData{Condition: &Condition{Kind: "vibes"}}}), true},
		"a split on the alias kind":        {draft(Node{ID: "s", Type: NodeSplit, Data: NodeData{Condition: &Condition{Kind: conditionInSegmentAlias}}}), false},
		"a split on a live kind":           {draft(Node{ID: "s", Type: NodeSplit, Data: NodeData{Condition: &Condition{Kind: "lifecycle"}}}), false},
		"an SMS node with no body":         {draft(Node{ID: "m", Type: NodeSendSMS}), true},
		"an SMS node with a body":          {draft(Node{ID: "m", Type: NodeSendSMS, Data: NodeData{SMSBody: "your order shipped"}}), false},
		"a remote action with no provider": {draft(Node{ID: "r", Type: NodeRemoteAction}), true},
		"a remote action with a provider":  {draft(Node{ID: "r", Type: NodeRemoteAction, Data: NodeData{Provider: "loyalty-cdp"}}), false},
		"an email with an unfinished block": {draft(Node{ID: "e", Type: NodeSendEmail, Data: NodeData{
			Blocks: []emailtemplate.Block{{Type: "video"}},
		}}), true},
		"an email with a text block": {draft(Node{ID: "e", Type: NodeSendEmail, Data: NodeData{
			Blocks: []emailtemplate.Block{{Type: "text"}},
		}}), false},
		"an unparseable triggerConfig":          {withConfig(draft(), `{"transactional":`), true},
		"an enrollment filter on a bad kind":    {withConfig(draft(), `{"enrollmentFilter":{"kind":"vibes"}}`), true},
		"an enrollment filter on a live kind":   {withConfig(draft(), `{"enrollmentFilter":{"kind":"consent","consentKind":"marketing_email"}}`), false},
		"a triggerConfig with no filter at all": {withConfig(draft(), `{"transactional":true}`), false},
	} {
		t.Run(name, func(t *testing.T) {
			err := validator().validate(tc.in)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validate err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEveryTriggerWeOfferCanBeSaved(t *testing.T) {
	// Both lists feed operator-facing behaviour - the composer's trigger dropdown and the
	// consent-bypass default - so an entry validate rejects is a flow that can never be saved
	for source, triggers := range map[string][]string{
		"offered by the composer":     WiredTriggerTypes(),
		"defaulting to transactional": TransactionalTriggers(),
	} {
		t.Run(source, func(t *testing.T) {
			if len(triggers) == 0 {
				t.Fatal("the list is empty; the drift check would pass vacuously")
			}
			for _, trig := range triggers {
				if !validTrigger(trig) {
					t.Errorf("trigger %q is %s but validate rejects it", trig, source)
				}
			}
		})
	}
}

func TestOnlyLifecycleMailBypassesTheConsentGate(t *testing.T) {
	// Transactional skips the marketing-consent check and suppression, so a marketing trigger drifting
	// into that default mails people who opted out
	for name, tc := range map[string]struct {
		trigger             string
		auth, transactional bool
	}{
		"email verification": {TriggerEmailVerification, true, true},
		"password reset":     {TriggerPasswordReset, true, true},
		"order placed":       {TriggerOrderPlaced, false, true},
		"order shipped":      {TriggerOrderShipped, false, true},
		"cart abandoned":     {TriggerCartAbandoned, false, false},
		"segment entered":    {TriggerSegmentEntered, false, false},
		"back in stock":      {TriggerBackInStock, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := IsAuthTrigger(tc.trigger); got != tc.auth {
				t.Errorf("IsAuthTrigger = %v, want %v", got, tc.auth)
			}
			if got := IsDefaultTransactional(tc.trigger); got != tc.transactional {
				t.Errorf("IsDefaultTransactional = %v, want %v", got, tc.transactional)
			}
		})
	}
}

func TestNextRefusesToFallThroughALabelledBranch(t *testing.T) {
	// A split's yes/no edges are decisions - walking one as the default "out" path would send the
	// branch's mail to everyone the condition excluded
	g := Graph{Edges: []Edge{
		{ID: "e1", Source: "split", Target: "yes-node", SourceHandle: "yes"},
		{ID: "e2", Source: "split", Target: "no-node", SourceHandle: "no"},
		{ID: "e3", Source: "wait", Target: "email"},
		{ID: "e4", Source: "email", Target: "done", SourceHandle: "out"},
	}}
	for name, tc := range map[string]struct {
		from   string
		want   string
		wantOK bool
	}{
		"an unlabelled edge":     {"wait", "email", true},
		"an explicit out handle": {"email", "done", true},
		"only labelled edges":    {"split", "", false},
		"no edges at all":        {"done", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := g.next(tc.from)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("next(%q) = (%q, %v), want (%q, %v)", tc.from, got, ok, tc.want, tc.wantOK)
			}
		})
	}
	if got, ok := g.branch("split", "yes"); got != "yes-node" || !ok {
		t.Errorf(`branch("split","yes") = (%q, %v), want ("yes-node", true)`, got, ok)
	}
	if _, ok := g.branch("split", "maybe"); ok {
		t.Error("an unwired handle resolved to a node")
	}
}
