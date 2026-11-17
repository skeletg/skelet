package sdk

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRegisterRecordExtension(t *testing.T) {
	t.Cleanup(recordExtensions.reset)
	acme := &Plugin{slug: "acme"}
	acme.RegisterRecordExtension(RecordExtension{Record: RecordProduct, Permission: "acme.manage"})
	acme.RegisterRecordExtension(RecordExtension{Record: RecordCustomer})
	if got := RecordExtensions(); len(got) != 2 || got[0].PluginSlug != "acme" || got[0].Extension.Permission != "acme.manage" {
		t.Fatalf("registered %+v", got)
	}
	for name, tc := range map[string]struct {
		e     RecordExtension
		panic string
	}{
		"twice":     {RecordExtension{Record: RecordProduct}, "already declared"},
		"no record": {RecordExtension{Record: "order"}, "takes a part"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, tc.panic) {
					t.Errorf("panic %q, want one about %q", r, tc.panic)
				}
			}()
			acme.RegisterRecordExtension(tc.e)
		})
	}
}

// null is how a client clears a field it doesn't mean to send, not a part to write as zeroes
func TestExtensionReadsNullAsNothingSent(t *testing.T) {
	acme := &Plugin{slug: "acme"}
	var part struct{ Months int }
	for raw, given := range map[string]bool{`null`: false, `{"months": 24}`: true} {
		if got, err := acme.Extension(fakeSaving{"acme": raw}, &part); got != given || err != nil {
			t.Errorf("Extension(%s) = %v, %v - want %v", raw, got, err, given)
		}
	}
	if _, err := acme.Extension(fakeSaving{"acme": `"oops"`}, &part); err == nil {
		t.Error("a part that doesn't decode was taken")
	}
}

type fakeSaving map[string]string

func (fakeSaving) SavingRecord() string      { return RecordProduct }
func (fakeSaving) SavingRecordID() uuid.UUID { return uuid.Nil }
func (fakeSaving) SavingCreated() bool       { return false }
func (fakeSaving) SavingTx() *sql.Tx         { return nil }
func (f fakeSaving) SavingExtension(key string) (json.RawMessage, bool) {
	v, ok := f[key]
	return json.RawMessage(v), ok
}
