package sdk

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

type warranty struct {
	Months    int32     `json:"months"`
	Provider  *string   `json:"provider,omitempty"`
	StartsAt  time.Time `json:"startsAt"`
	ProductID string    `json:"productID"`
	Parts     []string  `json:"parts"` // a list is its own resolver's
	Owner     *warranty `json:"owner"` // and so is an object
}

func TestLinkColumnsReadTheModelsScalarFields(t *testing.T) {
	cols, fields, err := linkColumns(typeOf[warranty]())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cols, ","); got != "months,provider,starts_at,product_id" || len(fields) != 4 {
		t.Errorf("columns %s (%d fields)", got, len(fields))
	}
	for in, want := range map[string]string{"monthsLeft": "months_left", "productID": "product_id", "URL": "url", "plainname": "plainname"} {
		if got := snake(in); got != want {
			t.Errorf("snake(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestLinkRefusesWhatCouldReachSQL(t *testing.T) {
	ctx := context.Background()
	for _, l := range []Link{{Table: "acme; DROP TABLE x", Key: "id"}, {Table: "acme", Key: "Product ID"}, {Table: "acme", Key: "id", Order: "1; --"}} {
		if _, err := LoadOne[warranty](ctx, l, []string{"x"}); err == nil || !strings.Contains(err.Error(), "name") {
			t.Errorf("%+v: %v", l, err)
		}
	}
	// No well-formed id asks nothing of the database, which isn't there in this test
	got, err := LoadMany[warranty](ctx, Link{Table: "acme_w", Key: "product_id"}, []string{"not-a-uuid"})
	if err != nil || len(got) != 1 || got[0] == nil || len(got[0]) != 0 {
		t.Errorf("malformed id: %v, %v", got, err)
	}
	if ids := IDs([]*warranty{{ProductID: "a"}, {ProductID: "b"}}, func(w *warranty) string { return w.ProductID }); strings.Join(ids, ",") != "a,b" {
		t.Errorf("IDs %v", ids)
	}
}
