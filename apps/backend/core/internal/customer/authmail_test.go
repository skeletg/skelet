package customer

import "testing"

// The fallback builds this itself, and a malformed From is what gets a transactional send rejected by the relay
func TestStoreSnapshotFromHeader(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store StoreSnapshot
		want  string
	}{
		{"name and address", StoreSnapshot{EmailFromAddress: "hi@shop.test", EmailFromName: "Shop"}, "Shop <hi@shop.test>"},
		{"address only", StoreSnapshot{EmailFromAddress: "hi@shop.test"}, "hi@shop.test"},
		{"neither configured", StoreSnapshot{}, "no-reply@skelet.local"},
		{"name only", StoreSnapshot{EmailFromName: "Shop"}, "Shop <no-reply@skelet.local>"},
		{"padded values", StoreSnapshot{EmailFromAddress: "  hi@shop.test ", EmailFromName: " Shop "}, "Shop <hi@shop.test>"},
	} {
		if got := tc.store.fromHeader(); got != tc.want {
			t.Errorf("%s: fromHeader() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
