package catalog

import (
	"reflect"
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

func TestChangedProductFieldsCoversEveryInputField(t *testing.T) {
	typ := reflect.TypeFor[UpdateProductInput]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Name == "ID" {
			continue
		}
		in := UpdateProductInput{}
		v := reflect.ValueOf(&in).Elem().Field(i)
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Pointer:
			v.Set(reflect.New(f.Type.Elem()))
		default:
			t.Fatalf("UpdateProductInput.%s has kind %s the test doesn't know how to set", f.Name, v.Kind())
		}
		if len(changedProductFields(in)) == 0 {
			t.Errorf("UpdateProductInput.%s is missing from changedProductFields", f.Name)
		}
	}
}
