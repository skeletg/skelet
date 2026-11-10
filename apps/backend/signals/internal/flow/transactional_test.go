package flow

import (
	"encoding/json"
	"testing"
)

// A run's email goes out in the language its moment carried, else the one the customer last signed in with
func TestARunsLanguage(t *testing.T) {
	carried := FlowRun{Context: json.RawMessage(`{"locale":"de","order":{"number":"1"}}`)}
	if got := runLocale(carried, Contact{Locale: "fr"}); got != "de" {
		t.Errorf("a moment in de with a customer in fr = %q, want de", got)
	}
	if got := runLocale(FlowRun{Context: json.RawMessage(`{"cart_id":"x"}`)}, Contact{Locale: "fr"}); got != "fr" {
		t.Errorf("a moment with no language = %q, want the customer's fr", got)
	}
	if got := runLocale(FlowRun{}, Contact{}); got != "" {
		t.Errorf("nothing known = %q, want the email as written", got)
	}
}
