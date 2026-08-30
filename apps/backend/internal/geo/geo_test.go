package geo

import "testing"

func TestOpenNoPathIsNoop(t *testing.T) {
	r, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error: %v", err)
	}
	if r.Enabled() {
		t.Error("resolver with no path should be disabled")
	}
	if got := r.Country("8.8.8.8"); got != "" {
		t.Errorf("disabled resolver Country = %q, want empty", got)
	}
}

func TestOpenMissingFileErrors(t *testing.T) {
	if _, err := Open("/no/such/GeoLite2-Country.mmdb"); err == nil {
		t.Error("Open(missing file) should return an error")
	}
}

func TestNilResolverSafe(t *testing.T) {
	var r *Resolver
	if r.Enabled() {
		t.Error("nil resolver should be disabled")
	}
	if got := r.Country("1.1.1.1"); got != "" {
		t.Errorf("nil resolver Country = %q, want empty", got)
	}
}
