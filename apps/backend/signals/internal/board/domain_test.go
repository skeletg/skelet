package board

import (
	"errors"
	"testing"
)

func TestParseDefinitionRefusesWhatItDoesntKnow(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown field":      `{"kind":"insights","evnt":"page_view"}`,
		"unknown kind":       `{"kind":"pie"}`,
		"one-step funnel":    `{"kind":"funnel","steps":["page_view"]}`,
		"filter off a step":  `{"kind":"funnel","steps":["a","b"],"filters":[{"step":2,"key":"device","value":"x"}]}`,
		"retention, no end":  `{"kind":"retention","startEvent":"order_placed"}`,
		"flow without focus": `{"kind":"flow"}`,
		"bad unit":           `{"kind":"insights","unit":"hour"}`,
		"period too long":    `{"kind":"insights","sinceDays":400}`,
		"not json":           `kind: insights`,
		"bad size":           `{"kind":"insights","size":"huge"}`,
		"window too long":    `{"kind":"stickiness","windowDays":400}`,
	} {
		if _, err := ParseDefinition([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	d, err := ParseDefinition([]byte(`{"kind":"funnel","steps":["page_view","order_placed"],"filters":[{"step":0,"key":"device","value":"mobile"}],"windowHours":24,"breakdown":"device"}`))
	if err != nil || d.WindowHours != 24 || len(d.Filters) != 1 || d.Breakdown != "device" {
		t.Fatalf("a valid funnel = %+v, %v", d, err)
	}
	if d, err := ParseDefinition([]byte(`{"kind":"insights","size":"full"}`)); err != nil || d.Size != "full" {
		t.Fatalf("a full-width tile = %+v, %v", d, err)
	}
	if d, err := ParseDefinition([]byte(`{"kind":"stickiness","windowDays":14}`)); err != nil || d.WindowDays != 14 {
		t.Fatalf("a 14-day return window = %+v, %v", d, err)
	}
}

func TestEveryStarterBoardIsValid(t *testing.T) {
	if len(Starters) != 4 {
		t.Fatalf("%d starter boards, want the four areas", len(Starters))
	}
	for _, b := range Starters {
		if err := b.validate(); err != nil {
			t.Errorf("%s: %v", b.Name, err)
		}
	}
}
