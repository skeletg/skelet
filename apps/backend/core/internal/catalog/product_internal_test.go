package catalog

import (
	"strings"
	"testing"
)

// Locks the update diff-list: a field is reported changed iff its input pointer is non-nil, in declaration order
func TestChangedProductFields(t *testing.T) {
	if got := changedProductFields(UpdateProductInput{}); len(got) != 0 {
		t.Fatalf("empty input: got %v, want none", got)
	}

	s := "x"
	price := int64(5)
	tracks := true
	got := changedProductFields(UpdateProductInput{
		DefaultPrice:            &price,
		URL:                     &s,
		TracksInventoryOverride: &tracks,
	})
	want := "defaultPrice,url,tracksInventoryOverride"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %q", got, want)
	}
}
