package syncveto

import (
	"encoding/json"
	"testing"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantReject bool
		wantReason string
		wantErr    bool
	}{
		{name: "empty body allows", body: "", wantReject: false},
		{name: "reject false allows", body: `{"reject":false,"reason":"ignored"}`, wantReject: false},
		{name: "reject true vetoes with reason", body: `{"reject":true,"reason":"legal hold"}`, wantReject: true, wantReason: "legal hold"},
		{name: "malformed json errors", body: `{not json`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejected, reason, err := Decode([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rejected != tc.wantReject || reason != tc.wantReason {
				t.Errorf("= (%v, %q), want (%v, %q)", rejected, reason, tc.wantReject, tc.wantReason)
			}
		})
	}
}

func TestResponseKeepsWireNamesWhenEmbedded(t *testing.T) {
	var resp struct {
		Response
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(`{"reject":true,"reason":"no","tags":["a"]}`), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Reject || resp.Reason != "no" || len(resp.Tags) != 1 {
		t.Errorf("embedded decode = %+v", resp)
	}
	out, err := json.Marshal(Response{Reject: true, Reason: "no"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"reject":true,"reason":"no"}` {
		t.Errorf("marshal = %s", out)
	}
}
