package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSummarizeProps(t *testing.T) {
	cases := []struct {
		event, in, want string
	}{
		// page_view: title (product/category/search name) wins over the raw path
		{EventPageView, `{"title":"Blue Widget","page_key":"/products/:id"}`, "Blue Widget"},
		{EventPageView, `{"page_key":"/products","viewport":"desktop"}`, "/products"},
		{EventProductFilter, `{"filters":"Color: Red, Blue","name":"product_filter"}`, "Color: Red, Blue"},
		{EventCustom, `{"name":"sidebar_open","page_key":"/x"}`, "sidebar_open"},
		{EventEmailClicked, `{"url":"https://x/y","link_index":0}`, "https://x/y"},
		{EventConversion, `{"order_id":"abc-123","revenue_cents":500}`, "abc-123"},
		{EventCustom, `{}`, ""},
		{EventCustom, ``, ""},
		{EventCustom, `not json`, ""},
	}
	for _, c := range cases {
		if got := SummarizeProps(c.event, json.RawMessage(c.in)); got != c.want {
			t.Errorf("SummarizeProps(%q, %q) = %q, want %q", c.event, c.in, got, c.want)
		}
	}
}

func TestAsTime(t *testing.T) {
	if asTime(nil) != nil {
		t.Error("asTime(nil) should be nil")
	}
	if asTime("not a time") != nil {
		t.Error("asTime(non-time) should be nil")
	}
	now := time.Now()
	got := asTime(now)
	if got == nil || !got.Equal(now) {
		t.Errorf("asTime(time) = %v, want %v", got, now)
	}
}
