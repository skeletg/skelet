package experiment

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func validInput() Input {
	return Input{
		ChannelID: uuid.New(),
		Key:       "pdp-cta-color",
		Name:      "PDP CTA color",
		Status:    StatusDraft,
		Coverage:  100,
		GoalEvent: "add_to_cart",
		Variations: []Variation{
			{Key: "control", Weight: 50, Value: "orange"},
			{Key: "blue", Weight: 50, Value: "blue"},
		},
	}
}

func TestValidateAcceptsAWellFormedExperiment(t *testing.T) {
	if err := validate(validInput()); err != nil {
		t.Errorf("validate = %v, want the well-formed input accepted", err)
	}
}

func TestValidateRefusalsNameTheProblem(t *testing.T) { // each broken field must fail on its own, and the message is what the admin form shows
	for name, tc := range map[string]struct {
		mutate func(*Input)
		wants  string
	}{
		"empty key":            {func(in *Input) { in.Key = "" }, "key"},
		"whitespace key":       {func(in *Input) { in.Key = "   " }, "key"},
		"empty name":           {func(in *Input) { in.Name = "\t" }, "name"},
		"made-up status":       {func(in *Input) { in.Status = "paused" }, "status"},
		"empty status":         {func(in *Input) { in.Status = "" }, "status"},
		"cased status":         {func(in *Input) { in.Status = "Running" }, "status"},
		"coverage below zero":  {func(in *Input) { in.Coverage = -1 }, "coverage"},
		"coverage above 100":   {func(in *Input) { in.Coverage = 101 }, "coverage"},
		"one variation":        {func(in *Input) { in.Variations = in.Variations[:1] }, "2 variations"},
		"no variations":        {func(in *Input) { in.Variations = nil }, "2 variations"},
		"blank variation key":  {func(in *Input) { in.Variations[1].Key = " " }, "key"},
		"duplicate variations": {func(in *Input) { in.Variations[1].Key = "control" }, "duplicate"},
		"negative weight":      {func(in *Input) { in.Variations[0].Weight = -10 }, "negative"},
	} {
		in := validInput()
		tc.mutate(&in)
		err := validate(in)
		if err == nil {
			t.Errorf("%s: err = nil, want a refusal", name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("%s: err = %q, want it to mention %q", name, err, tc.wants)
		}
	}
}

func TestValidateAllowsTheBoundaryValues(t *testing.T) { // 0 coverage is a staged rollout waiting to start and 0 weight parks a variation - both are legitimate states, not errors
	in := validInput()
	in.Coverage = 0
	in.Variations[0].Weight = 0
	if err := validate(in); err != nil {
		t.Errorf("validate = %v, want zero coverage and a zero-weight variation accepted", err)
	}
	in = validInput()
	in.Coverage = 100
	if err := validate(in); err != nil {
		t.Errorf("validate = %v, want full coverage accepted", err)
	}
}

func TestValidateEveryStatus(t *testing.T) {
	for _, st := range []string{StatusDraft, StatusRunning, StatusStopped} {
		in := validInput()
		in.Status = st
		if err := validate(in); err != nil {
			t.Errorf("status %q: %v, want it accepted", st, err)
		}
	}
}
