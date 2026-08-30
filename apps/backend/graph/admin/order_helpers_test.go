package admin

import (
	"testing"

	"github.com/skeletg/skelet/graph/admin/model"
)

// A nil address block has to come out zeroed so the service's own resolveAddress refuses it, rather than the resolver panicking on a draft order with no address yet
func TestToOrderAddressInputZeroesANilBlock(t *testing.T) {
	got := toOrderAddressInput(nil)
	if got.FirstName != "" || got.Street1 != "" || got.CountryCode != "" {
		t.Errorf("toOrderAddressInput(nil) = %+v, want the zero address", got)
	}
	if got.Company != nil || got.Street2 != nil || got.State != nil || got.Phone != nil {
		t.Errorf("toOrderAddressInput(nil) = %+v, want every optional field nil", got)
	}
}

// A dropped field means an order ships to an incomplete address, so every one has to survive the hop
func TestToOrderAddressInputCarriesEveryField(t *testing.T) {
	company, street2, state, phone := "Analytical", "Apt 2", "LDN", "+445550000"
	in := &model.AddressInput{
		FirstName: "Ada", LastName: "Lovelace", Company: &company,
		Street1: "1 Bridge St", Street2: &street2, City: "London",
		State: &state, PostalCode: "E1 6AN", CountryCode: "GB", Phone: &phone,
	}

	got := toOrderAddressInput(in)
	if got.FirstName != "Ada" || got.LastName != "Lovelace" || got.City != "London" ||
		got.Street1 != "1 Bridge St" || got.PostalCode != "E1 6AN" || got.CountryCode != "GB" {
		t.Errorf("required fields = %+v, want them carried through", got)
	}
	for name, pair := range map[string][2]*string{
		"company": {in.Company, got.Company},
		"street2": {in.Street2, got.Street2},
		"state":   {in.State, got.State},
		"phone":   {in.Phone, got.Phone},
	} {
		want, have := pair[0], pair[1]
		if have == nil || *have != *want {
			t.Errorf("%s = %v, want %q", name, have, *want)
		}
	}
}

// An omitted optional field must stay nil rather than becoming an empty string, which would print a blank line on the invoice
func TestToOrderAddressInputKeepsOmittedFieldsNil(t *testing.T) {
	got := toOrderAddressInput(&model.AddressInput{
		FirstName: "Ada", LastName: "Lovelace", Street1: "1 Bridge St",
		City: "London", PostalCode: "E1 6AN", CountryCode: "GB",
	})
	if got.Company != nil {
		t.Errorf("company = %v, want nil", got.Company)
	}
	if got.Street2 != nil {
		t.Errorf("street2 = %v, want nil", got.Street2)
	}
	if got.State != nil {
		t.Errorf("state = %v, want nil", got.State)
	}
	if got.Phone != nil {
		t.Errorf("phone = %v, want nil", got.Phone)
	}
}

func TestInt32Deref(t *testing.T) {
	if got := int32Deref(nil); got != 0 {
		t.Errorf("int32Deref(nil) = %d, want 0", got)
	}
	for _, in := range []int32{0, 1, -5, 2147483647} {
		v := in
		if got := int32Deref(&v); got != in {
			t.Errorf("int32Deref(%d) = %d, want %d", in, got, in)
		}
	}
}

// An empty string has to become GraphQL null, or an unset moderator note renders as a present-but-blank field
func TestStrPtrOrNil(t *testing.T) {
	if got := strPtrOrNil(""); got != nil {
		t.Errorf("strPtrOrNil(\"\") = %v, want nil", got)
	}
	for _, in := range []string{"note", " ", "0", "null"} {
		got := strPtrOrNil(in)
		if got == nil || *got != in {
			t.Errorf("strPtrOrNil(%q) = %v, want %q - only the empty string is null", in, got, in)
		}
	}
}

func TestLowerAllMapsEnumSpellingsOntoTheColumn(t *testing.T) { // statuses are stored lowercase, so an unlowered filter matches zero rows and the grid silently shows nothing
	got := lowerAll([]string{"PENDING", "Confirmed", "fulfilled"})
	want := []string{"pending", "confirmed", "fulfilled"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("lowerAll[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if empty := lowerAll(nil); len(empty) != 0 {
		t.Errorf("lowerAll(nil) = %v, want empty", empty)
	}
}
