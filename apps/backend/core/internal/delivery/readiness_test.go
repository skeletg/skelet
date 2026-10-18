package delivery

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestOverlapping(t *testing.T) {
	opt := func(kind, carrier string, enabled bool) Option {
		return Option{ID: uuid.New(), Kind: kind, Name: kind + "/" + carrier, CarrierSlug: carrier, Enabled: enabled}
	}
	cases := []struct {
		name  string
		self  Option
		other Option
		want  bool
	}{
		{"neither names a carrier", opt(KindShipToAddress, "", true), opt(KindShipToAddress, "", true), true},
		{"this one names none", opt(KindShipToAddress, "", true), opt(KindShipToAddress, "dhl", true), true},
		{"the other names none", opt(KindShipToAddress, "dhl", true), opt(KindShipToAddress, "", true), true},
		{"both name the same carrier", opt(KindShipToAddress, "dhl", true), opt(KindShipToAddress, "dhl", true), true},
		{"different carriers", opt(KindShipToAddress, "dhl", true), opt(KindShipToAddress, "post", true), false},
		{"different kinds", opt(KindShipToAddress, "", true), opt(KindParcelLocker, "", true), false},
		{"the other is disabled", opt(KindShipToAddress, "", true), opt(KindShipToAddress, "", false), false},
		// An option being set up still hears about the clash it would cause once switched on
		{"this one is disabled", opt(KindShipToAddress, "", false), opt(KindShipToAddress, "", true), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Overlapping(c.self, []Option{c.self, c.other})
			if (len(got) == 1) != c.want {
				t.Fatalf("Overlapping = %v, want overlap %v", got, c.want)
			}
			check := overlapCheck(c.self, []Option{c.self, c.other})
			if check.OK == c.want || !check.Advisory || check.Code != "overlap" {
				t.Errorf("overlapCheck = %+v, want OK %v and advisory", check, !c.want)
			}
		})
	}
}

func TestOverlapNamesEveryClashingOption(t *testing.T) {
	self := Option{ID: uuid.New(), Kind: KindShipToAddress, Name: "Delivery", Enabled: true}
	all := []Option{
		self,
		{ID: uuid.New(), Kind: KindShipToAddress, Name: "Express", CarrierSlug: "dhl", Enabled: true},
		{ID: uuid.New(), Kind: KindShipToAddress, Name: "Economy", Enabled: true},
	}
	var names []string
	for _, o := range Overlapping(self, all) {
		names = append(names, o.Name)
	}
	if !slices.Equal(names, []string{"Express", "Economy"}) {
		t.Errorf("got %v", names)
	}
	if c := overlapCheck(self, all); c.Detail == "" || !containsAll(c.Detail, "Express", "Economy") {
		t.Errorf("detail %q should name both options", c.Detail)
	}
}

// An advisory failure warns without holding the option back; any other failure still does
func TestReadyIgnoresAdvisoryChecks(t *testing.T) {
	ok := Check{Code: "kind", OK: true}
	warn := Check{Code: "overlap", OK: false, Advisory: true}
	block := Check{Code: "carrier", OK: false}
	if !Ready([]Check{ok, warn}) {
		t.Error("a failing advisory check made the option not ready")
	}
	if Ready([]Check{ok, warn, block}) {
		t.Error("a failing required check was ignored")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
